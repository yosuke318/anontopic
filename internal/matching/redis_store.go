package matching

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	queueKeyPrefix   = "matching:queue:"
	waitingKeyPrefix = "matching:waiting:"
	roomKeyPrefix    = "matching:room:"

	fieldTopicID        = "topic_id"
	fieldRoomType       = "room_type"
	fieldSince          = "since"
	fieldConversationID = "conversation_id"
	fieldStartedAt      = "started_at"

	// formingTTL is how long a room lives while its conversation is being
	// written to PostgreSQL. A server that dies in between leaves its
	// participants nothing to join, so the room disappears and they can queue
	// again instead of waiting for a conversation that will never come.
	formingTTL = 30 * time.Second
)

// enqueueScript places a token in a queue, refusing the tokens that are busy
// elsewhere. It answers with one of the codes below.
var enqueueScript = redis.NewScript(`
local waitingKey = KEYS[1]
local roomKey = KEYS[2]
local queueKey = KEYS[3]
local token = ARGV[1]
local topicID = ARGV[2]
local roomType = ARGV[3]
local now = ARGV[4]
local ttl = ARGV[5]

if redis.call('EXISTS', roomKey) == 1 then
	return 'matched'
end

local waiting = redis.call('HMGET', waitingKey, 'topic_id', 'room_type', 'since')
if waiting[1] then
	if waiting[1] ~= topicID or waiting[2] ~= roomType then
		return 'other_queue'
	end
	-- A repeated request keeps the place the token already holds, and puts it
	-- back if only the queue entry is gone.
	redis.call('ZADD', queueKey, 'NX', waiting[3], token)
	return 'waiting'
end

redis.call('HSET', waitingKey, 'topic_id', topicID, 'room_type', roomType, 'since', now)
redis.call('PEXPIRE', waitingKey, ttl)
redis.call('ZADD', queueKey, now, token)
return 'queued'
`)

// enqueueScript answers with one of these codes.
const (
	enqueueQueued     = "queued"
	enqueueWaiting    = "waiting"
	enqueueOtherQueue = "other_queue"
	enqueueMatched    = "matched"
)

// formRoomScript takes the participants of one room out of a queue and marks
// each of them as holding a room, in a single step so that two servers running
// it at the same time cannot hand the same token to both.
//
// It writes the keys of the participants it pops, which are not in KEYS
// because they are only known once the queue has been read. That is why the
// queues have to live on a single Redis node.
var formRoomScript = redis.NewScript(`
local queueKey = KEYS[1]
local size = tonumber(ARGV[1])
local timeoutBefore = ARGV[2]
local fallbackBefore = tonumber(ARGV[3])
local waitingPrefix = ARGV[4]
local roomPrefix = ARGV[5]
local formingTTL = ARGV[6]
local topicID = ARGV[7]

-- A token whose wait ran out leaves the queue. Its waiting key holds the same
-- lifetime and expires on its own.
redis.call('ZREMRANGEBYSCORE', queueKey, '-inf', timeoutBefore)

local waiting = redis.call('ZCARD', queueKey)
local take = 0
if waiting >= size then
	take = size
elseif size == 3 and waiting >= 2 then
	local oldest = redis.call('ZRANGE', queueKey, 0, 0, 'WITHSCORES')
	if tonumber(oldest[2]) <= fallbackBefore then
		take = 2
	end
end

if take == 0 then
	return {}
end

local popped = redis.call('ZPOPMIN', queueKey, take)
local participants = {}
for i = 1, #popped, 2 do
	local token = popped[i]
	participants[#participants + 1] = token
	redis.call('DEL', waitingPrefix .. token)
	redis.call('HSET', roomPrefix .. token,
		'topic_id', topicID, 'room_type', tostring(take), 'since', popped[i + 1])
	redis.call('PEXPIRE', roomPrefix .. token, formingTTL)
end
return participants
`)

// removeScript takes a token out of the queue its waiting key points at, so
// that a user who gives up leaves no entry behind for a room to be formed out
// of.
var removeScript = redis.NewScript(`
local waitingKey = KEYS[1]
local token = ARGV[1]
local queuePrefix = ARGV[2]

local waiting = redis.call('HMGET', waitingKey, 'topic_id', 'room_type')
if not waiting[1] then
	return 0
end

redis.call('DEL', waitingKey)
redis.call('ZREM', queuePrefix .. waiting[1] .. ':' .. waiting[2], token)
return 1
`)

// RedisStore keeps the queues in Redis, where every server reads the same ones.
//
// A queue is a sorted set whose members are session tokens and whose scores
// are the milliseconds at which each token started waiting, so the users of a
// queue come out of it in the order they arrived. Every timestamp this store
// writes is in milliseconds.
type RedisStore struct {
	client redis.UniversalClient
}

// NewRedisStore builds a Store on top of client.
func NewRedisStore(client redis.UniversalClient) *RedisStore {
	return &RedisStore{client: client}
}

// Enqueue puts token in the queue q names.
func (s *RedisStore) Enqueue(ctx context.Context, token string, q Queue, now time.Time, ttl time.Duration) error {
	keys := []string{waitingKey(token), roomKey(token), queueKey(q)}
	args := []any{
		token,
		strconv.Itoa(q.TopicID),
		strconv.Itoa(q.RoomType),
		strconv.FormatInt(now.UnixMilli(), 10),
		strconv.FormatInt(ttl.Milliseconds(), 10),
	}

	code, err := enqueueScript.Run(ctx, s.client, keys, args...).Text()
	if err != nil {
		return fmt.Errorf("enqueue: %w", err)
	}

	switch code {
	case enqueueQueued, enqueueWaiting:
		return nil
	case enqueueOtherQueue:
		return ErrAlreadyWaiting
	case enqueueMatched:
		return ErrAlreadyMatched
	default:
		return fmt.Errorf("enqueue: unknown answer %q", code)
	}
}

