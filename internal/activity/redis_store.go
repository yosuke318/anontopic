package activity

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// peakKey holds the largest number of users since the service last held
// nobody. It has no lifetime: the number lasts until the service is empty.
const peakKey = "activity:peak"

// peakScript starts the largest number over when nobody is held, and raises it
// otherwise, in one step so that two servers recording at the same time keep
// the larger of their counts.
var peakScript = redis.NewScript(`
local key = KEYS[1]
local users = tonumber(ARGV[1])

if users == 0 then
	redis.call('SET', key, 0)
	return 0
end

local peak = tonumber(redis.call('GET', key) or '0')
if users > peak then
	redis.call('SET', key, users)
	return users
end
return peak
`)

// RedisStore keeps the largest number in Redis, where every server reads the
// same one.
type RedisStore struct {
	client redis.UniversalClient
}

// NewRedisStore builds a Store on top of client.
func NewRedisStore(client redis.UniversalClient) *RedisStore {
	return &RedisStore{client: client}
}

// RecordPeak runs peakScript.
func (s *RedisStore) RecordPeak(ctx context.Context, users int) (int, error) {
	peak, err := peakScript.Run(ctx, s.client, []string{peakKey}, users).Int()
	if err != nil {
		return 0, fmt.Errorf("record the largest number of users: %w", err)
	}
	return peak, nil
}
