package matching

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisTestStore builds a store on the Redis of the local stack. The test
// skips when no server answers, so `go test ./...` runs without one.
func redisTestStore(t *testing.T) *RedisStore {
	t.Helper()

	url := os.Getenv("REDIS_URL")
	if url == "" {
		url = "redis://localhost:6379/0"
	}

	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatalf("parse REDIS_URL: %v", err)
	}

	client := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		t.Skipf("no Redis at %s: %v", url, err)
	}
	t.Cleanup(func() { _ = client.Close() })

	return NewRedisStore(client)
}

// testQueue is a queue of its own, so that tests running against the same
// Redis do not read each other's users.
func testQueue(t *testing.T, store *RedisStore, roomType int) Queue {
	t.Helper()

	q := Queue{TopicID: rand.IntN(1_000_000) + 1, RoomType: roomType}
	t.Cleanup(func() { store.client.Del(context.Background(), queueKey(q)) })

	return q
}

// testToken is a token of its own, removed from Redis when the test ends.
func testToken(t *testing.T, store *RedisStore, name string) string {
	t.Helper()

	token := fmt.Sprintf("%s-%d", name, rand.Int64())
	t.Cleanup(func() {
		store.client.Del(context.Background(), waitingKey(token), roomKey(token))
	})

	return token
}

// enqueue puts a token in a queue and fails the test when Redis refuses.
func enqueue(t *testing.T, store *RedisStore, token string, q Queue, since time.Time) {
	t.Helper()

	if err := store.Enqueue(context.Background(), token, q, since, time.Minute); err != nil {
		t.Fatalf("Enqueue(%s): %v", token, err)
	}
}

func TestEnqueuedUserIsWaitingInTheQueueItPicked(t *testing.T) {
	store := redisTestStore(t)
	q := testQueue(t, store, 2)
	token := testToken(t, store, "alice")
	now := time.Now().UTC().Truncate(time.Millisecond)

	enqueue(t, store, token, q, now)

	state, err := store.Lookup(context.Background(), token)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if state.Kind != StateWaiting || state.Queue != q {
		t.Fatalf("state = %+v, want waiting in %+v", state, q)
	}
	if !state.WaitingSince.Equal(now) {
		t.Fatalf("waiting since = %s, want %s", state.WaitingSince, now)
	}
}

func TestEnqueueRefusesAUserWaitingInAnotherQueue(t *testing.T) {
	store := redisTestStore(t)
	first := testQueue(t, store, 2)
	second := testQueue(t, store, 3)
	token := testToken(t, store, "alice")

	enqueue(t, store, token, first, time.Now().UTC())

	err := store.Enqueue(context.Background(), token, second, time.Now().UTC(), time.Minute)
	if !errors.Is(err, ErrAlreadyWaiting) {
		t.Fatalf("err = %v, want %v", err, ErrAlreadyWaiting)
	}
}

func TestEnqueueRefusesAUserWhoHoldsARoom(t *testing.T) {
	store := redisTestStore(t)
	q := testQueue(t, store, 2)
	token := testToken(t, store, "alice")
	ctx := context.Background()

	enqueue(t, store, token, q, time.Now().UTC())
	if _, err := store.FormRoom(ctx, q, time.Now().UTC(), time.Minute, time.Minute); err != nil {
		t.Fatalf("FormRoom: %v", err)
	}

	// A room of two cannot be formed out of one user, so the user is put back
	// in the queue by hand to reach the state the check is about.
	if err := store.Assign(ctx, []string{token}, Conversation{
		ID: "11111111-1111-1111-1111-111111111111", TopicID: q.TopicID, RoomType: 2,
		StartedAt: time.Now().UTC(),
	}, time.Minute); err != nil {
		t.Fatalf("Assign: %v", err)
	}

	err := store.Enqueue(ctx, token, q, time.Now().UTC(), time.Minute)
	if !errors.Is(err, ErrAlreadyMatched) {
		t.Fatalf("err = %v, want %v", err, ErrAlreadyMatched)
	}
}