// FormRoom takes the participants of one room out of q.
func (s *RedisStore) FormRoom(ctx context.Context, q Queue, now time.Time, ttl, fallbackAfter time.Duration) ([]string, error) {
	args := []any{
		strconv.Itoa(q.RoomType),
		strconv.FormatInt(now.Add(-ttl).UnixMilli(), 10),
		strconv.FormatInt(now.Add(-fallbackAfter).UnixMilli(), 10),
		waitingKeyPrefix,
		roomKeyPrefix,
		strconv.FormatInt(formingTTL.Milliseconds(), 10),
		strconv.Itoa(q.TopicID),
	}

	participants, err := formRoomScript.Run(ctx, s.client, []string{queueKey(q)}, args...).StringSlice()
	if err != nil {
		return nil, fmt.Errorf("form room: %w", err)
	}
	return participants, nil
}

// Assign writes the conversation every participant is to join.
func (s *RedisStore) Assign(ctx context.Context, participants []string, conv Conversation, ttl time.Duration) error {
	pipe := s.client.TxPipeline()
	for _, token := range participants {
		key := roomKey(token)
		pipe.HSet(ctx, key,
			fieldTopicID, strconv.Itoa(conv.TopicID),
			fieldRoomType, strconv.Itoa(conv.RoomType),
			fieldConversationID, conv.ID,
			fieldStartedAt, strconv.FormatInt(conv.StartedAt.UnixMilli(), 10),
		)
		pipe.PExpire(ctx, key, ttl)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("assign room: %w", err)
	}
	return nil
}

// Discard drops the room of every participant.
func (s *RedisStore) Discard(ctx context.Context, participants []string) error {
	keys := make([]string, 0, len(participants))
	for _, token := range participants {
		keys = append(keys, roomKey(token))
	}

	if err := s.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("discard room: %w", err)
	}
	return nil
}

// Lookup reads the room a token holds, and the queue it waits in otherwise.
func (s *RedisStore) Lookup(ctx context.Context, token string) (State, error) {
	pipe := s.client.Pipeline()
	room := pipe.HGetAll(ctx, roomKey(token))
	waiting := pipe.HGetAll(ctx, waitingKey(token))

	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return State{}, fmt.Errorf("look up token: %w", err)
	}

	if fields := room.Val(); len(fields) > 0 {
		return roomState(fields)
	}
	if fields := waiting.Val(); len(fields) > 0 {
		return waitingState(fields)
	}
	return State{Kind: StateIdle}, nil
}

// Remove takes token out of the queue it waits in.
func (s *RedisStore) Remove(ctx context.Context, token string) error {
	args := []any{token, queueKeyPrefix}

	if err := removeScript.Run(ctx, s.client, []string{waitingKey(token)}, args...).Err(); err != nil {
		return fmt.Errorf("remove from queue: %w", err)
	}
	return nil
}

// roomState reads a room. A room whose conversation is not written yet leaves
// its participant waiting, because there is nothing for them to join.
func roomState(fields map[string]string) (State, error) {
	q, since, err := queueOf(fields)
	if err != nil {
		return State{}, err
	}

	if fields[fieldConversationID] == "" {
		return State{Kind: StateWaiting, Queue: q, WaitingSince: since}, nil
	}

	startedAt, err := parseMillis(fields[fieldStartedAt])
	if err != nil {
		return State{}, fmt.Errorf("parse %s of room: %w", fieldStartedAt, err)
	}

	return State{
		Kind:  StateMatched,
		Queue: q,
		Conversation: Conversation{
			ID:        fields[fieldConversationID],
			TopicID:   q.TopicID,
			RoomType:  q.RoomType,
			StartedAt: startedAt,
		},
	}, nil
}

// waitingState reads the queue entry of a token.
func waitingState(fields map[string]string) (State, error) {
	q, since, err := queueOf(fields)
	if err != nil {
		return State{}, err
	}
	return State{Kind: StateWaiting, Queue: q, WaitingSince: since}, nil
}

// queueOf reads the fields both a waiting key and a room key carry.
func queueOf(fields map[string]string) (Queue, time.Time, error) {
	topicID, err := strconv.Atoi(fields[fieldTopicID])
	if err != nil {
		return Queue{}, time.Time{}, fmt.Errorf("parse %s: %w", fieldTopicID, err)
	}

	roomType, err := strconv.Atoi(fields[fieldRoomType])
	if err != nil {
		return Queue{}, time.Time{}, fmt.Errorf("parse %s: %w", fieldRoomType, err)
	}

	since, err := parseMillis(fields[fieldSince])
	if err != nil {
		return Queue{}, time.Time{}, fmt.Errorf("parse %s: %w", fieldSince, err)
	}

	return Queue{TopicID: topicID, RoomType: roomType}, since, nil
}

// parseMillis reads a timestamp the way this store writes it. A sorted set
// score comes back as a float, so the value can carry a fractional part.
func parseMillis(v string) (time.Time, error) {
	millis, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.UnixMilli(int64(millis)).UTC(), nil
}

func queueKey(q Queue) string {
	return queueKeyPrefix + strconv.Itoa(q.TopicID) + ":" + strconv.Itoa(q.RoomType)
}

func waitingKey(token string) string { return waitingKeyPrefix + token }

func roomKey(token string) string { return roomKeyPrefix + token }
