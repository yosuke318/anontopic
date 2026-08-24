package session

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisTestClient connects to the Redis of the local stack. The test skips
// when no server answers, so `go test ./...` runs without one.
func redisTestClient(t *testing.T) *redis.Client {
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

	return client
}

// storeTestSession writes a session and removes it once the test ends.
func storeTestSession(t *testing.T, store *RedisStore, ipHash string, ttl time.Duration) string {
	t.Helper()

	token, err := newToken()
	if err != nil {
		t.Fatalf("newToken: %v", err)
	}
	rec := Record{IPHash: ipHash, IssuedAt: time.Now().UTC().Truncate(time.Millisecond)}

	if err := store.Create(context.Background(), token, rec, ttl); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = store.Delete(context.Background(), token) })

	return token
}

// testIPHash keeps each test on its own index key.
func testIPHash(t *testing.T) string {
	t.Helper()

	token, err := newToken()
	if err != nil {
		t.Fatalf("newToken: %v", err)
	}
	return token
}

func TestRedisStoreKeepsARecordUnderItsTTL(t *testing.T) {
	store := NewRedisStore(redisTestClient(t))
	ctx := context.Background()

	ipHash := testIPHash(t)
	token := storeTestSession(t, store, ipHash, time.Minute)

	rec, err := store.Get(ctx, token)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.IPHash != ipHash {
		t.Fatalf("IPHash = %q, want %q", rec.IPHash, ipHash)
	}
	if rec.IssuedAt.IsZero() {
		t.Fatal("IssuedAt came back zero")
	}

	ttl, err := store.client.TTL(ctx, sessionKeyPrefix+token).Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl <= 0 || ttl > time.Minute {
		t.Fatalf("TTL = %v, want a value inside the minute it was created with", ttl)
	}
}

func TestRedisStoreReportsUnknownTokens(t *testing.T) {
	store := NewRedisStore(redisTestClient(t))
	ctx := context.Background()

	unknown, err := newToken()
	if err != nil {
		t.Fatalf("newToken: %v", err)
	}

	if _, err := store.Get(ctx, unknown); !errors.Is(err, ErrNotStored) {
		t.Fatalf("Get error = %v, want ErrNotStored", err)
	}
	if err := store.Refresh(ctx, unknown, "unknown-ip-hash", time.Minute); !errors.Is(err, ErrNotStored) {
		t.Fatalf("Refresh error = %v, want ErrNotStored", err)
	}
	if err := store.Delete(ctx, unknown); err != nil {
		t.Fatalf("Delete of an unknown token: %v", err)
	}
}

func TestRedisStoreRefreshMovesTheExpiry(t *testing.T) {
	store := NewRedisStore(redisTestClient(t))
	ctx := context.Background()

	ipHash := testIPHash(t)
	token := storeTestSession(t, store, ipHash, time.Second)

	if err := store.Refresh(ctx, token, ipHash, time.Hour); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	ttl, err := store.client.TTL(ctx, sessionKeyPrefix+token).Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl <= time.Second {
		t.Fatalf("TTL = %v, want the refreshed hour", ttl)
	}
}

func TestRedisStoreRefreshKeepsTheIndexAlive(t *testing.T) {
	store := NewRedisStore(redisTestClient(t))
	ctx := context.Background()

	ipHash := testIPHash(t)
	indexKey := ipIndexKeyPrefix + ipHash
	t.Cleanup(func() { _ = store.client.Del(ctx, indexKey).Err() })

	token := storeTestSession(t, store, ipHash, time.Second)

	if err := store.Refresh(ctx, token, ipHash, time.Hour); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// A ban reads the index, so it has to outlive the session it points at.
	ttl, err := store.client.TTL(ctx, indexKey).Result()
	if err != nil {
		t.Fatalf("TTL of the index: %v", err)
	}
	if ttl <= time.Second {
		t.Fatalf("index TTL = %v, want it extended with the session", ttl)
	}

	deleted, err := store.DeleteByIPHash(ctx, ipHash)
	if err != nil {
		t.Fatalf("DeleteByIPHash: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("a ban removed %d sessions, want 1", deleted)
	}
}

func TestRedisStoreRefreshDoesNotShortenTheIndex(t *testing.T) {
	store := NewRedisStore(redisTestClient(t))
	ctx := context.Background()

	ipHash := testIPHash(t)
	indexKey := ipIndexKeyPrefix + ipHash
	t.Cleanup(func() { _ = store.client.Del(ctx, indexKey).Err() })

	longLived := storeTestSession(t, store, ipHash, time.Hour)
	nearDeadline := storeTestSession(t, store, ipHash, time.Hour)

	// A session close to its absolute deadline refreshes with a short ttl and
	// must not cut the index short for the session sharing the address.
	if err := store.Refresh(ctx, nearDeadline, ipHash, time.Minute); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	ttl, err := store.client.TTL(ctx, indexKey).Result()
	if err != nil {
		t.Fatalf("TTL of the index: %v", err)
	}
	if ttl <= time.Minute {
		t.Fatalf("index TTL = %v, want the hour the other session needs", ttl)
	}
	if _, err := store.Get(ctx, longLived); err != nil {
		t.Fatalf("the long lived session: %v", err)
	}
}

func TestRedisStoreDeleteDropsTheIndexEntry(t *testing.T) {
	store := NewRedisStore(redisTestClient(t))
	ctx := context.Background()

	ipHash := testIPHash(t)
	token := storeTestSession(t, store, ipHash, time.Minute)

	if err := store.Delete(ctx, token); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	members, err := store.client.SMembers(ctx, ipIndexKeyPrefix+ipHash).Result()
	if err != nil {
		t.Fatalf("SMembers: %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("index holds %v after the session was deleted", members)
	}
}

func TestRedisStoreDeleteByIPHashRemovesEverySession(t *testing.T) {
	store := NewRedisStore(redisTestClient(t))
	ctx := context.Background()

	ipHash := testIPHash(t)
	tokens := []string{
		storeTestSession(t, store, ipHash, time.Minute),
		storeTestSession(t, store, ipHash, time.Minute),
	}
	other := storeTestSession(t, store, testIPHash(t), time.Minute)

	n, err := store.DeleteByIPHash(ctx, ipHash)
	if err != nil {
		t.Fatalf("DeleteByIPHash: %v", err)
	}
	if n != len(tokens) {
		t.Fatalf("deleted %d sessions, want %d", n, len(tokens))
	}

	for _, token := range tokens {
		if _, err := store.Get(ctx, token); !errors.Is(err, ErrNotStored) {
			t.Fatalf("Get after DeleteByIPHash error = %v, want ErrNotStored", err)
		}
	}
	if _, err := store.Get(ctx, other); err != nil {
		t.Fatalf("Get of a session from another address: %v", err)
	}
}
