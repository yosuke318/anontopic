package report

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// testClock is the time the ban list reads, which a test moves itself.
type testClock struct {
	mu      sync.Mutex
	current time.Time
}

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current
}

func (c *testClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.current = c.current.Add(d)
}

// fakeBanRepository keeps the rows of banned_identifiers in memory.
type fakeBanRepository struct {
	mu    sync.Mutex
	clock *testClock
	bans  []Ban
	reads int
}

func (r *fakeBanRepository) AddBan(_ context.Context, b Ban) (Ban, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	b.ID = int64(len(r.bans) + 1)
	b.CreatedAt = r.clock.Now()
	r.bans = append(r.bans, b)
	return b, nil
}

func inForceAt(b Ban, now time.Time) bool {
	return b.Sanction != SanctionWarning && b.LiftedAt.IsZero() &&
		(b.BannedUntil.IsZero() || b.BannedUntil.After(now))
}

func (r *fakeBanRepository) BannedUntil(_ context.Context, identifierType, identifier string, now time.Time) (bool, time.Time, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads++

	banned := false
	var until time.Time
	for _, b := range r.bans {
		if b.IdentifierType != identifierType || b.Identifier != identifier || !inForceAt(b, now) {
			continue
		}
		if b.BannedUntil.IsZero() {
			return true, time.Time{}, nil
		}
		banned = true
		if b.BannedUntil.After(until) {
			until = b.BannedUntil
		}
	}
	return banned, until, nil
}

func (r *fakeBanRepository) CountSanctions(_ context.Context, identifierType, identifier string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	n := 0
	for _, b := range r.bans {
		if b.IdentifierType == identifierType && b.Identifier == identifier && b.LiftedAt.IsZero() {
			n++
		}
	}
	return n, nil
}

func (r *fakeBanRepository) ListBans(_ context.Context, f BanFilter, now time.Time) ([]Ban, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var out []Ban
	for i := len(r.bans) - 1; i >= 0 && len(out) < f.Limit; i-- {
		b := r.bans[i]
		if f.Active && !inForceAt(b, now) {
			continue
		}
		if f.BeforeID > 0 && b.ID >= f.BeforeID {
			continue
		}
		out = append(out, b)
	}
	return out, nil
}

func (r *fakeBanRepository) GetBan(_ context.Context, id int64) (Ban, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if id < 1 || id > int64(len(r.bans)) {
		return Ban{}, ErrNotFound
	}
	return r.bans[id-1], nil
}

func (r *fakeBanRepository) LiftBan(_ context.Context, id int64, at time.Time) (Ban, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if id < 1 || id > int64(len(r.bans)) {
		return Ban{}, ErrNotFound
	}
	r.bans[id-1].LiftedAt = at
	return r.bans[id-1], nil
}

// fakeSanctionStore keeps what RedisSanctionStore keeps, with the cached
// answers expiring on the test clock. The windows of the counts are left to
// the Redis tests.
type fakeSanctionStore struct {
	mu       sync.Mutex
	clock    *testClock
	cache    map[string]cachedAnswer
	blocked  map[string]int
	reported map[string]map[string]bool
	warnings map[string]bool
}

type cachedAnswer struct {
	banned  bool
	expires time.Time
}

func newFakeSanctionStore(clock *testClock) *fakeSanctionStore {
	return &fakeSanctionStore{
		clock:    clock,
		cache:    make(map[string]cachedAnswer),
		blocked:  make(map[string]int),
		reported: make(map[string]map[string]bool),
		warnings: make(map[string]bool),
	}
}

func (s *fakeSanctionStore) CachedBan(_ context.Context, identifierType, identifier string) (bool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	answer, ok := s.cache[identifierType+":"+identifier]
	if !ok || !s.clock.Now().Before(answer.expires) {
		return false, false, nil
	}
	return answer.banned, true, nil
}

func (s *fakeSanctionStore) CacheBan(_ context.Context, identifierType, identifier string, banned bool, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cache[identifierType+":"+identifier] = cachedAnswer{banned: banned, expires: s.clock.Now().Add(ttl)}
	return nil
}

func (s *fakeSanctionStore) ForgetBan(_ context.Context, identifierType, identifier string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.cache, identifierType+":"+identifier)
	return nil
}

func (s *fakeSanctionStore) AddBlocked(_ context.Context, identifierType, identifier string, _ time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.blocked[identifierType+":"+identifier]++
	return s.blocked[identifierType+":"+identifier], nil
}

func (s *fakeSanctionStore) AddReported(_ context.Context, identifierType, identifier, conversationID string, _ time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := identifierType + ":" + identifier
	if s.reported[key] == nil {
		s.reported[key] = make(map[string]bool)
	}
	s.reported[key][conversationID] = true
	return len(s.reported[key]), nil
}

