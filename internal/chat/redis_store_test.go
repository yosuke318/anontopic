package chat

import (
	"context"
	"fmt"
	"maps"
	"math/rand/v2"
	"os"
	"slices"
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

// testConversationID is a room of its own, so that tests running against the
// same Redis do not read each other's participants.
func testConversationID(t *testing.T, store *RedisStore) string {
	t.Helper()

	id := fmt.Sprintf("%s-%d", t.Name(), rand.Int64())
	t.Cleanup(func() { store.client.Del(context.Background(), presenceKey(id)) })

	return id
}

// testTopicID is a topic of its own, so that tests running against the same
// Redis do not count each other's participants.
func testTopicID(t *testing.T, store *RedisStore) int {
	t.Helper()

	id := 1_000_000 + rand.IntN(1_000_000_000)
	t.Cleanup(func() { store.client.Del(context.Background(), topicPresenceKey(id)) })

	return id
}

func TestRedisStoreCarriesEventsToEverySubscriber(t *testing.T) {
	store := redisTestStore(t)
	ctx := t.Context()
	id := testConversationID(t, store)

	// One subscription per server holding a connection to the room.
	first, err := store.Subscribe(ctx, id)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer first.Close()

	second, err := store.Subscribe(ctx, id)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer second.Close()

	if err := store.Publish(ctx, id, []byte(`{"type":"message"}`)); err != nil {
		t.Fatalf("publish: %v", err)
	}

	for name, sub := range map[string]Subscription{"first": first, "second": second} {
		select {
		case payload := <-sub.Events():
			if string(payload) != `{"type":"message"}` {
				t.Fatalf("the %s subscriber read %q", name, payload)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("the %s subscriber read nothing", name)
		}
	}
}

func TestRedisStoreClosesTheStreamOfAClosedSubscription(t *testing.T) {
	store := redisTestStore(t)
	id := testConversationID(t, store)

	sub, err := store.Subscribe(t.Context(), id)
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if err := sub.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	select {
	case _, open := <-sub.Events():
		if open {
			t.Fatal("a closed subscription is still delivering events")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the stream of a closed subscription was left open")
	}
}

func TestRedisStoreCountsEveryParticipantOnce(t *testing.T) {
	store := redisTestStore(t)
	ctx := t.Context()
	id := testConversationID(t, store)
	topicID := testTopicID(t, store)
	now := time.Now().UTC()
	ttl := time.Minute

	connected, err := store.Join(ctx, Seat{ConversationID: id, TopicID: topicID, Token: "alice"}, now, ttl)
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if !slices.Equal(connected, []string{"alice"}) {
		t.Fatalf("connected = %v, want the participant that joined", connected)
	}

	// A participant who connects again holds the single entry they had.
	if connected, err = store.Join(ctx, Seat{ConversationID: id, TopicID: topicID, Token: "alice"}, now.Add(time.Second), ttl); err != nil {
		t.Fatalf("join again: %v", err)
	}
	if !slices.Equal(connected, []string{"alice"}) {
		t.Fatalf("connected = %v, want one entry for the participant that connected again", connected)
	}

	if connected, err = store.Join(ctx, Seat{ConversationID: id, TopicID: topicID, Token: "bob"}, now.Add(2*time.Second), ttl); err != nil {
		t.Fatalf("join: %v", err)
	}
	if len(connected) != 2 {
		t.Fatalf("connected = %v, want both participants", connected)
	}

	if connected, err = store.Leave(ctx, Seat{ConversationID: id, TopicID: topicID, Token: "alice"}, now.Add(3*time.Second), ttl); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if !slices.Equal(connected, []string{"bob"}) {
		t.Fatalf("connected = %v, want the participant that stayed", connected)
	}
}

func TestRedisStoreDropsAParticipantThatStoppedSayingItIsThere(t *testing.T) {
	store := redisTestStore(t)
	ctx := t.Context()
	id := testConversationID(t, store)
	topicID := testTopicID(t, store)
	now := time.Now().UTC()
	ttl := 30 * time.Second

	if _, err := store.Join(ctx, Seat{ConversationID: id, TopicID: topicID, Token: "alice"}, now, ttl); err != nil {
		t.Fatalf("join: %v", err)
	}
	if _, err := store.Join(ctx, Seat{ConversationID: id, TopicID: topicID, Token: "bob"}, now, ttl); err != nil {
		t.Fatalf("join: %v", err)
	}

	// The server holding alice is gone: only bob keeps saying it is there.
	connected, err := store.Heartbeat(ctx, Seat{ConversationID: id, TopicID: topicID, Token: "bob"}, now.Add(2*ttl), ttl)
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if !slices.Equal(connected, []string{"bob"}) {
		t.Fatalf("connected = %v, want the participant that is still there", connected)
	}
}

func TestRedisStoreCountsTheParticipantsConnectedUnderEachTopic(t *testing.T) {
	store := redisTestStore(t)
	ctx := t.Context()
	first := testConversationID(t, store)
	second := testConversationID(t, store)
	other := testConversationID(t, store)
	topicID := testTopicID(t, store)
	otherTopicID := testTopicID(t, store)
	quietTopicID := testTopicID(t, store)
	now := time.Now().UTC()
	ttl := 30 * time.Second

	for _, seat := range []Seat{
		{ConversationID: first, TopicID: topicID, Token: "alice"},
		{ConversationID: first, TopicID: topicID, Token: "bob"},
		{ConversationID: second, TopicID: topicID, Token: "carol"},
		{ConversationID: other, TopicID: otherTopicID, Token: "dave"},
	} {
		if _, err := store.Join(ctx, seat, now, ttl); err != nil {
			t.Fatalf("join %s: %v", seat.Token, err)
		}
	}

	// A participant who connects again is counted once, and one who left is
	// not counted.
	if _, err := store.Join(ctx, Seat{ConversationID: first, TopicID: topicID, Token: "alice"}, now.Add(time.Second), ttl); err != nil {
		t.Fatalf("join again: %v", err)
	}
	if _, err := store.Leave(ctx, Seat{ConversationID: second, TopicID: topicID, Token: "carol"}, now.Add(time.Second), ttl); err != nil {
		t.Fatalf("leave: %v", err)
	}

	counts, err := store.CountConnected(ctx, []int{topicID, otherTopicID, quietTopicID}, now.Add(2*time.Second), ttl)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	want := map[int]int{topicID: 2, otherTopicID: 1, quietTopicID: 0}
	if !maps.Equal(counts, want) {
		t.Fatalf("counts = %v, want %v", counts, want)
	}

	// Only alice was kept after the others joined, so once the others' entries
	// are ttl old only she is counted.
	counts, err = store.CountConnected(ctx, []int{topicID, otherTopicID}, now.Add(ttl), ttl)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	want = map[int]int{topicID: 1, otherTopicID: 0}
	if !maps.Equal(counts, want) {
		t.Fatalf("counts after ttl = %v, want %v", counts, want)
	}
}
