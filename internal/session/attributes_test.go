package session

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// issued is what POST /api/session answered: the status and the session
// cookie it set, which is nil when it set none.
type issued struct {
	status int
	cookie *http.Cookie
}

// postSession sends POST /api/session with body, and with cookie when it is
// not nil.
func postSession(t *testing.T, h *Handler, body string, cookie *http.Cookie) issued {
	t.Helper()

	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(http.MethodPost, "/api/session", nil)
	} else {
		r = httptest.NewRequest(http.MethodPost, "/api/session", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}

	rec := httptest.NewRecorder()
	h.handleIssue(rec, r)

	out := issued{status: rec.Code}
	for _, c := range (&http.Request{Header: http.Header{"Cookie": rec.Header().Values("Set-Cookie")}}).Cookies() {
		if c.Name == cookieName {
			out.cookie = c
		}
	}
	return out
}

// lookupAttributes reads the attributes the session behind cookie holds.
func lookupAttributes(t *testing.T, svc *Service, cookie *http.Cookie) Attributes {
	t.Helper()

	sess, err := svc.Lookup(context.Background(), cookie.Value)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	return sess.Attributes
}

func TestIssueEndpointKeepsTheAttributesOfANewSession(t *testing.T) {
	h, svc := newTestHandler(t)

	res := postSession(t, h, `{"gender":"female","age_band":"20s"}`, nil)
	if res.status != http.StatusCreated {
		t.Fatalf("status = %d, want %d", res.status, http.StatusCreated)
	}

	want := Attributes{Gender: "female", AgeBand: "20s"}
	if got := lookupAttributes(t, svc, res.cookie); got != want {
		t.Fatalf("attributes = %+v, want %+v", got, want)
	}
}

func TestIssueEndpointTakesNoAnswerAsEmptyAttributes(t *testing.T) {
	for _, body := range []string{"", `{}`, `{"gender":null,"age_band":""}`} {
		h, svc := newTestHandler(t)

		res := postSession(t, h, body, nil)
		if res.status != http.StatusCreated {
			t.Fatalf("body %q: status = %d, want %d", body, res.status, http.StatusCreated)
		}
		if got := lookupAttributes(t, svc, res.cookie); got != (Attributes{}) {
			t.Fatalf("body %q: attributes = %+v, want none", body, got)
		}
	}
}

func TestIssueEndpointReplacesTheAttributesOfAValidSession(t *testing.T) {
	h, svc := newTestHandler(t)

	first := postSession(t, h, `{"gender":"male","age_band":"30s"}`, nil)

	// A request without a body leaves the answers as they are.
	if res := postSession(t, h, "", first.cookie); res.status != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.status, http.StatusOK)
	}
	if got := lookupAttributes(t, svc, first.cookie); got != (Attributes{Gender: "male", AgeBand: "30s"}) {
		t.Fatalf("attributes after a request without a body = %+v", got)
	}

	// A body replaces them, and an unanswered field clears its answer.
	res := postSession(t, h, `{"age_band":"40s"}`, first.cookie)
	if res.status != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.status, http.StatusOK)
	}
	if got := res.cookie.Value; got != first.cookie.Value {
		t.Fatal("a valid session was replaced by a new token")
	}
	if got := lookupAttributes(t, svc, first.cookie); got != (Attributes{AgeBand: "40s"}) {
		t.Fatalf("attributes = %+v, want only the age band", got)
	}
}

func TestIssueEndpointRejectsAnUnknownValue(t *testing.T) {
	for _, body := range []string{
		`{"gender":"unknown"}`,
		`{"age_band":"10s"}`,
		`{"gender":"female","region":"tokyo"}`,
		`{"gender":"female"}{"gender":"male"}`,
		`not json`,
	} {
		h, _ := newTestHandler(t)

		res := postSession(t, h, body, nil)
		if res.status != http.StatusBadRequest {
			t.Fatalf("body %q: status = %d, want %d", body, res.status, http.StatusBadRequest)
		}
		if res.cookie != nil {
			t.Fatalf("body %q: a session was issued", body)
		}
	}
}

func TestRedisStoreSetsTheAttributesOfALiveSessionOnly(t *testing.T) {
	store := NewRedisStore(redisTestClient(t))
	ctx := context.Background()

	token := storeTestSession(t, store, testIPHash(t), time.Minute)
	want := Attributes{Gender: "other", AgeBand: "60_plus"}
	if err := store.SetAttributes(ctx, token, want); err != nil {
		t.Fatalf("SetAttributes: %v", err)
	}

	rec, err := store.Get(ctx, token)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rec.Attributes != want {
		t.Fatalf("attributes = %+v, want %+v", rec.Attributes, want)
	}

	gone, err := newToken()
	if err != nil {
		t.Fatalf("newToken: %v", err)
	}
	if err := store.SetAttributes(ctx, gone, want); !errors.Is(err, ErrNotStored) {
		t.Fatalf("SetAttributes of a token that is gone: %v, want ErrNotStored", err)
	}
	if n, _ := store.client.Exists(ctx, sessionKeyPrefix+gone).Result(); n != 0 {
		t.Fatal("SetAttributes brought back a session that was gone")
	}
}
