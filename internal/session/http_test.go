package session

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestHandler(t *testing.T) (*Handler, *Service) {
	t.Helper()

	svc, _ := newTestService(t)
	mux := http.NewServeMux()
	h := NewHandler(svc)
	h.Register(mux)

	return h, svc
}

// sessionCookie returns the session cookie a response sets.
func sessionCookie(t *testing.T, res *http.Response) *http.Cookie {
	t.Helper()

	for _, c := range res.Cookies() {
		if c.Name == cookieName {
			return c
		}
	}
	t.Fatalf("response sets no %s cookie", cookieName)
	return nil
}

func TestIssueEndpointSetsAnHTTPOnlyCookie(t *testing.T) {
	h, svc := newTestHandler(t)

	rec := httptest.NewRecorder()
	h.handleIssue(rec, httptest.NewRequest(http.MethodPost, "/api/session", nil))

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusCreated)
	}

	cookie := sessionCookie(t, res)
	if !cookie.HttpOnly || !cookie.Secure {
		t.Fatalf("cookie is HttpOnly=%v Secure=%v, want both true", cookie.HttpOnly, cookie.Secure)
	}
	if cookie.MaxAge <= 0 {
		t.Fatalf("cookie MaxAge = %d, want a positive lifetime", cookie.MaxAge)
	}

	if _, err := svc.Verify(context.Background(), cookie.Value); err != nil {
		t.Fatalf("Verify of the issued cookie: %v", err)
	}

	var body sessionResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.ExpiresAt.IsZero() {
		t.Fatal("body carries no expiry")
	}
}

func TestIssueEndpointKeepsAValidSession(t *testing.T) {
	h, _ := newTestHandler(t)

	first := httptest.NewRecorder()
	h.handleIssue(first, httptest.NewRequest(http.MethodPost, "/api/session", nil))
	firstRes := first.Result()
	defer firstRes.Body.Close()
	issued := sessionCookie(t, firstRes)

	second := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/session", nil)
	r.AddCookie(issued)
	h.handleIssue(second, r)

	secondRes := second.Result()
	defer secondRes.Body.Close()

	if secondRes.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", secondRes.StatusCode, http.StatusOK)
	}
	if got := sessionCookie(t, secondRes).Value; got != issued.Value {
		t.Fatal("a valid session was replaced by a new token")
	}
}

func TestRevokeEndpointClearsTheCookieAndTheSession(t *testing.T) {
	h, svc := newTestHandler(t)

	issue := httptest.NewRecorder()
	h.handleIssue(issue, httptest.NewRequest(http.MethodPost, "/api/session", nil))
	issueRes := issue.Result()
	defer issueRes.Body.Close()
	issued := sessionCookie(t, issueRes)

	revoke := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodDelete, "/api/session", nil)
	r.AddCookie(issued)
	h.handleRevoke(revoke, r)

	revokeRes := revoke.Result()
	defer revokeRes.Body.Close()

	if revokeRes.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", revokeRes.StatusCode, http.StatusNoContent)
	}
	if got := sessionCookie(t, revokeRes).MaxAge; got >= 0 {
		t.Fatalf("cookie MaxAge = %d, want a negative value that drops it", got)
	}
	if _, err := svc.Verify(context.Background(), issued.Value); err == nil {
		t.Fatal("the session outlived its revocation")
	}
}

func TestRequireSessionGuardsTheWrappedHandler(t *testing.T) {
	h, _ := newTestHandler(t)

	var seen string
	guarded := h.svc.RequireSession(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen, _ = TokenFrom(r.Context())
	}))

	anonymous := httptest.NewRecorder()
	guarded.ServeHTTP(anonymous, httptest.NewRequest(http.MethodGet, "/", nil))
	anonymousRes := anonymous.Result()
	defer anonymousRes.Body.Close()

	if anonymousRes.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status without a session = %d, want %d", anonymousRes.StatusCode, http.StatusUnauthorized)
	}
	if seen != "" {
		t.Fatal("the wrapped handler ran without a session")
	}

	issue := httptest.NewRecorder()
	h.handleIssue(issue, httptest.NewRequest(http.MethodPost, "/api/session", nil))
	issueRes := issue.Result()
	defer issueRes.Body.Close()
	issued := sessionCookie(t, issueRes)

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(issued)
	authorized := httptest.NewRecorder()
	guarded.ServeHTTP(authorized, r)
	authorizedRes := authorized.Result()
	defer authorizedRes.Body.Close()

	if authorizedRes.StatusCode != http.StatusOK {
		t.Fatalf("status with a session = %d, want %d", authorizedRes.StatusCode, http.StatusOK)
	}
	if seen != issued.Value {
		t.Fatalf("token in context = %q, want %q", seen, issued.Value)
	}
}
