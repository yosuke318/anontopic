package capacity

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// testTTL is the lifetime the tests give a lease. They pass the moments they
// want themselves, so nothing waits for it to run out.
const testTTL = 30 * time.Second

// redisTestStore builds a store on the Redis of the local stack. The test
// skips when no server answers, so `go test ./...` runs without one. Every
// store writes under a prefix of its own, so that tests running against the
// same Redis do not count each other's connections.
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

	prefix := fmt.Sprintf("capacity-test:%s:%d:", t.Name(), rand.Int64())
	t.Cleanup(func() {
		keys, err := client.Keys(context.Background(), prefix+"*").Result()
		if err == nil && len(keys) > 0 {
			client.Del(context.Background(), keys...)
		}
		_ = client.Close()
	})

	return &RedisStore{client: client, prefix: prefix}
}

// acquire counts one connection and fails the test when the store errors.
func acquire(t *testing.T, store *RedisStore, id, ipHash string, now time.Time, limits Limits) Acquisition {
	t.Helper()

	got, err := store.Acquire(t.Context(), id, ipHash, now, testTTL, limits)
	if err != nil {
		t.Fatalf("Acquire %s: %v", id, err)
	}
	return got
}

func TestRedisStoreCountsConnectionsAcrossServers(t *testing.T) {
	store := redisTestStore(t)
	now := time.Now().UTC()
	limits := Limits{Connections: 3, PerIPHash: 3}

	// Every connection is counted against the same total, whichever server
	// took it, so the third one is the last that fits.
	for i := range 3 {
		got := acquire(t, store, fmt.Sprintf("lease-%d", i), "one-network", now, limits)
		if got.Outcome != Granted {
			t.Fatalf("connection %d: outcome = %d, want %d", i+1, got.Outcome, Granted)
		}
		if got.Connections != i+1 {
			t.Fatalf("connection %d: connections = %d, want %d", i+1, got.Connections, i+1)
		}
	}

	got := acquire(t, store, "lease-3", "one-network", now, limits)
	if got.Outcome != AtCapacity {
		t.Fatalf("outcome = %d, want %d", got.Outcome, AtCapacity)
	}
	if got.Connections != 3 {
		t.Fatalf("connections = %d, want 3", got.Connections)
	}

	held, err := store.Connections(t.Context(), now, testTTL)
	if err != nil {
		t.Fatalf("Connections: %v", err)
	}
	if held != 3 {
		t.Fatalf("connections = %d, want 3", held)
	}
}

func TestRedisStoreCountsTheConnectionsOfOneAddress(t *testing.T) {
	store := redisTestStore(t)
	now := time.Now().UTC()
	limits := Limits{Connections: 100, PerIPHash: 2}

	for i := range 2 {
		if got := acquire(t, store, fmt.Sprintf("lease-%d", i), "one-network", now, limits); got.Outcome != Granted {
			t.Fatalf("connection %d: outcome = %d, want %d", i+1, got.Outcome, Granted)
		}
	}

	got := acquire(t, store, "lease-2", "one-network", now, limits)
	if got.Outcome != TooManyPerIPHash {
		t.Fatalf("outcome = %d, want %d", got.Outcome, TooManyPerIPHash)
	}
	if got.PerIPHash != 2 {
		t.Fatalf("per ip hash = %d, want 2", got.PerIPHash)
	}

	// The address next door is counted on its own.
	if got := acquire(t, store, "lease-3", "another-network", now, limits); got.Outcome != Granted {
		t.Fatalf("another network: outcome = %d, want %d", got.Outcome, Granted)
	}
}

func TestRedisStoreStopsCountingALeaseNothingRenews(t *testing.T) {
	store := redisTestStore(t)
	start := time.Now().UTC()
	limits := Limits{Connections: 1, PerIPHash: 1}

	acquire(t, store, "abandoned", "one-network", start, limits)

	// A lease exactly as old as its lifetime is already gone, which is the
	// boundary a server that died leaves its leases on.
	if got := acquire(t, store, "next", "another-network", start.Add(testTTL-time.Millisecond), limits); got.Outcome != AtCapacity {
		t.Fatalf("outcome = %d, want %d", got.Outcome, AtCapacity)
	}
	if got := acquire(t, store, "next", "another-network", start.Add(testTTL), limits); got.Outcome != Granted {
		t.Fatalf("outcome = %d, want %d", got.Outcome, Granted)
	}
}

