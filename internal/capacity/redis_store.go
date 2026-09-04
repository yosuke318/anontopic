package capacity

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// keyPrefix starts every key this store writes.
const keyPrefix = "capacity:"

// The keys the store writes, each under keyPrefix: the connections every
// server holds, those of one hashed address, and one rate's bucket for one
// subject.
const (
	connectionsKey   = "connections"
	perIPHashPrefix  = "connections:ip:"
	rateBucketPrefix = "rate:"
)

// The values acquireScript answers with, read as an Outcome.
const (
	scriptGranted          = 0
	scriptAtCapacity       = 1
	scriptTooManyPerIPHash = 2
)

// acquireScript counts one connection unless a cap leaves no room for it, and
// answers with the outcome and both counts.
//
// The connections are a sorted set whose members are lease identifiers and
// whose scores are the milliseconds at which each lease was last renewed. One
// set holds every connection and one holds those of a single hashed address.
// A lease older than the lifetime it is renewed for is dropped before the
// caps are read, so a server that died stops holding places it cannot serve.
var acquireScript = redis.NewScript(`
local total = KEYS[1]
local perIP = KEYS[2]
local id = ARGV[1]
local now = ARGV[2]
local staleBefore = ARGV[3]
local ttl = ARGV[4]
local maxTotal = tonumber(ARGV[5])
local maxPerIP = tonumber(ARGV[6])

redis.call('ZREMRANGEBYSCORE', total, '-inf', staleBefore)
redis.call('ZREMRANGEBYSCORE', perIP, '-inf', staleBefore)

local held = redis.call('ZCARD', total)
local heldByIP = redis.call('ZCARD', perIP)

if held >= maxTotal then
	return {1, held, heldByIP}
end
if heldByIP >= maxPerIP then
	return {2, held, heldByIP}
end

redis.call('ZADD', total, now, id)
redis.call('PEXPIRE', total, ttl)
redis.call('ZADD', perIP, now, id)
redis.call('PEXPIRE', perIP, ttl)

return {0, held + 1, heldByIP + 1}
`)

// renewScript keeps one connection counted for another lifetime. It counts a
// lease that was already dropped again, because the connection behind it is
// open and cutting it off frees nothing.
var renewScript = redis.NewScript(`
local total = KEYS[1]
local perIP = KEYS[2]
local id = ARGV[1]
local now = ARGV[2]
local staleBefore = ARGV[3]
local ttl = ARGV[4]

redis.call('ZREMRANGEBYSCORE', total, '-inf', staleBefore)
redis.call('ZREMRANGEBYSCORE', perIP, '-inf', staleBefore)

redis.call('ZADD', total, now, id)
redis.call('PEXPIRE', total, ttl)
redis.call('ZADD', perIP, now, id)
redis.call('PEXPIRE', perIP, ttl)

return redis.call('ZCARD', total)
`)

// releaseScript stops counting one connection. A set the last member is
// removed from is deleted by Redis itself.
var releaseScript = redis.NewScript(`
redis.call('ZREM', KEYS[1], ARGV[1])
redis.call('ZREM', KEYS[2], ARGV[1])
return redis.status_reply('OK')
`)

// takeScript spends one token of a bucket and answers whether there was one.
//
// A bucket holds the tokens left and the millisecond they were counted at.
// The tokens earned since then are added before one is spent, up to the burst
// the bucket holds, so a subject that stops for a while starts full again.
var takeScript = redis.NewScript(`
local key = KEYS[1]
local burst = tonumber(ARGV[1])
local intervalMs = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local ttl = ARGV[4]

local bucket = redis.call('HMGET', key, 'tokens', 'at')
local tokens = tonumber(bucket[1])
local at = tonumber(bucket[2])

if tokens == nil or at == nil then
	tokens = burst
	at = now
end

tokens = math.min(burst, tokens + (now - at) / intervalMs)

local allowed = 0
if tokens >= 1 then
	tokens = tokens - 1
	allowed = 1
end

redis.call('HSET', key, 'tokens', tostring(tokens), 'at', ARGV[3])
redis.call('PEXPIRE', key, ttl)

return allowed
`)

// RedisStore counts connections and holds the rate buckets in Redis, where
// every server reads the same ones. Every timestamp it writes is in
// milliseconds.
type RedisStore struct {
	client redis.UniversalClient
	prefix string
}