func TestFormRoomTakesTheUsersThatWaitedLongest(t *testing.T) {
	store := redisTestStore(t)
	q := testQueue(t, store, 2)
	now := time.Now().UTC()

	first := testToken(t, store, "first")
	second := testToken(t, store, "second")
	third := testToken(t, store, "third")
	enqueue(t, store, first, q, now.Add(-3*time.Second))
	enqueue(t, store, second, q, now.Add(-2*time.Second))
	enqueue(t, store, third, q, now.Add(-time.Second))

	participants, err := store.FormRoom(context.Background(), q, now, time.Minute, time.Minute)
	if err != nil {
		t.Fatalf("FormRoom: %v", err)
	}
	if !slices.Equal(participants, []string{first, second}) {
		t.Fatalf("participants = %v, want %v", participants, []string{first, second})
	}

	state, err := store.Lookup(context.Background(), third)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if state.Kind != StateWaiting {
		t.Fatalf("third user kind = %v, want %v", state.Kind, StateWaiting)
	}
}

func TestFormRoomOfThreeSettlesForTwoAfterTheFallbackWait(t *testing.T) {
	store := redisTestStore(t)
	q := testQueue(t, store, 3)
	now := time.Now().UTC()

	first := testToken(t, store, "first")
	second := testToken(t, store, "second")
	enqueue(t, store, first, q, now.Add(-90*time.Second))
	enqueue(t, store, second, q, now.Add(-80*time.Second))

	ctx := context.Background()
	if participants, err := store.FormRoom(ctx, q, now, 5*time.Minute, 2*time.Minute); err != nil {
		t.Fatalf("FormRoom: %v", err)
	} else if len(participants) != 0 {
		t.Fatalf("participants = %v, want none before the fallback wait", participants)
	}

	participants, err := store.FormRoom(ctx, q, now, 5*time.Minute, time.Minute)
	if err != nil {
		t.Fatalf("FormRoom: %v", err)
	}
	if !slices.Equal(participants, []string{first, second}) {
		t.Fatalf("participants = %v, want %v", participants, []string{first, second})
	}

	state, err := store.Lookup(ctx, first)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if state.Queue.RoomType != 2 {
		t.Fatalf("room type = %d, want 2", state.Queue.RoomType)
	}
}

func TestFormRoomDropsAWaitThatRanOut(t *testing.T) {
	store := redisTestStore(t)
	q := testQueue(t, store, 2)
	now := time.Now().UTC()

	stale := testToken(t, store, "stale")
	fresh := testToken(t, store, "fresh")
	enqueue(t, store, stale, q, now.Add(-2*time.Minute))
	enqueue(t, store, fresh, q, now)

	participants, err := store.FormRoom(context.Background(), q, now, time.Minute, time.Minute)
	if err != nil {
		t.Fatalf("FormRoom: %v", err)
	}
	if len(participants) != 0 {
		t.Fatalf("participants = %v, want none: the queue holds one user in time", participants)
	}
}

func TestAssignHandsTheConversationToEveryParticipant(t *testing.T) {
	store := redisTestStore(t)
	q := testQueue(t, store, 2)
	ctx := context.Background()
	now := time.Now().UTC()

	first := testToken(t, store, "first")
	second := testToken(t, store, "second")
	enqueue(t, store, first, q, now)
	enqueue(t, store, second, q, now)

	participants, err := store.FormRoom(ctx, q, now, time.Minute, time.Minute)
	if err != nil {
		t.Fatalf("FormRoom: %v", err)
	}

	// Until the conversation is written, a participant has nothing to join.
	state, err := store.Lookup(ctx, first)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if state.Kind != StateWaiting {
		t.Fatalf("kind = %v, want %v while the conversation is being written", state.Kind, StateWaiting)
	}

	conv := Conversation{
		ID:        "22222222-2222-2222-2222-222222222222",
		TopicID:   q.TopicID,
		RoomType:  2,
		StartedAt: now.Truncate(time.Millisecond),
	}
	if err := store.Assign(ctx, participants, conv, time.Minute); err != nil {
		t.Fatalf("Assign: %v", err)
	}

	for _, token := range participants {
		state, err := store.Lookup(ctx, token)
		if err != nil {
			t.Fatalf("Lookup(%s): %v", token, err)
		}
		if state.Kind != StateMatched || state.Conversation.ID != conv.ID {
			t.Fatalf("%s state = %+v, want conversation %s", token, state, conv.ID)
		}
		if !state.Conversation.StartedAt.Equal(conv.StartedAt) {
			t.Fatalf("started at = %s, want %s", state.Conversation.StartedAt, conv.StartedAt)
		}
	}
}

