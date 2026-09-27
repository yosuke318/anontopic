package report

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// The keys RedisSanctionStore writes, each followed by the identifier type
// and the identifier.
const (
	sanctionBanPrefix      = "sanction:ban:"
	sanctionBlockedPrefix  = "sanction:blocked:"
	sanctionReportedPrefix = "sanction:reported:"
	sanctionWarningPrefix  = "sanction:warning:"
)

// The values sanctionBanPrefix keys hold.
const (
	cachedBanned    = "1"
	cachedNotBanned = "0"
)

// RedisSanctionStore keeps the answers of the ban list, the offences counted
// towards the next sanction, and the warnings waiting to be shown in Redis.
//
// Offences are counted from the first one: the count and the window start
// together and are dropped together, which keeps each count a single key.
type RedisSanctionStore struct {
	client *redis.Client
	prefix string
}

// NewRedisSanctionStore builds a store on top of client.
func NewRedisSanctionStore(client *redis.Client) *RedisSanctionStore {
	return &RedisSanctionStore{client: client}
}

func (s *RedisSanctionStore) key(prefix, identifierType, identifier string) string {
	return s.prefix + prefix + identifierType + ":" + identifier
}

// CachedBan reads what CacheBan kept.
func (s *RedisSanctionStore) CachedBan(ctx context.Context, identifierType, identifier string) (bool, bool, error) {
	v, err := s.client.Get(ctx, s.key(sanctionBanPrefix, identifierType, identifier)).Result()
	if errors.Is(err, redis.Nil) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("get cached ban: %w", err)
	}
	return v == cachedBanned, true, nil
}

// CacheBan keeps whether the identifier is banned for ttl.
func (s *RedisSanctionStore) CacheBan(ctx context.Context, identifierType, identifier string, banned bool, ttl time.Duration) error {
	v := cachedNotBanned
	if banned {
		v = cachedBanned
	}
	if err := s.client.Set(ctx, s.key(sanctionBanPrefix, identifierType, identifier), v, ttl).Err(); err != nil {
		return fmt.Errorf("set cached ban: %w", err)
	}
	return nil
}

// ForgetBan drops what CacheBan kept.
func (s *RedisSanctionStore) ForgetBan(ctx context.Context, identifierType, identifier string) error {
	if err := s.client.Del(ctx, s.key(sanctionBanPrefix, identifierType, identifier)).Err(); err != nil {
		return fmt.Errorf("delete cached ban: %w", err)
	}
	return nil
}

// AddBlocked counts one blocked message.
func (s *RedisSanctionStore) AddBlocked(ctx context.Context, identifierType, identifier string, window time.Duration) (int, error) {
	key := s.key(sanctionBlockedPrefix, identifierType, identifier)

	var incr *redis.IntCmd
	_, err := s.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		incr = pipe.Incr(ctx, key)
		pipe.ExpireNX(ctx, key, window)
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("count blocked message: %w", err)
	}
	return int(incr.Val()), nil
}

// AddReported counts one conversation the identifier was reported in. A set
// of conversation IDs keeps a conversation reported twice from counting
// twice.
func (s *RedisSanctionStore) AddReported(ctx context.Context, identifierType, identifier, conversationID string, window time.Duration) (int, error) {
	key := s.key(sanctionReportedPrefix, identifierType, identifier)

	var card *redis.IntCmd
	_, err := s.client.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.SAdd(ctx, key, conversationID)
		pipe.ExpireNX(ctx, key, window)
		card = pipe.SCard(ctx, key)
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("count reported conversation: %w", err)
	}
	return int(card.Val()), nil
}

// ResetOffences drops both counts.
func (s *RedisSanctionStore) ResetOffences(ctx context.Context, identifierType, identifier string) error {
	err := s.client.Del(ctx,
		s.key(sanctionBlockedPrefix, identifierType, identifier),
		s.key(sanctionReportedPrefix, identifierType, identifier)).Err()
	if err != nil {
		return fmt.Errorf("delete offences: %w", err)
	}
	return nil
}

// MarkWarning keeps a warning to be shown.
func (s *RedisSanctionStore) MarkWarning(ctx context.Context, identifierType, identifier string, ttl time.Duration) error {
	if err := s.client.Set(ctx, s.key(sanctionWarningPrefix, identifierType, identifier), "1", ttl).Err(); err != nil {
		return fmt.Errorf("set warning: %w", err)
	}
	return nil
}

// TakeWarning reads and drops a waiting warning in one step, so that two
// connections of one client do not both show it.
func (s *RedisSanctionStore) TakeWarning(ctx context.Context, identifierType, identifier string) (bool, error) {
	err := s.client.GetDel(ctx, s.key(sanctionWarningPrefix, identifierType, identifier)).Err()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("take warning: %w", err)
	}
	return true, nil
}