func TestRedisStoreKeepsARenewedLeaseCounted(t *testing.T) {
	store := redisTestStore(t)
	start := time.Now().UTC()
	limits := Limits{Connections: 1, PerIPHash: 1}

	acquire(t, store, "open", "one-network", start, limits)

	later := start.Add(testTTL)
	if err := store.Renew(t.Context(), "open", "one-network", later, testTTL); err != nil {
		t.Fatalf("Renew: %v", err)
	}

	// The connection is still open, so it still holds the only place there is.
	if got := acquire(t, store, "next", "another-network", later, limits); got.Outcome != AtCapacity {
		t.Fatalf("outcome = %d, want %d", got.Outcome, AtCapacity)
	}
}

func TestRedisStoreGivesBackAReleasedPlace(t *testing.T) {
	store := redisTestStore(t)
	now := time.Now().UTC()
	limits := Limits{Connections: 1, PerIPHash: 1}

	acquire(t, store, "closing", "one-network", now, limits)

	if err := store.Release(t.Context(), "closing", "one-network"); err != nil {
		t.Fatalf("Release: %v", err)
	}
	// Releasing a place that is already gone is not an error.
	if err := store.Release(t.Context(), "closing", "one-network"); err != nil {
		t.Fatalf("Release again: %v", err)
	}

	if got := acquire(t, store, "next", "another-network", now, limits); got.Outcome != Granted {
		t.Fatalf("outcome = %d, want %d", got.Outcome, Granted)
	}

	held, err := store.Connections(t.Context(), now, testTTL)
	if err != nil {
		t.Fatalf("Connections: %v", err)
	}
	if held != 1 {
		t.Fatalf("connections = %d, want 1", held)
	}
}

func TestRedisStoreSpendsABurstAndThenRefills(t *testing.T) {
	store := redisTestStore(t)
	limit := Limit{Burst: 3, Interval: time.Second}
	start := time.Now().UTC()

	take := func(at time.Time) bool {
		t.Helper()

		allowed, err := store.Take(t.Context(), "message", "a-session-token", limit, at)
		if err != nil {
			t.Fatalf("Take: %v", err)
		}
		return allowed
	}

	// The bucket starts full, so a burst goes through at once.
	for i := range 3 {
		if !take(start) {
			t.Fatalf("token %d was held back", i+1)
		}
	}
	if take(start) {
		t.Fatal("a fourth token was spent, want the burst spent")
	}

	// Half an interval is not a whole token.
	if take(start.Add(500 * time.Millisecond)) {
		t.Fatal("half an interval let a token through")
	}
	if !take(start.Add(time.Second)) {
		t.Fatal("a whole interval earned no token")
	}

	// Long enough without spending anything and the bucket is full again,
	// but no fuller than the burst it holds.
	for i := range 3 {
		if !take(start.Add(time.Minute)) {
			t.Fatalf("token %d after a quiet minute was held back", i+1)
		}
	}
	if take(start.Add(time.Minute)) {
		t.Fatal("the bucket holds more than its burst")
	}
}

func TestRedisStoreKeepsOneBucketPerRateAndSubject(t *testing.T) {
	store := redisTestStore(t)
	limit := Limit{Burst: 1, Interval: time.Minute}
	now := time.Now().UTC()

	take := func(name, subject string) bool {
		t.Helper()

		allowed, err := store.Take(t.Context(), name, subject, limit, now)
		if err != nil {
			t.Fatalf("Take: %v", err)
		}
		return allowed
	}

	if !take("message", "alice") {
		t.Fatal("the first message was held back")
	}
	if take("message", "alice") {
		t.Fatal("the second message went through the same bucket")
	}

	// Another subject, and another rate for the same subject, each have a
	// bucket of their own.
	if !take("message", "bob") {
		t.Fatal("another sender was held back")
	}
	if !take("match", "alice") {
		t.Fatal("another rate was held back")
	}
}

// Several servers taking the last places at once must not get more of them
// than there are, which is the property the whole cap rests on.
func TestRedisStoreNeverCountsPastTheLimit(t *testing.T) {
	store := redisTestStore(t)
	now := time.Now().UTC()
	limits := Limits{Connections: 50, PerIPHash: 200}

	const attempts = 200

	granted := make(chan bool, attempts)
	var wg sync.WaitGroup

	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()

			got, err := store.Acquire(context.Background(), fmt.Sprintf("lease-%d", i), "one-network", now, testTTL, limits)
			if err != nil {
				t.Errorf("Acquire: %v", err)
				return
			}
			granted <- got.Outcome == Granted
		}()
	}
	wg.Wait()
	close(granted)

	counted := 0
	for ok := range granted {
		if ok {
			counted++
		}
	}
	if counted != limits.Connections {
		t.Fatalf("counted %d connections, want %d", counted, limits.Connections)
	}

	held, err := store.Connections(t.Context(), now, testTTL)
	if err != nil {
		t.Fatalf("Connections: %v", err)
	}
	if held != limits.Connections {
		t.Fatalf("connections = %d, want %d", held, limits.Connections)
	}
}
