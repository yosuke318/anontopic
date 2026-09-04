// Package capacity owns the limits the service refuses work at: how many
// WebSocket connections it holds at once, how many of them come from one
// hashed address, and how often a session may send a message or ask to be
// matched.
//
// The connection count is kept in Redis so that every server counts against
// the same total. A connection holds its place for a lease, renews the lease
// while it is open, and gives it back when it closes; a place whose lease ran
// out stops being counted, so a server that died does not hold the total up.
// The reasoning is in docs/adr/0013-cap-connections-with-leases-in-redis.md.
//
// The rates are token buckets in Redis, one per subject, so a client that
// stops for a moment gets its allowance back and a client that keeps pushing
// is held to a steady rate.
//
// Boundary: other modules reach these limits through interfaces they declare
// themselves, and never read this module's Redis keys.
package capacity

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	// DefaultConnections is the number of WebSocket connections the service
	// holds at once. The cost the service is built for is quoted at this
	// number of connections (設計書 2 / 5).
	DefaultConnections = 1000

	// DefaultPerIPHash is how many of those connections one hashed address
	// may hold. It leaves room for the several tabs and the several people a
	// household or an office shares one address with.
	DefaultPerIPHash = 5

	// DefaultLeaseTTL is how long a connection stays counted without renewing
	// its lease. It outlasts several renewals, so that a server under load
	// does not lose the places it is holding.
	DefaultLeaseTTL = 30 * time.Second

	// DefaultRenewInterval is how often an open connection renews its lease.
	DefaultRenewInterval = 10 * time.Second

	// renewalsPerLease is how many times a connection renews its place before
	// the lease on it would run out. A renewal that is late or lost has to be
	// able to happen again before an open connection drops out of the count,
	// which is what keeps the next connection from being let past the cap.
	renewalsPerLease = 3

	// leaseIDBytes sizes the identifier one connection holds its place under.
	leaseIDBytes = 16

	// releaseTimeout bounds giving a place back, which happens after the
	// request that took it is over and cannot bound it.
	releaseTimeout = 5 * time.Second
)

// The names the buckets of each rate are kept under.
const (
	rateMessage = "message"
	rateMatch   = "match"
)

var (
	// DefaultMessageLimit is how fast one session may send messages: a few in
	// a row, and then one per second.
	DefaultMessageLimit = Limit{Burst: 5, Interval: time.Second}

	// DefaultMatchLimit is how often one hashed address may ask to be
	// matched: a few tries in a row, and then one per ten seconds.
	DefaultMatchLimit = Limit{Burst: 3, Interval: 10 * time.Second}
)

var (
	// ErrAtCapacity is returned when the service already holds every
	// connection it is allowed to hold.
	ErrAtCapacity error = &refusal{
		message:    "capacity: the service holds every connection it is allowed to",
		atCapacity: true,
	}

	// ErrTooManyPerIPHash is returned when one hashed address already holds
	// every connection it is allowed to.
	ErrTooManyPerIPHash error = &refusal{
		message: "capacity: too many connections from one address",
	}
)

// refusal is a connection a limit left no room for. It answers which limit
// that was through AtCapacity, so that a caller can tell the two apart
// without naming this package: a caller declares the interface it reads the
// answer through and matches it with errors.As.
type refusal struct {
	message    string
	atCapacity bool
}

func (r *refusal) Error() string { return r.message }

// AtCapacity reports whether the limit that was reached is the number of
// connections the service holds, rather than the number one hashed address
// may hold.
func (r *refusal) AtCapacity() bool { return r.atCapacity }

// Limit is how often something may be done: Burst of them in a row, and one
// more every Interval once that is spent.
type Limit struct {
	Burst    int
	Interval time.Duration
}

// valid reports whether the limit lets anything through at all.
func (l Limit) valid() bool { return l.Burst > 0 && l.Interval > 0 }

// Service decides whether a connection is admitted and whether an action is
// within its rate.
type Service struct {
	store Store

	limits        Limits
	leaseTTL      time.Duration
	renewInterval time.Duration
	message       Limit
	match         Limit
	now           func() time.Time
}

// Options configures a Service. The zero value of each field selects the
// default described on the field.
type Options struct {
	// Connections defaults to DefaultConnections.
	Connections int
	// PerIPHash defaults to DefaultPerIPHash.
	PerIPHash int
	// LeaseTTL defaults to DefaultLeaseTTL.
	LeaseTTL time.Duration
	// RenewInterval defaults to DefaultRenewInterval. An interval that does
	// not leave room for renewalsPerLease renewals within LeaseTTL is
	// shortened until it does.
	RenewInterval time.Duration
	// Message defaults to DefaultMessageLimit.
	Message Limit
	// Match defaults to DefaultMatchLimit.
	Match Limit
}

