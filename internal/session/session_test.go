package session

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// testClock lets a test move the service and its store forward together.
type testClock struct {
	current time.Time
}

func (c *testClock) Now() time.Time { return c.current }

func (c *testClock) Advance(d time.Duration) { c.current = c.current.Add(d) }

// memoryStore is a Store that keeps records in a map and expires them against
// the clock the test controls.
type memoryStore struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string]memoryEntry
}

type memoryEntry struct {
	rec       Record
	expiresAt time.Time
}

func newMemoryStore(now func() time.Time) *memoryStore {
	return &memoryStore{now: now, entries: map[string]memoryEntry{}}
}

func (s *memoryStore) Create(_ context.Context, token string, rec Record, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries[token] = memoryEntry{rec: rec, expiresAt: s.now().Add(ttl)}
	return nil
}

func (s *memoryStore) Get(_ context.Context, token string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[token]
	if !ok || !s.now().Before(entry.expiresAt) {
		return Record{}, ErrNotStored
	}
	return entry.rec, nil
}

func (s *memoryStore) Refresh(_ context.Context, token string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[token]
	if !ok || !s.now().Before(entry.expiresAt) {
		return ErrNotStored
	}
	entry.expiresAt = s.now().Add(ttl)
	s.entries[token] = entry
	return nil
}

func (s *memoryStore) Delete(_ context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.entries, token)
	return nil
}

func (s *memoryStore) DeleteByIPHash(_ context.Context, ipHash string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	deleted := 0
	for token, entry := range s.entries {
		if entry.rec.IPHash == ipHash {
			delete(s.entries, token)
			deleted++
		}
	}
	return deleted, nil
}

func newTestService(t *testing.T) (*Service, *testClock) {
	t.Helper()

	clock := &testClock{current: time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)}
	svc := NewService(newMemoryStore(clock.Now), []byte("test-key"), Options{CookieSecure: true})
	svc.now = clock.Now

	return svc, clock
}

func requestFrom(remoteAddr string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = remoteAddr
	return r
}

func mustIssue(t *testing.T, svc *Service, remoteAddr string) Session {
	t.Helper()

	sess, err := svc.Issue(context.Background(), requestFrom(remoteAddr))
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	return sess
}

func TestIssueProducesTokenThatFitsTheSchema(t *testing.T) {
	svc, clock := newTestService(t)

	first := mustIssue(t, svc, "192.0.2.10:5000")
	second := mustIssue(t, svc, "192.0.2.10:5000")

	if first.Token == second.Token {
		t.Fatal("two issued sessions share a token")
	}
	if got := len(first.Token); got > 64 {
		t.Fatalf("token is %d characters, session_token holds 64", got)
	}
	if want := clock.Now().Add(DefaultIdleTTL); !first.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %v, want %v", first.ExpiresAt, want)
	}
}

func TestVerifyAcceptsAnIssuedToken(t *testing.T) {
	svc, _ := newTestService(t)

	issued := mustIssue(t, svc, "192.0.2.10:5000")

	verified, err := svc.Verify(context.Background(), issued.Token)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if verified.IPHash != issued.IPHash {
		t.Fatalf("IPHash = %q, want %q", verified.IPHash, issued.IPHash)
	}
}

func TestVerifyRejectsTokensItDidNotIssue(t *testing.T) {
	svc, _ := newTestService(t)

	unknown, err := newToken()
	if err != nil {
		t.Fatalf("newToken: %v", err)
	}

	for name, token := range map[string]string{
		"empty":      "",
		"malformed":  "not-a-session-token",
		"unknown":    unknown,
		"wrong size": strings.Repeat("a", 20),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.Verify(context.Background(), token); !errors.Is(err, ErrInvalidSession) {
				t.Fatalf("Verify(%q) error = %v, want ErrInvalidSession", token, err)
			}
		})
	}
}

func TestVerifyExtendsTheIdleWindow(t *testing.T) {
	svc, clock := newTestService(t)

	issued := mustIssue(t, svc, "192.0.2.10:5000")

	clock.Advance(DefaultIdleTTL - time.Hour)
	if _, err := svc.Verify(context.Background(), issued.Token); err != nil {
		t.Fatalf("Verify inside the idle window: %v", err)
	}

	clock.Advance(DefaultIdleTTL - time.Hour)
	if _, err := svc.Verify(context.Background(), issued.Token); err != nil {
		t.Fatalf("Verify after the window was extended: %v", err)
	}
}

