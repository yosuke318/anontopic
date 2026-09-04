package capacity

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeStore counts connections in memory the way the Redis store counts them
// in Redis: a lease that has not been renewed within its lifetime stops being
// counted.
type fakeStore struct {
	mu sync.Mutex
	// held maps a lease identifier to the address holding it and the moment
	// the lease was last renewed at.
	held map[string]leaseRecord
	// taken records every rate a token was spent from, in order.
	taken []takeCall
	// allow is the answer Take gives.
	allow bool
	// err is what every call answers with when it is set.
	err error
}

type leaseRecord struct {
	ipHash    string
	renewedAt time.Time
}

// takeCall is one call to Take.
type takeCall struct {
	name    string
	subject string
	limit   Limit
}

func newFakeStore() *fakeStore {
	return &fakeStore{held: make(map[string]leaseRecord), allow: true}
}

func (s *fakeStore) Acquire(_ context.Context, id, ipHash string, now time.Time, ttl time.Duration, limits Limits) (Acquisition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.err != nil {
		return Acquisition{}, s.err
	}
	s.dropStale(now, ttl)

	held, heldByIP := s.counts(ipHash)
	if held >= limits.Connections {
		return Acquisition{Outcome: AtCapacity, Connections: held, PerIPHash: heldByIP}, nil
	}
	if heldByIP >= limits.PerIPHash {
		return Acquisition{Outcome: TooManyPerIPHash, Connections: held, PerIPHash: heldByIP}, nil
	}

	s.held[id] = leaseRecord{ipHash: ipHash, renewedAt: now}
	return Acquisition{Outcome: Granted, Connections: held + 1, PerIPHash: heldByIP + 1}, nil
}

func (s *fakeStore) Renew(_ context.Context, id, ipHash string, now time.Time, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.err != nil {
		return s.err
	}
	s.dropStale(now, ttl)
	s.held[id] = leaseRecord{ipHash: ipHash, renewedAt: now}

	return nil
}

func (s *fakeStore) Release(_ context.Context, id, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.held, id)
	return nil
}

func (s *fakeStore) Connections(_ context.Context, now time.Time, ttl time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.err != nil {
		return 0, s.err
	}
	s.dropStale(now, ttl)

	return len(s.held), nil
}

func (s *fakeStore) Take(_ context.Context, name, subject string, limit Limit, _ time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.err != nil {
		return false, s.err
	}
	s.taken = append(s.taken, takeCall{name: name, subject: subject, limit: limit})

	return s.allow, nil
}

// dropStale forgets the leases that were not renewed within the last ttl. The
// Redis store drops a lease that is exactly ttl old, so this one does too.
func (s *fakeStore) dropStale(now time.Time, ttl time.Duration) {
	for id, rec := range s.held {
		if !rec.renewedAt.After(now.Add(-ttl)) {
			delete(s.held, id)
		}
	}
}

// counts is how many connections are held in total and by one address.
func (s *fakeStore) counts(ipHash string) (int, int) {
	byIP := 0
	for _, rec := range s.held {
		if rec.ipHash == ipHash {
			byIP++
		}
	}
	return len(s.held), byIP
}

func (s *fakeStore) calls() []takeCall {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]takeCall(nil), s.taken...)
}

func TestAcquireRefusesTheConnectionThatWouldPassTheLimit(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, Options{Connections: 2, PerIPHash: 2})

	for i := range 2 {
		if _, err := svc.Acquire(t.Context(), "hash-of-a-network"); err != nil {
			t.Fatalf("connection %d: %v", i+1, err)
		}
	}

	// The third connection is one more than the service may hold, whatever
	// address it comes from.
	_, err := svc.Acquire(t.Context(), "hash-of-another-network")
	if !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("err = %v, want %v", err, ErrAtCapacity)
	}

	held, err := svc.Connections(t.Context())
	if err != nil {
		t.Fatalf("Connections: %v", err)
	}
	if held != 2 {
		t.Fatalf("connections = %d, want 2", held)
	}
}

func TestAcquireRefusesAnAddressHoldingItsShare(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, Options{Connections: 10, PerIPHash: 2})

	for i := range 2 {
		if _, err := svc.Acquire(t.Context(), "one-network"); err != nil {
			t.Fatalf("connection %d: %v", i+1, err)
		}
	}

	_, err := svc.Acquire(t.Context(), "one-network")
	if !errors.Is(err, ErrTooManyPerIPHash) {
		t.Fatalf("err = %v, want %v", err, ErrTooManyPerIPHash)
	}

	// The service is nowhere near its own limit, so another address connects.
	if _, err := svc.Acquire(t.Context(), "another-network"); err != nil {
		t.Fatalf("another network: %v", err)
	}
}

// A refusal says which limit was reached without the caller naming this
// package, which is how the modules behind an interface read it.
func TestARefusalSaysWhichLimitWasReached(t *testing.T) {
	type refused interface {
		error
		AtCapacity() bool
	}

	cases := map[string]struct {
		err  error
		want bool
	}{
		"the service holds every connection it may": {ErrAtCapacity, true},
		"the address holds every connection it may": {ErrTooManyPerIPHash, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			var r refused
			if !errors.As(c.err, &r) {
				t.Fatalf("%v cannot be read as a refusal", c.err)
			}
			if r.AtCapacity() != c.want {
				t.Fatalf("AtCapacity() = %v, want %v", r.AtCapacity(), c.want)
			}
		})
	}
}