// NewRedisStore builds a Store on top of client.
func NewRedisStore(client redis.UniversalClient) *RedisStore {
	return &RedisStore{client: client, prefix: keyPrefix}
}

// Acquire counts one connection unless a cap leaves no room for it.
func (s *RedisStore) Acquire(ctx context.Context, id, ipHash string, now time.Time, ttl time.Duration, limits Limits) (Acquisition, error) {
	args := []any{
		id,
		millis(now),
		millis(now.Add(-ttl)),
		strconv.FormatInt(ttl.Milliseconds(), 10),
		strconv.Itoa(limits.Connections),
		strconv.Itoa(limits.PerIPHash),
	}

	answer, err := acquireScript.Run(ctx, s.client, s.keys(ipHash), args...).Int64Slice()
	if err != nil {
		return Acquisition{}, fmt.Errorf("count the connection: %w", err)
	}
	if len(answer) != 3 {
		return Acquisition{}, fmt.Errorf("count the connection: read %d values, want 3", len(answer))
	}

	acq := Acquisition{Connections: int(answer[1]), PerIPHash: int(answer[2])}
	switch answer[0] {
	case scriptGranted:
		acq.Outcome = Granted
	case scriptAtCapacity:
		acq.Outcome = AtCapacity
	case scriptTooManyPerIPHash:
		acq.Outcome = TooManyPerIPHash
	default:
		return Acquisition{}, fmt.Errorf("count the connection: unknown outcome %d", answer[0])
	}

	return acq, nil
}

// Renew keeps one connection counted for another ttl.
func (s *RedisStore) Renew(ctx context.Context, id, ipHash string, now time.Time, ttl time.Duration) error {
	args := []any{
		id,
		millis(now),
		millis(now.Add(-ttl)),
		strconv.FormatInt(ttl.Milliseconds(), 10),
	}

	if err := renewScript.Run(ctx, s.client, s.keys(ipHash), args...).Err(); err != nil {
		return fmt.Errorf("keep the connection counted: %w", err)
	}
	return nil
}

// Release stops counting one connection.
func (s *RedisStore) Release(ctx context.Context, id, ipHash string) error {
	if err := releaseScript.Run(ctx, s.client, s.keys(ipHash), id).Err(); err != nil {
		return fmt.Errorf("stop counting the connection: %w", err)
	}
	return nil
}

// Connections is how many connections are counted, stale places dropped.
func (s *RedisStore) Connections(ctx context.Context, now time.Time, ttl time.Duration) (int, error) {
	key := s.prefix + connectionsKey

	if err := s.client.ZRemRangeByScore(ctx, key, "-inf", millis(now.Add(-ttl))).Err(); err != nil {
		return 0, fmt.Errorf("drop the connections that were not renewed: %w", err)
	}

	held, err := s.client.ZCard(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("count the connections held: %w", err)
	}
	return int(held), nil
}

// Take spends one token of a subject's bucket.
func (s *RedisStore) Take(ctx context.Context, name, subject string, limit Limit, now time.Time) (bool, error) {
	// A bucket that has had the time to refill completely says the same thing
	// as one that was never there, so it does not have to be kept any longer.
	ttl := time.Duration(limit.Burst) * limit.Interval

	args := []any{
		strconv.Itoa(limit.Burst),
		strconv.FormatFloat(float64(limit.Interval)/float64(time.Millisecond), 'f', -1, 64),
		millis(now),
		strconv.FormatInt(ttl.Milliseconds(), 10),
	}

	allowed, err := takeScript.Run(ctx, s.client, []string{s.rateKey(name, subject)}, args...).Int64()
	if err != nil {
		return false, fmt.Errorf("spend a token of the %s rate: %w", name, err)
	}
	return allowed == 1, nil
}

// keys are the sets one connection is counted in: every connection, and those
// of its hashed address.
func (s *RedisStore) keys(ipHash string) []string {
	return []string{s.prefix + connectionsKey, s.prefix + perIPHashPrefix + ipHash}
}

func (s *RedisStore) rateKey(name, subject string) string {
	return s.prefix + rateBucketPrefix + name + ":" + subject
}

func millis(t time.Time) string {
	return strconv.FormatInt(t.UnixMilli(), 10)
}
