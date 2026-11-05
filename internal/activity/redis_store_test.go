package activity

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisTestStore builds a store on the Redis of the local stack. The test
// skips when no server answers, so `go test ./...` runs without one. The key
// is the one the service uses, so it is put back as it was when the test ends.
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

	before, err := client.Get(ctx, peakKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		t.Fatalf("read %s: %v", peakKey, err)
	}
	absent := errors.Is(err, redis.Nil)
	t.Cleanup(func() {
		if absent {
			client.Del(context.Background(), peakKey)
		} else {
			client.Set(context.Background(), peakKey, before, 0)
		}
		_ = client.Close()
	})

	return NewRedisStore(client)
}

func TestRedisStoreKeepsTheLargestNumberUntilNobodyIsHeld(t *testing.T) {
	store := redisTestStore(t)
	ctx := t.Context()

	steps := []struct {
		users int
		want  int
	}{
		{users: 0, want: 0},
		{users: 2, want: 2},
		{users: 11, want: 11},
		{users: 5, want: 11},
		{users: 0, want: 0},
		{users: 1, want: 1},
	}
	for _, step := range steps {
		peak, err := store.RecordPeak(ctx, step.users)
		if err != nil {
			t.Fatalf("RecordPeak(%d): %v", step.users, err)
		}
		if peak != step.want {
			t.Fatalf("RecordPeak(%d) = %d, want %d", step.users, peak, step.want)
		}
	}
}
