package capacity

import (
	"context"
	"time"
)

// Limits are the caps a connection is counted against.
type Limits struct {
	// Connections is how many connections every server together may hold.
	Connections int
	// PerIPHash is how many of them one hashed address may hold.
	PerIPHash int
}

// Outcome is what a Store decided about one connection.
type Outcome int

const (
	// Granted counts the connection: both caps had room for it.
	Granted Outcome = iota
	// AtCapacity refuses it: every server together holds Limits.Connections.
	AtCapacity
	// TooManyPerIPHash refuses it: the address holds Limits.PerIPHash.
	TooManyPerIPHash
)

// Acquisition is what a Store decided, together with what it counted.
type Acquisition struct {
	Outcome Outcome
	// Connections is how many connections are counted, and PerIPHash how many
	// of them the address holds. Both are read after the decision, so a
	// granted connection is included in them.
	Connections int
	PerIPHash   int
}

// Store counts the connections every server holds and keeps one token bucket
// per rate and subject. Every server reads the same counts, which is what
// makes the caps hold across servers.
type Store interface {
	// Acquire counts id as a connection held by ipHash for ttl, unless
	// limits leave no room for it. A place that has not been renewed within
	// the last ttl is dropped first, an entry exactly that old included,
	// which is what a server that died leaves behind.
	Acquire(ctx context.Context, id, ipHash string, now time.Time, ttl time.Duration, limits Limits) (Acquisition, error)

	// Renew keeps id counted for another ttl. A place that was already
	// dropped is counted again, because the connection behind it is open.
	Renew(ctx context.Context, id, ipHash string, now time.Time, ttl time.Duration) error

	// Release stops counting id. Releasing a place that is already gone is
	// not an error.
	Release(ctx context.Context, id, ipHash string) error

	// Connections is how many places are counted, stale ones dropped.
	Connections(ctx context.Context, now time.Time, ttl time.Duration) (int, error)

	// Take spends one token of subject's bucket for the rate named name, and
	// reports whether there was one to spend. A bucket nobody has spent from
	// for long enough to refill is forgotten.
	Take(ctx context.Context, name, subject string, limit Limit, now time.Time) (bool, error)
}
