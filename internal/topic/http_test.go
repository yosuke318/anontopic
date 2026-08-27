package topic

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testAdminToken = "test-admin-token"

// newTestMux serves the module the way main.go does, so that the tests go
// through the routing and the admin guard.
func newTestMux(t *testing.T, repo *fakeRepository, adminToken string) *http.ServeMux {
	t.Helper()

	svc, _ := newTestService(t, repo)
	mux := http.NewServeMux()
	NewHandler(svc, adminToken).Register(mux)

	return mux
}

// do sends a request to mux and returns the recorded answer.
func do(mux *http.ServeMux, method, target, body string, admin bool) *httptest.ResponseRecorder {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, target, nil)
	} else {
		r = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	if admin {
		r.Header.Set("Authorization", adminAuthScheme+testAdminToken)
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

func TestListEndpointAnswersWithTheActiveTopics(t *testing.T) {
	repo := newFakeRepository("雑談", "趣味")
	repo.topics[1].IsActive = false
	mux := newTestMux(t, repo, testAdminToken)

	rec := do(mux, http.MethodGet, "/api/topics", "", false)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := decodeBody[listResponse](t, rec)
	if len(body.Topics) != 1 || body.Topics[0].ID != 1 || body.Topics[0].Name != "雑談" {
		t.Fatalf("topics = %+v, want only 雑談", body.Topics)
	}
}

func TestListEndpointAnswersAnEmptyCatalogueWithAnEmptyArray(t *testing.T) {
	mux := newTestMux(t, newFakeRepository(), testAdminToken)

	rec := do(mux, http.MethodGet, "/api/topics", "", false)
	if got := strings.TrimSpace(rec.Body.String()); got != `{"topics":[]}` {
		t.Fatalf("body = %s, want an empty array", got)
	}
}

func TestAdminEndpointsRejectRequestsWithoutTheToken(t *testing.T) {
	mux := newTestMux(t, newFakeRepository("雑談"), testAdminToken)

	requests := []struct {
		method string
		target string
		body   string
	}{
		{http.MethodGet, "/api/admin/topics", ""},
		{http.MethodPost, "/api/admin/topics", `{"name":"ゲーム"}`},
		{http.MethodPatch, "/api/admin/topics/1", `{"is_active":false}`},
		{http.MethodDelete, "/api/admin/topics/1", ""},
	}
	for _, req := range requests {
		rec := do(mux, req.method, req.target, req.body, false)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want %d", req.method, req.target, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestAdminEndpointsAreNotServedWithoutAConfiguredToken(t *testing.T) {
	mux := newTestMux(t, newFakeRepository("雑談"), "")

	rec := do(mux, http.MethodPost, "/api/admin/topics", `{"name":"ゲーム"}`, true)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestCreateEndpointAddsATopicToTheSelectionScreen(t *testing.T) {
	mux := newTestMux(t, newFakeRepository(), testAdminToken)

	rec := do(mux, http.MethodPost, "/api/admin/topics", `{"name":"ゲーム"}`, true)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	created := decodeBody[adminTopicResponse](t, rec)
	if created.Name != "ゲーム" || !created.IsActive {
		t.Fatalf("created = %+v, want an active ゲーム", created)
	}

	listed := decodeBody[listResponse](t, do(mux, http.MethodGet, "/api/topics", "", false))
	if len(listed.Topics) != 1 || listed.Topics[0].Name != "ゲーム" {
		t.Fatalf("topics = %+v, want ゲーム", listed.Topics)
	}
}

func TestCreateEndpointRejectsAnEmptyName(t *testing.T) {
	mux := newTestMux(t, newFakeRepository(), testAdminToken)

	rec := do(mux, http.MethodPost, "/api/admin/topics", `{"name":"  "}`, true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestUpdateEndpointTakesATopicOffTheSelectionScreen(t *testing.T) {
	mux := newTestMux(t, newFakeRepository("雑談"), testAdminToken)

	if listed := decodeBody[listResponse](t, do(mux, http.MethodGet, "/api/topics", "", false)); len(listed.Topics) != 1 {
		t.Fatalf("topics before the update = %+v, want 雑談", listed.Topics)
	}

	rec := do(mux, http.MethodPatch, "/api/admin/topics/1", `{"is_active":false}`, true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	listed := decodeBody[listResponse](t, do(mux, http.MethodGet, "/api/topics", "", false))
	if len(listed.Topics) != 0 {
		t.Fatalf("topics after the update = %+v, want none", listed.Topics)
	}
}

func TestAdminListEndpointAnswersWithInactiveTopicsToo(t *testing.T) {
	repo := newFakeRepository("雑談", "趣味")
	repo.topics[1].IsActive = false
	mux := newTestMux(t, repo, testAdminToken)

	rec := do(mux, http.MethodGet, "/api/admin/topics", "", true)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := decodeBody[adminListResponse](t, rec)
	if len(body.Topics) != 2 || body.Topics[1].IsActive {
		t.Fatalf("topics = %+v, want 雑談 and the inactive 趣味", body.Topics)
	}
}

func TestDeleteEndpointReportsATopicConversationsReference(t *testing.T) {
	repo := newFakeRepository("雑談")
	repo.deleteErr = ErrInUse
	mux := newTestMux(t, repo, testAdminToken)

	rec := do(mux, http.MethodDelete, "/api/admin/topics/1", "", true)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestDeleteEndpointRemovesATopic(t *testing.T) {
	mux := newTestMux(t, newFakeRepository("雑談"), testAdminToken)

	rec := do(mux, http.MethodDelete, "/api/admin/topics/1", "", true)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	listed := decodeBody[listResponse](t, do(mux, http.MethodGet, "/api/topics", "", false))
	if len(listed.Topics) != 0 {
		t.Fatalf("topics = %+v, want none", listed.Topics)
	}
}

func TestEndpointsRejectAnIDThatIsNotANumber(t *testing.T) {
	mux := newTestMux(t, newFakeRepository("雑談"), testAdminToken)

	rec := do(mux, http.MethodDelete, "/api/admin/topics/abc", "", true)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