func (s *fakeSanctionStore) ResetOffences(_ context.Context, identifierType, identifier string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.blocked, identifierType+":"+identifier)
	delete(s.reported, identifierType+":"+identifier)
	return nil
}

func (s *fakeSanctionStore) MarkWarning(_ context.Context, identifierType, identifier string, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.warnings[identifierType+":"+identifier] = true
	return nil
}

func (s *fakeSanctionStore) TakeWarning(_ context.Context, identifierType, identifier string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := identifierType + ":" + identifier
	warned := s.warnings[key]
	delete(s.warnings, key)
	return warned, nil
}

// newTestBans builds a ban list with the default thresholds on fakes that
// share one clock.
func newTestBans() (*Bans, *fakeBanRepository, *fakeSanctionStore, *testClock) {
	clock := &testClock{current: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)}
	repo := &fakeBanRepository{clock: clock}
	store := newFakeSanctionStore(clock)

	bans := NewBans(repo, store, BanOptions{})
	bans.now = clock.Now
	return bans, repo, store, clock
}

// block counts n blocked messages against id and returns the last sanction.
func block(t *testing.T, bans *Bans, id Identity, n int) string {
	t.Helper()

	var sanction string
	for range n {
		got, err := bans.RecordBlocked(context.Background(), id)
		if err != nil {
			t.Fatalf("RecordBlocked: %v", err)
		}
		sanction = got
	}
	return sanction
}

func isBanned(t *testing.T, bans *Bans, id Identity) bool {
	t.Helper()

	banned, err := bans.IsBanned(context.Background(), id)
	if err != nil {
		t.Fatalf("IsBanned: %v", err)
	}
	return banned
}

func TestBlockedMessagesEscalateFromAWarningToAPermanentBan(t *testing.T) {
	bans, repo, _, clock := newTestBans()
	id := Identity{IPHash: "shared-network", Device: "device-1"}

	if got := block(t, bans, id, DefaultBlockedThreshold-1); got != "" {
		t.Fatalf("sanction below the threshold = %q, want none", got)
	}
	if got := block(t, bans, id, 1); got != SanctionWarning {
		t.Fatalf("first sanction = %q, want %q", got, SanctionWarning)
	}
	if isBanned(t, bans, id) {
		t.Fatal("a warning barred the device")
	}

	// The count starts again after each sanction.
	if got := block(t, bans, id, DefaultBlockedThreshold); got != SanctionSuspension {
		t.Fatalf("second sanction = %q, want %q", got, SanctionSuspension)
	}
	if !isBanned(t, bans, id) {
		t.Fatal("a suspension did not bar the device")
	}
	suspension := repo.bans[len(repo.bans)-1]
	if want := clock.Now().Add(DefaultSuspension); !suspension.BannedUntil.Equal(want) {
		t.Fatalf("suspension ends at %v, want %v", suspension.BannedUntil, want)
	}

	clock.Advance(DefaultSuspension)
	if got := block(t, bans, id, DefaultBlockedThreshold); got != SanctionPermanent {
		t.Fatalf("third sanction = %q, want %q", got, SanctionPermanent)
	}

	for _, b := range repo.bans {
		if b.IdentifierType != IdentifierDevice || b.Source != SourceNGWord {
			t.Fatalf("ban %+v, want one on the device from blocked messages", b)
		}
	}
}

func TestAnAutomaticSanctionLeavesTheSharedAddressAlone(t *testing.T) {
	bans, _, _, _ := newTestBans()
	offender := Identity{IPHash: "school-network", Device: "device-1"}
	neighbour := Identity{IPHash: "school-network", Device: "device-2"}

	block(t, bans, offender, 2*DefaultBlockedThreshold)

	if !isBanned(t, bans, offender) {
		t.Fatal("the offending device is not barred")
	}
	if isBanned(t, bans, neighbour) {
		t.Fatal("another device behind the same address is barred")
	}
}

func TestAClientWithoutADeviceIDIsNotCounted(t *testing.T) {
	bans, repo, _, _ := newTestBans()

	block(t, bans, Identity{IPHash: "network"}, 3*DefaultBlockedThreshold)

	if len(repo.bans) != 0 {
		t.Fatalf("bans = %+v, want none", repo.bans)
	}
}

func TestASuspensionLiftsWhenItEnds(t *testing.T) {
	bans, _, _, clock := newTestBans()
	id := Identity{Device: "device-1"}

	block(t, bans, id, 2*DefaultBlockedThreshold)
	if !isBanned(t, bans, id) {
		t.Fatal("the suspension did not bar the device")
	}

	// The answer Redis keeps outlasts no suspension, so nothing has to clear
	// it for the suspension to lift.
	clock.Advance(DefaultSuspension - time.Second)
	if !isBanned(t, bans, id) {
		t.Fatal("the suspension lifted early")
	}
	clock.Advance(time.Second)
	if isBanned(t, bans, id) {
		t.Fatal("the suspension did not lift when it ended")
	}
}

