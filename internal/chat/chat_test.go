package chat

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubAuthenticator accepts one token and rejects everything else. It counts
// its calls so that a test can tell whether the session was touched at all.
type stubAuthenticator struct {
	token string
	calls int
}

func (a *stubAuthenticator) Authenticate(r *http.Request) (string, error) {
	a.calls++

	c, err := r.Cookie("anontopic_session")
	if err != nil || c.Value != a.token {
		return "", errors.New("invalid session")
	}
	return a.token, nil
}

func roomRequest(cookie *http.Cookie) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/ws/rooms/1c8f", nil)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	return r
}

func TestRoomSocketRejectsRequestsWithoutASession(t *testing.T) {
	h := NewHandler(&stubAuthenticator{token: "valid-token"}, nil)

	for name, cookie := range map[string]*http.Cookie{
		"no cookie":     nil,
		"unknown token": {Name: "anontopic_session", Value: "someone-elses-token"},
		"empty token":   {Name: "anontopic_session", Value: ""},
		"other cookie":  {Name: "unrelated", Value: "valid-token"},
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.handleRoomSocket(rec, roomRequest(cookie))

			res := rec.Result()
			defer res.Body.Close()

			if res.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusUnauthorized)
			}
		})
	}
}

func TestRoomSocketAcceptsASession(t *testing.T) {
	h := NewHandler(&stubAuthenticator{token: "valid-token"}, nil)

	rec := httptest.NewRecorder()
	h.handleRoomSocket(rec, roomRequest(&http.Cookie{Name: "anontopic_session", Value: "valid-token"}))

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusNotImplemented)
	}
}

func TestRoomSocketRejectsAnotherSiteBeforeTouchingTheSession(t *testing.T) {
	sessions := &stubAuthenticator{token: "valid-token"}
	h := NewHandler(sessions, []string{"https://anontopic.example"})

	r := roomRequest(&http.Cookie{Name: "anontopic_session", Value: "valid-token"})
	r.Host = "api.anontopic.example"
	r.Header.Set("Origin", "https://attacker.example")

	rec := httptest.NewRecorder()
	h.handleRoomSocket(rec, r)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusForbidden)
	}
	// Verifying a session extends it, so a rejected origin must not reach it.
	if sessions.calls != 0 {
		t.Fatalf("the session was authenticated %d times for a request from another site", sessions.calls)
	}
}

func TestRoomSocketAcceptsAConfiguredOrigin(t *testing.T) {
	h := NewHandler(&stubAuthenticator{token: "valid-token"}, []string{"https://anontopic.example"})

	r := roomRequest(&http.Cookie{Name: "anontopic_session", Value: "valid-token"})
	r.Host = "api.anontopic.example"
	r.Header.Set("Origin", "https://anontopic.example")

	rec := httptest.NewRecorder()
	h.handleRoomSocket(rec, r)

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusNotImplemented)
	}
}

func TestOriginCheckerAdmitsOurOwnPagesOnly(t *testing.T) {
	check := originChecker([]string{"https://anontopic.example"})

	tests := map[string]struct {
		origin string
		want   bool
	}{
		"configured origin":        {origin: "https://anontopic.example", want: true},
		"same host as the request": {origin: "http://api.example", want: true},
		"another site":             {origin: "https://attacker.example", want: false},
		"no origin":                {origin: "", want: true},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/ws/rooms/1c8f", nil)
			r.Host = "api.example"
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}

			if got := check(r); got != tc.want {
				t.Fatalf("originChecker(%q) = %v, want %v", tc.origin, got, tc.want)
			}
		})
	}
}