func TestVerifyRejectsAnIdleSession(t *testing.T) {
	svc, clock := newTestService(t)

	issued := mustIssue(t, svc, "192.0.2.10:5000")

	clock.Advance(DefaultIdleTTL + time.Minute)

	if _, err := svc.Verify(context.Background(), issued.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("Verify error = %v, want ErrInvalidSession", err)
	}
}

func TestVerifyRejectsASessionPastItsAbsoluteAge(t *testing.T) {
	svc, clock := newTestService(t)

	issued := mustIssue(t, svc, "192.0.2.10:5000")

	// Keeping the session in use holds off the idle window but not the age.
	for elapsed := time.Duration(0); elapsed < DefaultAbsoluteTTL; elapsed += time.Hour {
		if _, err := svc.Verify(context.Background(), issued.Token); err != nil {
			t.Fatalf("Verify after %v: %v", elapsed, err)
		}
		clock.Advance(time.Hour)
	}

	if _, err := svc.Verify(context.Background(), issued.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("Verify error = %v, want ErrInvalidSession", err)
	}
}

func TestRevokeEndsTheSession(t *testing.T) {
	svc, _ := newTestService(t)

	issued := mustIssue(t, svc, "192.0.2.10:5000")

	if err := svc.Revoke(context.Background(), issued.Token); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := svc.Verify(context.Background(), issued.Token); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("Verify after Revoke error = %v, want ErrInvalidSession", err)
	}
}

func TestRevokeByIPHashEndsEverySessionOfThatAddress(t *testing.T) {
	svc, _ := newTestService(t)

	banned := []Session{
		mustIssue(t, svc, "192.0.2.10:5000"),
		mustIssue(t, svc, "192.0.2.10:5001"),
	}
	other := mustIssue(t, svc, "192.0.2.11:5000")

	n, err := svc.RevokeByIPHash(context.Background(), banned[0].IPHash)
	if err != nil {
		t.Fatalf("RevokeByIPHash: %v", err)
	}
	if n != len(banned) {
		t.Fatalf("revoked %d sessions, want %d", n, len(banned))
	}

	for _, sess := range banned {
		if _, err := svc.Verify(context.Background(), sess.Token); !errors.Is(err, ErrInvalidSession) {
			t.Fatalf("Verify of a revoked session error = %v, want ErrInvalidSession", err)
		}
	}
	if _, err := svc.Verify(context.Background(), other.Token); err != nil {
		t.Fatalf("Verify of a session from another address: %v", err)
	}
}

func TestIPHashDoesNotCarryTheAddress(t *testing.T) {
	svc, _ := newTestService(t)

	hash := svc.IPHash(requestFrom("192.0.2.10:5000"))
	if strings.Contains(hash, "192.0.2.10") {
		t.Fatalf("IPHash %q contains the address", hash)
	}

	same := svc.IPHash(requestFrom("192.0.2.10:6000"))
	if hash != same {
		t.Fatal("IPHash changed with the source port")
	}

	other := svc.IPHash(requestFrom("192.0.2.11:5000"))
	if hash == other {
		t.Fatal("IPHash is the same for two addresses")
	}
}

func TestIPHashReadsForwardedForOnlyWhenTrusted(t *testing.T) {
	untrusted, _ := newTestService(t)

	trusted, _ := newTestService(t)
	trusted.trustForwardedFor = true

	forwarded := requestFrom("192.0.2.10:5000")
	forwarded.Header.Set("X-Forwarded-For", "198.51.100.7, 203.0.113.9")

	if got, want := untrusted.IPHash(forwarded), untrusted.IPHash(requestFrom("192.0.2.10:5000")); got != want {
		t.Fatal("IPHash followed X-Forwarded-For without being told to trust it")
	}
	if got, want := trusted.IPHash(forwarded), trusted.IPHash(requestFrom("203.0.113.9:5000")); got != want {
		t.Fatal("IPHash did not take the last X-Forwarded-For entry")
	}
}