func TestDiscardReturnsTheParticipantsToTheStateTheyHadBeforeQueueing(t *testing.T) {
	store := redisTestStore(t)
	q := testQueue(t, store, 2)
	ctx := context.Background()
	now := time.Now().UTC()

	first := testToken(t, store, "first")
	second := testToken(t, store, "second")
	enqueue(t, store, first, q, now)
	enqueue(t, store, second, q, now)

	participants, err := store.FormRoom(ctx, q, now, time.Minute, time.Minute)
	if err != nil {
		t.Fatalf("FormRoom: %v", err)
	}
	if err := store.Discard(ctx, participants); err != nil {
		t.Fatalf("Discard: %v", err)
	}

	for _, token := range participants {
		state, err := store.Lookup(ctx, token)
		if err != nil {
			t.Fatalf("Lookup(%s): %v", token, err)
		}
		if state.Kind != StateIdle {
			t.Fatalf("%s kind = %v, want %v", token, state.Kind, StateIdle)
		}
	}
}

func TestRemoveTakesTheUserOutOfTheQueue(t *testing.T) {
	store := redisTestStore(t)
	q := testQueue(t, store, 2)
	ctx := context.Background()
	now := time.Now().UTC()

	first := testToken(t, store, "first")
	second := testToken(t, store, "second")
	enqueue(t, store, first, q, now)
	enqueue(t, store, second, q, now)

	if err := store.Remove(ctx, first); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	participants, err := store.FormRoom(ctx, q, now, time.Minute, time.Minute)
	if err != nil {
		t.Fatalf("FormRoom: %v", err)
	}
	if len(participants) != 0 {
		t.Fatalf("participants = %v, want none: one of the two users left", participants)
	}

	state, err := store.Lookup(ctx, first)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if state.Kind != StateIdle {
		t.Fatalf("kind = %v, want %v", state.Kind, StateIdle)
	}
}

func TestConcurrentFormRoomGivesEachUserOneRoom(t *testing.T) {
	store := redisTestStore(t)
	q := testQueue(t, store, 2)
	ctx := context.Background()
	now := time.Now().UTC()

	const users = 20
	for i := range users {
		enqueue(t, store, testToken(t, store, fmt.Sprintf("user%02d", i)), q, now)
	}

	var (
		mu     sync.Mutex
		formed [][]string
		wg     sync.WaitGroup
	)
	for range users {
		wg.Add(1)
		go func() {
			defer wg.Done()

			participants, err := store.FormRoom(ctx, q, now, time.Minute, time.Minute)
			if err != nil {
				t.Errorf("FormRoom: %v", err)
				return
			}
			if len(participants) == 0 {
				return
			}

			mu.Lock()
			formed = append(formed, participants)
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(formed) != users/2 {
		t.Fatalf("rooms = %d, want %d", len(formed), users/2)
	}

	seen := make(map[string]int, users)
	for _, participants := range formed {
		if len(participants) != 2 {
			t.Fatalf("participants = %v, want two", participants)
		}
		for _, token := range participants {
			seen[token]++
			if seen[token] > 1 {
				t.Fatalf("%s was put in %d rooms", token, seen[token])
			}
		}
	}
	if len(seen) != users {
		t.Fatalf("users in a room = %d, want %d", len(seen), users)
	}
}
