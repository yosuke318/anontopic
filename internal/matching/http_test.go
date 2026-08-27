package matching

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testTokenHeader carries the session token a test request stands for.
const testTokenHeader = "X-Test-Session"

// fakeSessions reads the token out of a header, which lets one test send
// requests as several users.
type fakeSessions struct{}

func (fakeSessions) Authenticate(r *http.Request) (string, error) {
	token := r.Header.Get(testTokenHeader)
	if token == "" {
		return "", errors.New("no session")
	}
	return token, nil
}

func (fakeSessions) IPHash(r *http.Request) string {
	return "ip-hash-" + r.Header.Get(testTokenHeader)
}

// newTestMux serves the module the way main.go does, so that the tests go
// through the routing.
func newTestMux(t *testing.T, svc *Service) *http.ServeMux {
	t.Helper()

	mux := http.NewServeMux()
	NewHandler(svc, fakeSessions{}).Register(mux)

	return mux
}

// do sends a request as the user behind token. An empty token stands for a
// client without a session.
func do(mux *http.ServeMux, method, target, token, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	if token != "" {
		r.Header.Set(testTokenHeader, token)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	return rec
}

func decodeBody[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()

	var body T
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return body
}

func TestJoinEndpointAnswersWaitingUntilTheRoomIsFormed(t *testing.T) {
	svc, _ := newTestService(t, newFakeStore(), newFakeRepository())
	mux := newTestMux(t, svc)

	rec := do(mux, http.MethodPost, "/api/matching", "alice", `{"topic_id":1,"room_type":2}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	waiting := decodeBody[stateResponse](t, rec)
	if waiting.State != stateWaiting || waiting.RoomType != 2 || waiting.WaitingSince == nil {
		t.Fatalf("body = %+v, want a wait in a room of two", waiting)
	}

	rec = do(mux, http.MethodPost, "/api/matching", "bob", `{"topic_id":1,"room_type":2}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	matched := decodeBody[stateResponse](t, rec)
	if matched.State != stateMatched || matched.Conversation == nil || matched.Conversation.ID == "" {
		t.Fatalf("body = %+v, want a conversation", matched)
	}
}

func TestStateEndpointHandsTheConversationToTheUserWhoWasWaiting(t *testing.T) {
	svc, _ := newTestService(t, newFakeStore(), newFakeRepository())
	mux := newTestMux(t, svc)

	do(mux, http.MethodPost, "/api/matching", "alice", `{"topic_id":1,"room_type":2}`)
	do(mux, http.MethodPost, "/api/matching", "bob", `{"topic_id":1,"room_type":2}`)

	rec := do(mux, http.MethodGet, "/api/matching", "alice", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := decodeBody[stateResponse](t, rec)
	if body.State != stateMatched || body.Conversation == nil {
		t.Fatalf("body = %+v, want a conversation", body)
	}
}

func TestStateEndpointAnswers404ForAUserWhoIsNotWaiting(t *testing.T) {
	svc, _ := newTestService(t, newFakeStore(), newFakeRepository())

	rec := do(newTestMux(t, svc), http.MethodGet, "/api/matching", "alice", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestLeaveEndpointAnswers204WithAndWithoutAWait(t *testing.T) {
	svc, _ := newTestService(t, newFakeStore(), newFakeRepository())
	mux := newTestMux(t, svc)

	do(mux, http.MethodPost, "/api/matching", "alice", `{"topic_id":1,"room_type":2}`)

	for range 2 {
		rec := do(mux, http.MethodDelete, "/api/matching", "alice", "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
		}
	}
}

func TestEndpointsRefuseARequestWithoutASession(t *testing.T) {
	svc, _ := newTestService(t, newFakeStore(), newFakeRepository())
	mux := newTestMux(t, svc)

	cases := []struct{ method, body string }{
		{http.MethodPost, `{"topic_id":1,"room_type":2}`},
		{http.MethodGet, ""},
		{http.MethodDelete, ""},
	}
	for _, c := range cases {
		rec := do(mux, c.method, "/api/matching", "", c.body)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d, want %d", c.method, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestJoinEndpointRefusesRequestsItCannotQueue(t *testing.T) {
	svc := NewService(newFakeStore(), newFakeRepository(), fakeTopics{active: []int{1}},
		fakeBans{banned: []string{"ip-hash-banned"}}, Options{})
	mux := newTestMux(t, svc)

	cases := []struct {
		name  string
		token string
		body  string
		want  int
	}{
		{"body that is not the expected shape", "alice", `{"topic":1}`, http.StatusBadRequest},
		{"a second JSON value after the body", "alice", `{"topic_id":1,"room_type":2}{}`, http.StatusBadRequest},
		{"a stray bracket after the body", "alice", `{"topic_id":1,"room_type":2}]`, http.StatusBadRequest},
		{"room type no conversation can have", "alice", `{"topic_id":1,"room_type":4}`, http.StatusBadRequest},
		{"topic that is not offered", "alice", `{"topic_id":99,"room_type":2}`, http.StatusBadRequest},
		{"banned identifier", "banned", `{"topic_id":1,"room_type":2}`, http.StatusForbidden},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(mux, http.MethodPost, "/api/matching", c.token, c.body)
			if rec.Code != c.want {
				t.Fatalf("status = %d, want %d", rec.Code, c.want)
			}
		})
	}
}

func TestJoinEndpointAnswers409ForAUserWaitingInAnotherQueue(t *testing.T) {
	svc, _ := newTestService(t, newFakeStore(), newFakeRepository())
	mux := newTestMux(t, svc)

	do(mux, http.MethodPost, "/api/matching", "alice", `{"topic_id":1,"room_type":2}`)

	rec := do(mux, http.MethodPost, "/api/matching", "alice", `{"topic_id":2,"room_type":2}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}
