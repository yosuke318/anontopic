package session

import (
	"context"
	"errors"
	"time"
)

// ErrNotStored is what a Store returns when a token has no record, either
// because it never existed or because its TTL ran out.
var ErrNotStored = errors.New("session: token not stored")

// Record is the state a Store keeps for one token.
type Record struct {
	IPHash   string
	IssuedAt time.Time
}

// Store keeps sessions until their TTL expires or they are deleted.
//
// Implementations index records by IPHash as well, so that a ban can end every
// session of one address in a single call.
type Store interface {
	// Create stores rec under token for ttl.
	Create(ctx context.Context, token string, rec Record, ttl time.Duration) error
	// Get returns the record stored under token, or ErrNotStored.
	Get(ctx context.Context, token string) (Record, error)
	// Refresh sets the remaining lifetime of an existing token to ttl and
	// returns ErrNotStored if the token is already gone.
	Refresh(ctx context.Context, token string, ttl time.Duration) error
	// Delete removes token. Deleting an absent token is not an error.
	Delete(ctx context.Context, token string) error
	// DeleteByIPHash removes every session issued to ipHash and returns how
	// many were removed.
	DeleteByIPHash(ctx context.Context, ipHash string) (int, error)
}