// NewService builds a Service.
func NewService(store Store, opts Options) *Service {
	if opts.Connections <= 0 {
		opts.Connections = DefaultConnections
	}
	if opts.PerIPHash <= 0 {
		opts.PerIPHash = DefaultPerIPHash
	}
	if opts.LeaseTTL <= 0 {
		opts.LeaseTTL = DefaultLeaseTTL
	}
	if opts.RenewInterval <= 0 {
		opts.RenewInterval = DefaultRenewInterval
	}
	// A place is renewed several times over before its lease runs out. An
	// interval that does not fit under the lease would let an open connection
	// drop out of the count between two renewals, and the cap would be read
	// against a total lower than the connections actually held.
	if opts.RenewInterval*renewalsPerLease > opts.LeaseTTL {
		fits := max(opts.LeaseTTL/renewalsPerLease, time.Millisecond)
		slog.Warn("the renew interval does not fit under the lease, shortening it",
			slog.Duration("configured", opts.RenewInterval),
			slog.Duration("lease_ttl", opts.LeaseTTL),
			slog.Duration("renew_interval", fits))
		opts.RenewInterval = fits
	}

	if !opts.Message.valid() {
		opts.Message = DefaultMessageLimit
	}
	if !opts.Match.valid() {
		opts.Match = DefaultMatchLimit
	}

	return &Service{
		store:         store,
		limits:        Limits{Connections: opts.Connections, PerIPHash: opts.PerIPHash},
		leaseTTL:      opts.LeaseTTL,
		renewInterval: opts.RenewInterval,
		message:       opts.Message,
		match:         opts.Match,
		now:           time.Now,
	}
}

// Acquire counts one connection held by ipHash and returns the function that
// gives its place back. The place is renewed for as long as it is held, so
// the returned function has to be called once the connection is closed.
//
// It returns ErrAtCapacity when the service holds every connection it is
// allowed to, and ErrTooManyPerIPHash when the address does.
func (s *Service) Acquire(ctx context.Context, ipHash string) (func(), error) {
	id, err := newLeaseID()
	if err != nil {
		return nil, err
	}

	got, err := s.store.Acquire(ctx, id, ipHash, s.now().UTC(), s.leaseTTL, s.limits)
	if err != nil {
		return nil, err
	}

	switch got.Outcome {
	case AtCapacity:
		slog.Warn("refusing a connection because the service is at its limit",
			slog.Int("connections", got.Connections),
			slog.Int("limit", s.limits.Connections))
		return nil, ErrAtCapacity
	case TooManyPerIPHash:
		return nil, ErrTooManyPerIPHash
	}

	l := &lease{svc: s, id: id, ipHash: ipHash, done: make(chan struct{})}
	// The lease outlives the request that took it: the connection it counts
	// is served after the handler has returned.
	go l.renewLoop(context.WithoutCancel(ctx))

	return l.release, nil
}

// Connections is how many connections are counted right now. It reads the
// count a load test checks the cap against.
func (s *Service) Connections(ctx context.Context) (int, error) {
	return s.store.Connections(ctx, s.now().UTC(), s.leaseTTL)
}

// AllowMessage reports whether subject may send another message now.
func (s *Service) AllowMessage(ctx context.Context, subject string) (bool, error) {
	return s.store.Take(ctx, rateMessage, subject, s.message, s.now().UTC())
}

// AllowMatch reports whether subject may ask to be matched again now, and how
// long it has to wait when it may not. The wait is the interval the bucket
// earns a token over, which is the soonest the next request can go through.
func (s *Service) AllowMatch(ctx context.Context, subject string) (bool, time.Duration, error) {
	allowed, err := s.store.Take(ctx, rateMatch, subject, s.match, s.now().UTC())
	if err != nil || allowed {
		return allowed, 0, err
	}
	return false, s.match.Interval, nil
}

// lease is one connection's place in the count.
type lease struct {
	svc    *Service
	id     string
	ipHash string

	done      chan struct{}
	closeOnce sync.Once
}

// release gives the place back and stops renewing it.
func (l *lease) release() {
	l.closeOnce.Do(func() {
		close(l.done)

		ctx, cancel := context.WithTimeout(context.Background(), releaseTimeout)
		defer cancel()

		if err := l.svc.store.Release(ctx, l.id, l.ipHash); err != nil {
			// The lease runs out on its own, so the place comes back either
			// way; until it does the service holds one connection fewer than
			// it is allowed to.
			slog.Error("release a connection's place", slog.Any("error", err))
		}
	})
}

// renewLoop keeps the place counted for as long as the connection is open.
func (l *lease) renewLoop(ctx context.Context) {
	ticker := time.NewTicker(l.svc.renewInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			err := l.svc.store.Renew(ctx, l.id, l.ipHash, l.svc.now().UTC(), l.svc.leaseTTL)
			if err != nil {
				slog.Error("renew a connection's place", slog.Any("error", err))
			}
		case <-l.done:
			return
		}
	}
}

// newLeaseID draws the identifier a connection holds its place under.
func newLeaseID() (string, error) {
	buf := make([]byte, leaseIDBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
