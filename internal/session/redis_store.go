package session

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	sessionKeyPrefix = "session:"
	ipIndexKeyPrefix = "session_ip:"

	fieldIPHash   = "ip_hash"
	fieldIssuedAt = "issued_at"
)

// RedisStore keeps sessions in Redis and lets its TTL express the idle window.
type RedisStore struct {
	client redis.UniversalClient
}

// NewRedisStore builds a Store on top of client.
func NewRedisStore(client redis.UniversalClient) *RedisStore {
	return &RedisStore{client: client}
}

// Create writes the record and adds the token to the index of its address.
func (s *RedisStore) Create(ctx context.Context, token string, rec Record, ttl time.Duration) error {
	sessionKey := sessionKeyPrefix + token
	indexKey := ipIndexKeyPrefix + rec.IPHash

	pipe := s.client.TxPipeline()
	pipe.HSet(ctx, sessionKey,
		fieldIPHash, rec.IPHash,
		fieldIssuedAt, strconv.FormatInt(rec.IssuedAt.UnixNano(), 10),
	)
	pipe.Expire(ctx, sessionKey, ttl)
	pipe.SAdd(ctx, indexKey, token)
	// The index only has to outlive the sessions it points at. Members whose
	// session already expired are skipped on delete.
	pipe.Expire(ctx, indexKey, ttl)

	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// Get reads the record stored under token.
func (s *RedisStore) Get(ctx context.Context, token string) (Record, error) {
	fields, err := s.client.HGetAll(ctx, sessionKeyPrefix+token).Result()
	if err != nil {
		return Record{}, fmt.Errorf("get session: %w", err)
	}
	if len(fields) == 0 {
		return Record{}, ErrNotStored
	}

	nanos, err := strconv.ParseInt(fields[fieldIssuedAt], 10, 64)
	if err != nil {
		return Record{}, fmt.Errorf("parse %s of session: %w", fieldIssuedAt, err)
	}

	return Record{
		IPHash:   fields[fieldIPHash],
		IssuedAt: time.Unix(0, nanos).UTC(),
	}, nil
}

// Refresh moves the expiry of an existing session.
func (s *RedisStore) Refresh(ctx context.Context, token string, ttl time.Duration) error {
	ok, err := s.client.Expire(ctx, sessionKeyPrefix+token, ttl).Result()
	if err != nil {
		return fmt.Errorf("refresh session: %w", err)
	}
	if !ok {
		return ErrNotStored
	}
	return nil
}

// Delete removes the session and its index entry.
func (s *RedisStore) Delete(ctx context.Context, token string) error {
	sessionKey := sessionKeyPrefix + token

	ipHash, err := s.client.HGet(ctx, sessionKey, fieldIPHash).Result()
	switch {
	case errors.Is(err, redis.Nil):
		return nil
	case err != nil:
		return fmt.Errorf("read %s of session: %w", fieldIPHash, err)
	}

	pipe := s.client.TxPipeline()
	pipe.Del(ctx, sessionKey)
	pipe.SRem(ctx, ipIndexKeyPrefix+ipHash, token)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// DeleteByIPHash removes every session the index holds for ipHash.
func (s *RedisStore) DeleteByIPHash(ctx context.Context, ipHash string) (int, error) {
	indexKey := ipIndexKeyPrefix + ipHash

	tokens, err := s.client.SMembers(ctx, indexKey).Result()
	if err != nil {
		return 0, fmt.Errorf("read session index: %w", err)
	}

	deleted := 0
	if len(tokens) > 0 {
		keys := make([]string, 0, len(tokens))
		for _, token := range tokens {
			keys = append(keys, sessionKeyPrefix+token)
		}

		n, err := s.client.Del(ctx, keys...).Result()
		if err != nil {
			return 0, fmt.Errorf("delete sessions of ip hash: %w", err)
		}
		// Tokens whose session already expired stay in the index, so the
		// count comes from the keys Redis actually removed.
		deleted = int(n)
	}

	if err := s.client.Del(ctx, indexKey).Err(); err != nil {
		return deleted, fmt.Errorf("delete session index: %w", err)
	}
	return deleted, nil
}