func TestTheBanListIsReadFromRedisBetweenChanges(t *testing.T) {
	bans, repo, _, _ := newTestBans()
	id := Identity{IPHash: "network", Device: "device-1"}

	for range 3 {
		isBanned(t, bans, id)
	}
	if repo.reads != 2 {
		t.Fatalf("database reads = %d, want one per identifier", repo.reads)
	}

	// A ban clears what Redis kept, so it takes effect on the next check.
	block(t, bans, id, 2*DefaultBlockedThreshold)
	if !isBanned(t, bans, id) {
		t.Fatal("the new ban was not seen")
	}
}

func TestAWarningIsShownOnce(t *testing.T) {
	bans, _, _, _ := newTestBans()
	id := Identity{IPHash: "network", Device: "device-1"}

	block(t, bans, id, DefaultBlockedThreshold)

	for i, want := range []bool{true, false} {
		got, err := bans.TakeWarning(context.Background(), id)
		if err != nil {
			t.Fatalf("TakeWarning: %v", err)
		}
		if got != want {
			t.Fatalf("take %d = %v, want %v", i+1, got, want)
		}
	}
}

func TestReportsCountConversationsRatherThanReporters(t *testing.T) {
	bans, _, _, _ := newTestBans()
	id := Identity{Device: "device-1"}

	for range DefaultReportedThreshold {
		got, err := bans.RecordReported(context.Background(), id, "conversation-1")
		if err != nil {
			t.Fatalf("RecordReported: %v", err)
		}
		if got != "" {
			t.Fatalf("sanction for one conversation = %q, want none", got)
		}
	}

	var got string
	for _, conv := range []string{"conversation-2", "conversation-3"} {
		var err error
		if got, err = bans.RecordReported(context.Background(), id, conv); err != nil {
			t.Fatalf("RecordReported: %v", err)
		}
	}
	if got != SanctionWarning {
		t.Fatalf("sanction for three conversations = %q, want %q", got, SanctionWarning)
	}
}

func TestALiftedBanNeitherBarsNorCounts(t *testing.T) {
	bans, repo, _, _ := newTestBans()
	id := Identity{Device: "device-1"}

	block(t, bans, id, 2*DefaultBlockedThreshold)
	suspension := repo.bans[1]

	lifted, err := bans.Lift(context.Background(), suspension.ID)
	if err != nil {
		t.Fatalf("Lift: %v", err)
	}
	if lifted.LiftedAt.IsZero() {
		t.Fatalf("lifted = %+v, want a lift time", lifted)
	}
	if isBanned(t, bans, id) {
		t.Fatal("a lifted ban still bars the device")
	}
	if _, err := bans.Lift(context.Background(), suspension.ID); !errors.Is(err, ErrAlreadyLifted) {
		t.Fatalf("second Lift err = %v, want %v", err, ErrAlreadyLifted)
	}

	// The lifted suspension is not counted, so the next sanction is a
	// suspension again.
	if got := block(t, bans, id, DefaultBlockedThreshold); got != SanctionSuspension {
		t.Fatalf("sanction after the lift = %q, want %q", got, SanctionSuspension)
	}
}

func TestImposeRefusesWhatAnOperatorCannotHaveMeant(t *testing.T) {
	bans, _, _, _ := newTestBans()

	cases := map[string]struct {
		imp  Imposition
		want error
	}{
		"an unknown identifier type": {Imposition{IdentifierType: "session_token", Sanction: SanctionPermanent}, ErrUnknownIdentifierType},
		"an unknown sanction":        {Imposition{IdentifierType: IdentifierIPHash, Sanction: "mute"}, ErrUnknownSanction},
		"a suspension of no length":  {Imposition{IdentifierType: IdentifierIPHash, Sanction: SanctionSuspension}, ErrInvalidSuspension},
		"a suspension over a year":   {Imposition{IdentifierType: IdentifierIPHash, Sanction: SanctionSuspension, Duration: MaxSuspension + time.Hour}, ErrInvalidSuspension},
		"a permanent ban with a length": {
			Imposition{IdentifierType: IdentifierIPHash, Sanction: SanctionPermanent, Duration: time.Hour}, ErrInvalidSuspension,
		},
		"a reason too long": {
			Imposition{IdentifierType: IdentifierIPHash, Sanction: SanctionWarning, Reason: strings.Repeat("理", maxBanReasonRunes+1)}, ErrReasonTooLong,
		},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := bans.Impose(context.Background(), "identifier", c.imp); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}
