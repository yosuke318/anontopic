package chat

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	roomChannelPrefix = "chat:room:"
	presenceKeyPrefix = "chat:presence:"

	// subscriptionBuffer is how many events one room's subscription holds
	// while its connections are being written to.
	subscriptionBuffer = 64
)

// presenceScript records that a participant is connected to a room, or that
// they are not, and answers with everyone connected to it.
//
// The participants of a room are a sorted set whose members are session
// tokens and whose scores are the milliseconds at which each connection last
// said it was still there. A token whose entry is older than the lifetime a
// connection keeps it for is dropped, so a server that died stops counting
// towards the room it was holding connections to.
var presenceScript = redis.NewScript(`
local key = KEYS[1]
local token = ARGV[1]
local now = ARGV[2]
local staleBefore = ARGV[3]
local ttl = ARGV[4]
local connected = ARGV[5]

redis.call('ZREMRANGEBYSCORE', key, '-inf', staleBefore)

if connected == '1' then
	redis.call('ZADD', key, now, token)
	redis.call('PEXPIRE', key, ttl)
else
	redis.call('ZREM', key, token)
end

return redis.call('ZRANGE', key, 0, -1)
`)

// The values presenceScript reads as its last argument.
const (
	presenceConnected    = "1"
	presenceDisconnected = "0"
)

// RedisStore carries room events over Redis Pub/Sub and keeps the
// participants connected to a room in Redis, where every server reads the
// same ones. Every timestamp it writes is in milliseconds.
type RedisStore struct {
	client redis.UniversalClient
}

// NewRedisStore builds a Store on top of client.
func NewRedisStore(client redis.UniversalClient) *RedisStore {
	return &RedisStore{client: client}
}

// Publish hands payload to every server subscribed to the room.
func (s *RedisStore) Publish(ctx context.Context, conversationID string, payload []byte) error {
	if err := s.client.Publish(ctx, roomChannel(conversationID), payload).Err(); err != nil {
		return fmt.Errorf("publish room event: %w", err)
	}
	return nil
}

// Subscribe opens this server's stream of one room's events.
func (s *RedisStore) Subscribe(ctx context.Context, conversationID string) (Subscription, error) {
	pubsub := s.client.Subscribe(ctx, roomChannel(conversationID))

	// Waiting for Redis to confirm the subscription keeps the events
	// published right after this call from being missed.
	if _, err := pubsub.Receive(ctx); err != nil {
		_ = pubsub.Close()
		return nil, fmt.Errorf("subscribe to room: %w", err)
	}

	sub := &redisSubscription{
		pubsub: pubsub,
		events: make(chan []byte, subscriptionBuffer),
		done:   make(chan struct{}),
	}
	go sub.run()

	return sub, nil
}

// Join records token as connected to the room.
func (s *RedisStore) Join(ctx context.Context, conversationID, token string, now time.Time, ttl time.Duration) ([]string, error) {
	return s.presence(ctx, conversationID, token, now, ttl, presenceConnected)
}

// Heartbeat keeps token connected to the room for another ttl.
func (s *RedisStore) Heartbeat(ctx context.Context, conversationID, token string, now time.Time, ttl time.Duration) ([]string, error) {
	return s.presence(ctx, conversationID, token, now, ttl, presenceConnected)
}

// Leave drops token from the participants connected to the room.
func (s *RedisStore) Leave(ctx context.Context, conversationID, token string, now time.Time, ttl time.Duration) ([]string, error) {
	return s.presence(ctx, conversationID, token, now, ttl, presenceDisconnected)
}

// presence runs presenceScript and returns the tokens connected to the room.
func (s *RedisStore) presence(ctx context.Context, conversationID, token string, now time.Time, ttl time.Duration, connected string) ([]string, error) {
	args := []any{
		token,
		strconv.FormatInt(now.UnixMilli(), 10),
		strconv.FormatInt(now.Add(-ttl).UnixMilli(), 10),
		strconv.FormatInt(ttl.Milliseconds(), 10),
		connected,
	}

	tokens, err := presenceScript.Run(ctx, s.client, []string{presenceKey(conversationID)}, args...).StringSlice()
	if err != nil {
		return nil, fmt.Errorf("read the participants connected to the room: %w", err)
	}
	return tokens, nil
}

// redisSubscription turns one Redis subscription into a stream of payloads.
type redisSubscription struct {
	pubsub    *redis.PubSub
	events    chan []byte
	done      chan struct{}
	closeOnce sync.Once
}

// run copies what Redis delivers into the stream the room reads.
func (s *redisSubscription) run() {
	defer close(s.events)

	for msg := range s.pubsub.Channel() {
		select {
		case s.events <- []byte(msg.Payload):
		case <-s.done:
			return
		}
	}
}

// Events yields what is published for the room.
func (s *redisSubscription) Events() <-chan []byte { return s.events }

// Close ends the subscription.
func (s *redisSubscription) Close() error {
	s.closeOnce.Do(func() { close(s.done) })

	if err := s.pubsub.Close(); err != nil {
		return fmt.Errorf("close room subscription: %w", err)
	}
	return nil
}

func roomChannel(conversationID string) string { return roomChannelPrefix + conversationID }

func presenceKey(conversationID string) string { return presenceKeyPrefix + conversationID }
