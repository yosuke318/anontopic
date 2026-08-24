// Package session owns the anonymous session tokens that identify a
// participant while they use the service. It issues a token on first access,
// verifies it on every request, and revokes it on leave or on a ban.
//
// A token is an opaque random string. Everything it stands for lives in the
// Store behind a TTL, so revoking a session is a delete. The token value is
// what other modules persist as session_token.
//
// Boundary: other modules receive the token string through an interface they
// own and never read this module's storage.
package session

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	// A 32 byte token is 43 characters in base64url, which fits the
	// session_token columns of the schema (VARCHAR(64)).
	tokenBytes = 32

	// DefaultIdleTTL is how long a session survives without being used.
	DefaultIdleTTL = 24 * time.Hour

	// DefaultAbsoluteTTL is the age at which a session expires even if it is
	// used continuously.
	DefaultAbsoluteTTL = 7 * 24 * time.Hour
)

// ErrInvalidSession is returned when a token is unknown, expired or revoked.
// Callers translate it into 401 for the client.
var ErrInvalidSession = errors.New("session: invalid session")

// Session is what a token stands for.
type Session struct {
	Token string
	// IPHash identifies the network the session was issued to without keeping
	// the address itself.
	IPHash   string
	IssuedAt time.Time
	// ExpiresAt is when the session dies if it is not used again.
	ExpiresAt time.Time
}

// Service issues, verifies and revokes sessions.
type Service struct {
	store             Store
	ipHashKey         []byte
	idleTTL           time.Duration
	absoluteTTL       time.Duration
	trustForwardedFor bool
	cookieSecure      bool
	cookieSameSite    http.SameSite
	now               func() time.Time
}

// Options configures a Service. The zero value of each field selects the
// default described on the field.
type Options struct {
	// IdleTTL defaults to DefaultIdleTTL.
	IdleTTL time.Duration
	// AbsoluteTTL defaults to DefaultAbsoluteTTL.
	AbsoluteTTL time.Duration
	// TrustForwardedFor makes the client address come from the last
	// X-Forwarded-For entry. Enable it only when every request passes through
	// a load balancer that appends the peer it saw, because earlier entries
	// are written by the client and can be forged.
	TrustForwardedFor bool
	// CookieSecure marks the session cookie Secure.
	CookieSecure bool
	// CookieSameSite defaults to http.SameSiteLaxMode. Serving the frontend
	// from another site requires http.SameSiteNoneMode, which browsers accept
	// only together with CookieSecure.
	CookieSameSite http.SameSite
}

// NewService builds a Service. ipHashKey is the secret the client address is
// hashed with; it must stay stable for bans to keep matching returning users.
func NewService(store Store, ipHashKey []byte, opts Options) *Service {
	if opts.IdleTTL <= 0 {
		opts.IdleTTL = DefaultIdleTTL
	}
	if opts.AbsoluteTTL <= 0 {
		opts.AbsoluteTTL = DefaultAbsoluteTTL
	}
	if opts.CookieSameSite == http.SameSiteDefaultMode {
		opts.CookieSameSite = http.SameSiteLaxMode
	}

	return &Service{
		store:             store,
		ipHashKey:         ipHashKey,
		idleTTL:           opts.IdleTTL,
		absoluteTTL:       opts.AbsoluteTTL,
		trustForwardedFor: opts.TrustForwardedFor,
		cookieSecure:      opts.CookieSecure,
		cookieSameSite:    opts.CookieSameSite,
		now:               time.Now,
	}
}

// Issue creates a session for the client behind r.
func (s *Service) Issue(ctx context.Context, r *http.Request) (Session, error) {
	token, err := newToken()
	if err != nil {
		return Session{}, err
	}

	issuedAt := s.now().UTC()
	rec := Record{IPHash: s.IPHash(r), IssuedAt: issuedAt}
	ttl := min(s.idleTTL, s.absoluteTTL)

	if err := s.store.Create(ctx, token, rec, ttl); err != nil {
		return Session{}, fmt.Errorf("store session: %w", err)
	}

	return Session{
		Token:     token,
		IPHash:    rec.IPHash,
		IssuedAt:  issuedAt,
		ExpiresAt: issuedAt.Add(ttl),
	}, nil
}

// Verify resolves a token and extends its idle window. A session that has
// reached its absolute age is deleted and reported as invalid.
func (s *Service) Verify(ctx context.Context, token string) (Session, error) {
	if !wellFormed(token) {
		return Session{}, ErrInvalidSession
	}

	rec, err := s.store.Get(ctx, token)
	if errors.Is(err, ErrNotStored) {
		return Session{}, ErrInvalidSession
	}
	if err != nil {
		return Session{}, fmt.Errorf("read session: %w", err)
	}

	now := s.now().UTC()
	deadline := rec.IssuedAt.Add(s.absoluteTTL)
	if !now.Before(deadline) {
		if err := s.store.Delete(ctx, token); err != nil {
			return Session{}, fmt.Errorf("delete expired session: %w", err)
		}
		return Session{}, ErrInvalidSession
	}

	ttl := min(s.idleTTL, deadline.Sub(now))
	if err := s.store.Refresh(ctx, token, ttl); err != nil {
		if errors.Is(err, ErrNotStored) {
			return Session{}, ErrInvalidSession
		}
		return Session{}, fmt.Errorf("refresh session: %w", err)
	}

	return Session{
		Token:     token,
		IPHash:    rec.IPHash,
		IssuedAt:  rec.IssuedAt,
		ExpiresAt: now.Add(ttl),
	}, nil
}

// Authenticate resolves the session carried by r and returns its token. It is
// the entry point other modules use to guard a request.
func (s *Service) Authenticate(r *http.Request) (string, error) {
	token, ok := tokenFromRequest(r)
	if !ok {
		return "", ErrInvalidSession
	}

	sess, err := s.Verify(r.Context(), token)
	if err != nil {
		return "", err
	}
	return sess.Token, nil
}

// Revoke ends a single session.
func (s *Service) Revoke(ctx context.Context, token string) error {
	if !wellFormed(token) {
		return ErrInvalidSession
	}
	if err := s.store.Delete(ctx, token); err != nil && !errors.Is(err, ErrNotStored) {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

// RevokeByIPHash ends every session issued to one hashed address and reports
// how many it ended. Enforcement of a ban calls it so that the banned user
// loses the connection they already hold.
func (s *Service) RevokeByIPHash(ctx context.Context, ipHash string) (int, error) {
	n, err := s.store.DeleteByIPHash(ctx, ipHash)
	if err != nil {
		return 0, fmt.Errorf("delete sessions by ip hash: %w", err)
	}
	return n, nil
}

// IPHash is the keyed hash of the address r came from. It is stable for the
// lifetime of the key, so it can be stored as a ban identifier while the
// address itself is never written down.
func (s *Service) IPHash(r *http.Request) string {
	mac := hmac.New(sha256.New, s.ipHashKey)
	mac.Write([]byte(s.clientIP(r)))
	return hex.EncodeToString(mac.Sum(nil))
}

// clientIP is the address the session belongs to.
func (s *Service) clientIP(r *http.Request) string {
	if s.trustForwardedFor {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			entries := strings.Split(fwd, ",")
			return strings.TrimSpace(entries[len(entries)-1])
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// newToken draws a token from the cryptographic random source.
func newToken() (string, error) {
	buf := make([]byte, tokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// wellFormed reports whether token can be one this package issued. It keeps
// arbitrary client input out of the store lookup.
func wellFormed(token string) bool {
	if len(token) != base64.RawURLEncoding.EncodedLen(tokenBytes) {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil
}
