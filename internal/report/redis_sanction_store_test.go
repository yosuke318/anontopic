package report

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisTestSanctionStore builds a store on the Redis of the local stack. The
// test skips when no server answers, and every store writes under a prefix of
// its own.
func redisTestSanctionStore(t *testing.T) *RedisSanctionStore {
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

	prefix := fmt.Sprintf("report-test:%s:%d:", t.Name(), rand.Int64())
	t.Cleanup(func() {
		keys, err := client.Keys(context.Background(), prefix+"*").Result()
		if err == nil && len(keys) > 0 {
			client.Del(context.Background(), keys...)
		}
		_ = client.Close()
	})

	return &RedisSanctionStore{client: client, prefix: prefix}
}

func TestRedisSanctionStoreCachesAndForgetsABan(t *testing.T) {
	store := redisTestSanctionStore(t)
	ctx := t.Context()

	if _, ok, err := store.CachedBan(ctx, IdentifierDevice, "d"); err != nil || ok {
		t.Fatalf("CachedBan = %v, %v, want nothing kept", ok, err)
	}

	for _, want := range []bool{true, false} {
		if err := store.CacheBan(ctx, IdentifierDevice, "d", want, time.Minute); err != nil {
			t.Fatalf("CacheBan: %v", err)
		}
		banned, ok, err := store.CachedBan(ctx, IdentifierDevice, "d")
		if err != nil || !ok || banned != want {
			t.Fatalf("CachedBan = %v, %v, %v, want %v", banned, ok, err, want)
		}
	}

	if err := store.ForgetBan(ctx, IdentifierDevice, "d"); err != nil {
		t.Fatalf("ForgetBan: %v", err)
	}
	if _, ok, err := store.CachedBan(ctx, IdentifierDevice, "d"); err != nil || ok {
		t.Fatalf("CachedBan = %v, %v, want nothing kept after ForgetBan", ok, err)
	}
}

func TestRedisSanctionStoreCountsOffencesUntilTheyAreReset(t *testing.T) {
	store := redisTestSanctionStore(t)
	ctx := t.Context()

	for want := 1; want <= 3; want++ {
		n, err := store.AddBlocked(ctx, IdentifierDevice, "d", time.Hour)
		if err != nil || n != want {
			t.Fatalf("AddBlocked = %d, %v, want %d", n, err, want)
		}
	}

	for i, conv := range []string{"c1", "c1", "c2"} {
		n, err := store.AddReported(ctx, IdentifierDevice, "d", conv, time.Hour)
		want := []int{1, 1, 2}[i]
		if err != nil || n != want {
			t.Fatalf("AddReported(%s) = %d, %v, want %d", conv, n, err, want)
		}
	}

	// The window starts with the first offence, and a later one does not
	// push it back.
	ttl := store.client.TTL(ctx, store.key(sanctionBlockedPrefix, IdentifierDevice, "d")).Val()
	if ttl <= 0 || ttl > time.Hour {
		t.Fatalf("TTL of the count = %v, want at most an hour", ttl)
	}

	if err := store.ResetOffences(ctx, IdentifierDevice, "d"); err != nil {
		t.Fatalf("ResetOffences: %v", err)
	}
	if n, err := store.AddBlocked(ctx, IdentifierDevice, "d", time.Hour); err != nil || n != 1 {
		t.Fatalf("AddBlocked after the reset = %d, %v, want 1", n, err)
	}
	if n, err := store.AddReported(ctx, IdentifierDevice, "d", "c3", time.Hour); err != nil || n != 1 {
		t.Fatalf("AddReported after the reset = %d, %v, want 1", n, err)
	}
}

func TestRedisSanctionStoreHandsAWarningOutOnce(t *testing.T) {
	store := redisTestSanctionStore(t)
	ctx := t.Context()

	if err := store.MarkWarning(ctx, IdentifierDevice, "d", time.Minute); err != nil {
		t.Fatalf("MarkWarning: %v", err)
	}
	for i, want := range []bool{true, false} {
		got, err := store.TakeWarning(ctx, IdentifierDevice, "d")
		if err != nil || got != want {
			t.Fatalf("take %d = %v, %v, want %v", i+1, got, err, want)
		}
	}
}