func TestReleasingAConnectionMakesRoomForTheNextOne(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, Options{Connections: 1, PerIPHash: 1})

	release, err := svc.Acquire(t.Context(), "one-network")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if _, err := svc.Acquire(t.Context(), "another-network"); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("err = %v, want %v", err, ErrAtCapacity)
	}

	release()
	// Releasing twice is what a handler does when it returns twice over the
	// same connection; the place comes back once.
	release()

	if _, err := svc.Acquire(t.Context(), "another-network"); err != nil {
		t.Fatalf("after the place was given back: %v", err)
	}
}

// A server that dies leaves its leases behind. They stop being counted once
// nothing renews them, which is what keeps the count from creeping up.
func TestAConnectionThatIsNotRenewedStopsBeingCounted(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, Options{Connections: 1, PerIPHash: 1, LeaseTTL: 30 * time.Second})

	start := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return start }

	if _, err := svc.Acquire(t.Context(), "one-network"); err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	svc.now = func() time.Time { return start.Add(29 * time.Second) }
	if _, err := svc.Acquire(t.Context(), "another-network"); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("within the lifetime of the lease: err = %v, want %v", err, ErrAtCapacity)
	}

	svc.now = func() time.Time { return start.Add(30 * time.Second) }
	if _, err := svc.Acquire(t.Context(), "another-network"); err != nil {
		t.Fatalf("once the lease ran out: %v", err)
	}
}

func TestAnOpenConnectionKeepsItsPlace(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, Options{
		Connections:   1,
		PerIPHash:     1,
		LeaseTTL:      60 * time.Millisecond,
		RenewInterval: 5 * time.Millisecond,
	})

	release, err := svc.Acquire(t.Context(), "one-network")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer release()

	// The lease is renewed several times over, so the place is still counted
	// well after a lease that nothing renewed would have run out.
	time.Sleep(120 * time.Millisecond)

	held, err := svc.Connections(t.Context())
	if err != nil {
		t.Fatalf("Connections: %v", err)
	}
	if held != 1 {
		t.Fatalf("connections = %d, want 1", held)
	}
}

func TestTheRatesAreAskedForUnderTheirOwnLimits(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, Options{
		Message: Limit{Burst: 2, Interval: time.Second},
		Match:   Limit{Burst: 4, Interval: time.Minute},
	})

	if _, err := svc.AllowMessage(t.Context(), "a-session-token"); err != nil {
		t.Fatalf("AllowMessage: %v", err)
	}
	if _, _, err := svc.AllowMatch(t.Context(), "hash-of-a-network"); err != nil {
		t.Fatalf("AllowMatch: %v", err)
	}

	want := []takeCall{
		{name: "message", subject: "a-session-token", limit: Limit{Burst: 2, Interval: time.Second}},
		{name: "match", subject: "hash-of-a-network", limit: Limit{Burst: 4, Interval: time.Minute}},
	}
	if got := store.calls(); len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("calls = %+v, want %+v", got, want)
	}
}

func TestASpentRateIsReported(t *testing.T) {
	store := newFakeStore()
	store.allow = false
	svc := NewService(store, Options{})

	allowed, err := svc.AllowMessage(t.Context(), "a-session-token")
	if err != nil {
		t.Fatalf("AllowMessage: %v", err)
	}
	if allowed {
		t.Fatal("the message was allowed, want it held back")
	}
}

// A renew interval that does not fit under the lease would let an open
// connection drop out of the count between two renewals, and the cap would be
// read against fewer connections than are held.
func TestARenewIntervalThatDoesNotFitUnderTheLeaseIsShortened(t *testing.T) {
	cases := map[string]struct {
		leaseTTL time.Duration
		renew    time.Duration
		want     time.Duration
	}{
		"an interval longer than the lease": {
			leaseTTL: 30 * time.Second,
			renew:    time.Minute,
			want:     10 * time.Second,
		},
		"an interval that leaves room for one renewal": {
			leaseTTL: 30 * time.Second,
			renew:    20 * time.Second,
			want:     10 * time.Second,
		},
		"an interval that fits is kept": {
			leaseTTL: 30 * time.Second,
			renew:    2 * time.Second,
			want:     2 * time.Second,
		},
		"the default fits under the default lease": {
			leaseTTL: DefaultLeaseTTL,
			renew:    DefaultRenewInterval,
			want:     DefaultRenewInterval,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			svc := NewService(newFakeStore(), Options{LeaseTTL: c.leaseTTL, RenewInterval: c.renew})
			if svc.renewInterval != c.want {
				t.Fatalf("renew interval = %v, want %v", svc.renewInterval, c.want)
			}
		})
	}
}

// A lease so short that a third of it rounds to nothing still has to leave an
// interval a ticker can run on.
func TestTheRenewIntervalStaysAboveZero(t *testing.T) {
	svc := NewService(newFakeStore(), Options{LeaseTTL: time.Nanosecond, RenewInterval: time.Second})
	if svc.renewInterval <= 0 {
		t.Fatalf("renew interval = %v, want a positive interval", svc.renewInterval)
	}
}

func TestARefusedMatchNamesTheWaitBeforeTheNextTry(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, Options{Match: Limit{Burst: 1, Interval: 25 * time.Second}})

	allowed, wait, err := svc.AllowMatch(t.Context(), "hash-of-a-network")
	if err != nil {
		t.Fatalf("AllowMatch: %v", err)
	}
	if !allowed || wait != 0 {
		t.Fatalf("allowed = %v with a wait of %v, want it allowed with no wait", allowed, wait)
	}

	store.allow = false
	allowed, wait, err = svc.AllowMatch(t.Context(), "hash-of-a-network")
	if err != nil {
		t.Fatalf("AllowMatch: %v", err)
	}
	// The wait is the configured interval, so a deployment that changes the
	// rate changes what its clients are told.
	if allowed || wait != 25*time.Second {
		t.Fatalf("allowed = %v with a wait of %v, want it held back for 25s", allowed, wait)
	}
}
