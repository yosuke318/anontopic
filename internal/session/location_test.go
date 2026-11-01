package session

import (
	"context"
	"net/netip"
	"testing"
	"time"

	"github.com/yosuke318/anontopic/internal/geoip"
)

// fakeLocator places one address in Tokyo and knows no other.
type fakeLocator struct{}

func (fakeLocator) Locate(addr netip.Addr) geoip.Location {
	if addr == netip.MustParseAddr("203.0.113.7") {
		return geoip.Location{Country: "JP", Prefecture: "JP-13", ASN: 2516}
	}
	return geoip.Location{}
}

func TestIssueLocatesTheAddressTheSessionCameFrom(t *testing.T) {
	clock := &testClock{current: time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)}
	svc := NewService(newMemoryStore(clock.Now), []byte("test-key"), Options{Locator: fakeLocator{}})
	svc.now = clock.Now

	sess, err := svc.Issue(context.Background(), requestFrom("203.0.113.7:5000"), Attributes{})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	want := geoip.Location{Country: "JP", Prefecture: "JP-13", ASN: 2516}
	if sess.Location != want {
		t.Fatalf("Location = %+v, want %+v", sess.Location, want)
	}

	looked, err := svc.Lookup(context.Background(), sess.Token)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if looked.Location != want {
		t.Fatalf("Location after Lookup = %+v, want %+v", looked.Location, want)
	}

	other, err := svc.Issue(context.Background(), requestFrom("192.0.2.1:5000"), Attributes{})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if other.Location != (geoip.Location{}) {
		t.Fatalf("Location of an unknown address = %+v, want none", other.Location)
	}
}

func TestIssueWithoutALocatorLocatesNothing(t *testing.T) {
	svc, _ := newTestService(t)

	sess, err := svc.Issue(context.Background(), requestFrom("203.0.113.7:5000"), Attributes{})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if sess.Location != (geoip.Location{}) {
		t.Fatalf("Location = %+v, want none", sess.Location)
	}
}

func TestRedisStoreKeepsTheLocation(t *testing.T) {
	store := NewRedisStore(redisTestClient(t))
	ctx := context.Background()

	token, err := newToken()
	if err != nil {
		t.Fatalf("newToken: %v", err)
	}
	want := geoip.Location{Country: "JP", Prefecture: "JP-27", ASN: 4294967295}
	rec := Record{IPHash: testIPHash(t), IssuedAt: time.Now().UTC(), Location: want}
	if err := store.Create(ctx, token, rec, time.Minute); err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = store.Delete(context.Background(), token) })

	got, err := store.Get(ctx, token)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Location != want {
		t.Fatalf("Location = %+v, want %+v", got.Location, want)
	}
}
