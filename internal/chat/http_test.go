package chat

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// roomRequest is a handshake for the conversation the fake repository holds.
func roomRequest(conversationID string, cookie *http.Cookie) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/ws/rooms/"+conversationID, nil)
	r.SetPathValue("roomID", conversationID)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	return r
}

// testHandler builds a handler on one conversation between two participants.
func testHandler(t *testing.T, allowedOrigins []string) (*Handler, *fakeRepository, *stubAuthenticator) {
	t.Helper()

	repo := newFakeRepository(tokenAlice, tokenBob)
	sessions := &stubAuthenticator{tokens: []string{tokenAlice, tokenBob}}
	svc := NewService(repo, newFakeStore(), nil, nil, testOptions())

	return NewHandler(svc, sessions, nil, allowedOrigins), repo, sessions
}

func TestRoomSocketRejectsRequestsWithoutASession(t *testing.T) {
	h, repo, _ := testHandler(t, nil)

	for name, cookie := range map[string]*http.Cookie{
		"no cookie":     nil,
		"unknown token": {Name: "anontopic_session", Value: "someone-elses-token"},
		"empty token":   {Name: "anontopic_session", Value: ""},
		"other cookie":  {Name: "unrelated", Value: tokenAlice},
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.handleRoomSocket(rec, roomRequest(repo.conv.ID, cookie))

			res := rec.Result()
			defer res.Body.Close()

			if res.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusUnauthorized)
			}
		})
	}
}

func TestRoomSocketRejectsAnotherSiteBeforeTouchingTheSession(t *testing.T) {
	h, repo, sessions := testHandler(t, []string{"https://anontopic.example"})

	r := roomRequest(repo.conv.ID, &http.Cookie{Name: "anontopic_session", Value: tokenAlice})
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
	if sessions.callCount() != 0 {
		t.Fatalf("the session was authenticated %d times for a request from another site", sessions.callCount())
	}
}

func TestRoomSocketRejectsARoomTheCallerIsNotIn(t *testing.T) {
	h, repo, _ := testHandler(t, nil)

	tests := map[string]struct {
		conversationID string
		token          string
		want           int
	}{
		// A conversation somebody else's answers as one that does not exist,
		// so that nobody can learn which conversations are being held.
		"another conversation":     {conversationID: "e6a7d1c7-6d4e-4a2f-89b6-2b2a3e6b1f10", token: tokenAlice, want: http.StatusForbidden},
		"an id that is no id":      {conversationID: "1c8f", token: tokenAlice, want: http.StatusForbidden},
		"a session of no room":     {conversationID: repo.conv.ID, token: "token-of-nobody", want: http.StatusUnauthorized},
		"a conversation of theirs": {conversationID: repo.conv.ID, token: tokenBob, want: http.StatusBadRequest},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.handleRoomSocket(rec, roomRequest(tc.conversationID, &http.Cookie{Name: "anontopic_session", Value: tc.token}))

			res := rec.Result()
			defer res.Body.Close()

			// An admitted participant reaches the upgrade, which turns down a
			// request that is not a WebSocket handshake.
			if res.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", res.StatusCode, tc.want)
			}
		})
	}
}

func TestRoomSocketRefusesAConversationThatIsOver(t *testing.T) {
	h, repo, _ := testHandler(t, nil)

	if _, err := repo.End(t.Context(), repo.conv.ID, endReasonUserLeft, time.Now().UTC()); err != nil {
		t.Fatalf("end the conversation: %v", err)
	}

	rec := httptest.NewRecorder()
	h.handleRoomSocket(rec, roomRequest(repo.conv.ID, &http.Cookie{Name: "anontopic_session", Value: tokenAlice}))

	res := rec.Result()
	defer res.Body.Close()

	if res.StatusCode != http.StatusGone {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusGone)
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

func TestRoomSocketRefusesAHandshakeThereIsNoRoomFor(t *testing.T) {
	tests := map[string]struct {
		err  error
		want int
	}{
		"the service holds every connection it may": {
			err:  stubRefusal{message: "at capacity", atCapacity: true},
			want: http.StatusServiceUnavailable,
		},
		"the address holds every connection it may": {
			err:  stubRefusal{message: "too many from one address"},
			want: http.StatusTooManyRequests,
		},
		"the count could not be read": {
			err:  errors.New("redis is unreachable"),
			want: http.StatusInternalServerError,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			repo := newFakeRepository(tokenAlice, tokenBob)
			sessions := &stubAuthenticator{tokens: []string{tokenAlice}}
			svc := NewService(repo, newFakeStore(), nil, nil, testOptions())
			connections := &stubConnectionLimiter{err: tc.err}
			h := NewHandler(svc, sessions, connections, nil)

			rec := httptest.NewRecorder()
			h.handleRoomSocket(rec, roomRequest(repo.conv.ID, &http.Cookie{
				Name:  "anontopic_session",
				Value: tokenAlice,
			}))

			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
			// A connection there is no room for costs no session lookup.
			if sessions.callCount() != 0 {
				t.Fatalf("the session was read %d times, want 0", sessions.callCount())
			}
			if tc.want != http.StatusInternalServerError && rec.Header().Get("Retry-After") == "" {
				t.Fatal("no Retry-After was sent")
			}
		})
	}
}

func TestRoomSocketGivesBackThePlaceOfAHandshakeItRefuses(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	sessions := &stubAuthenticator{tokens: []string{tokenAlice}}
	svc := NewService(repo, newFakeStore(), nil, nil, testOptions())
	connections := &stubConnectionLimiter{}
	h := NewHandler(svc, sessions, connections, nil)

	rec := httptest.NewRecorder()
	h.handleRoomSocket(rec, roomRequest(repo.conv.ID, &http.Cookie{
		Name:  "anontopic_session",
		Value: "someone-elses-token",
	}))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	acquired, released := connections.counts()
	if acquired != 1 || released != 1 {
		t.Fatalf("acquired %d and released %d places, want 1 and 1", acquired, released)
	}
	if seen := connections.seen(); len(seen) != 1 || seen[0] != sessions.IPHash(nil) {
		t.Fatalf("counted against %v, want the hashed address of the request", seen)
	}
}
