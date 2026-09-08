package report

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	testConversation = "5f3a1c2e-7e9d-4b0a-9f3e-1c2e7e9d4b0a"
	testToken        = "reporter-token"
)

// fakeRepository keeps the reports the service records, one per reporter per
// conversation like the table it stands for.
type fakeRepository struct {
	added []Report
	err   error
}

func (f *fakeRepository) Add(_ context.Context, r Report) error {
	if f.err != nil {
		return f.err
	}
	for _, stored := range f.added {
		if stored.ConversationID == r.ConversationID && stored.ReporterToken == r.ReporterToken {
			return nil
		}
	}
	f.added = append(f.added, r)
	return nil
}

// fakeParticipation answers for the conversations it was built with.
type fakeParticipation struct {
	members map[string][]string
	err     error
}

func (f *fakeParticipation) IsParticipant(_ context.Context, conversationID, token string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	for _, member := range f.members[conversationID] {
		if member == token {
			return true, nil
		}
	}
	return false, nil
}

// fakeSessions resolves every request to token, and refuses the ones without
// the header a session cookie stands for in these tests.
type fakeSessions struct {
	token string
}

func (f *fakeSessions) Authenticate(r *http.Request) (string, error) {
	if r.Header.Get("X-Test-Session") == "" {
		return "", errors.New("no session")
	}
	return f.token, nil
}

func newTestService(members map[string][]string) (*Service, *fakeRepository) {
	repo := &fakeRepository{}
	return NewService(repo, &fakeParticipation{members: members}), repo
}

func newTestMux(svc *Service) *http.ServeMux {
	mux := http.NewServeMux()
	NewHandler(svc, &fakeSessions{token: testToken}).Register(mux)
	return mux
}

// do sends a report request carrying a session.
func do(mux *http.ServeMux, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/reports", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Test-Session", "yes")

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	return rec
}

func TestSubmitRecordsTheReportOfAParticipant(t *testing.T) {
	svc, repo := newTestService(map[string][]string{testConversation: {"other", testToken}})

	if err := svc.Submit(context.Background(), testConversation, testToken, ReasonContact); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	if len(repo.added) != 1 {
		t.Fatalf("added = %+v, want one report", repo.added)
	}
	want := Report{ConversationID: testConversation, ReporterToken: testToken, Reason: ReasonContact}
	if repo.added[0] != want {
		t.Fatalf("added[0] = %+v, want %+v", repo.added[0], want)
	}
}

func TestSubmitRefusesAReasonOutsideTheFixedSet(t *testing.T) {
	svc, repo := newTestService(map[string][]string{testConversation: {testToken}})

	err := svc.Submit(context.Background(), testConversation, testToken, "whatever")
	if !errors.Is(err, ErrUnknownReason) {
		t.Fatalf("Submit error = %v, want %v", err, ErrUnknownReason)
	}
	if len(repo.added) != 0 {
		t.Fatalf("added = %+v, want nothing", repo.added)
	}
}

func TestSubmitRefusesSomeoneWhoWasNotInTheConversation(t *testing.T) {
	svc, repo := newTestService(map[string][]string{testConversation: {"other"}})

	err := svc.Submit(context.Background(), testConversation, testToken, ReasonSpam)
	if !errors.Is(err, ErrNotParticipant) {
		t.Fatalf("Submit error = %v, want %v", err, ErrNotParticipant)
	}
	if len(repo.added) != 0 {
		t.Fatalf("added = %+v, want nothing", repo.added)
	}
}

func TestSubmitAnswersAnUnknownConversationAsANonParticipant(t *testing.T) {
	svc, _ := newTestService(nil)

	err := svc.Submit(context.Background(), testConversation, testToken, ReasonSpam)
	if !errors.Is(err, ErrNotParticipant) {
		t.Fatalf("Submit error = %v, want %v", err, ErrNotParticipant)
	}
}

func TestSubmitTwiceLeavesOneReport(t *testing.T) {
	svc, repo := newTestService(map[string][]string{testConversation: {testToken}})

	for range 2 {
		if err := svc.Submit(context.Background(), testConversation, testToken, ReasonSexual); err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}

	if len(repo.added) != 1 {
		t.Fatalf("added = %+v, want one report", repo.added)
	}
}

func TestEndpointAnswersARecordedReportWithNoContent(t *testing.T) {
	svc, repo := newTestService(map[string][]string{testConversation: {testToken}})
	mux := newTestMux(svc)

	rec := do(mux, `{"conversation_id":"`+testConversation+`","reason":"`+ReasonHarassment+`"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(repo.added) != 1 {
		t.Fatalf("added = %+v, want one report", repo.added)
	}
}

func TestEndpointAnswersAnUnknownReasonWithBadRequest(t *testing.T) {
	svc, _ := newTestService(map[string][]string{testConversation: {testToken}})

	rec := do(newTestMux(svc), `{"conversation_id":"`+testConversation+`","reason":"nope"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestEndpointAnswersANonParticipantWithForbidden(t *testing.T) {
	svc, _ := newTestService(map[string][]string{testConversation: {"other"}})

	rec := do(newTestMux(svc), `{"conversation_id":"`+testConversation+`","reason":"`+ReasonOther+`"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestEndpointAnswersAnUnknownFieldWithBadRequest(t *testing.T) {
	svc, _ := newTestService(map[string][]string{testConversation: {testToken}})

	rec := do(newTestMux(svc), `{"conversation_id":"`+testConversation+`","participant":2}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestEndpointAnswersAMissingRequiredFieldWithBadRequest(t *testing.T) {
	svc, _ := newTestService(map[string][]string{testConversation: {testToken}})
	mux := newTestMux(svc)

	tests := map[string]string{
		"missing conversation_id": `{"reason":"` + ReasonOther + `"}`,
		"empty conversation_id":   `{"conversation_id":"","reason":"` + ReasonOther + `"}`,
		"missing reason":          `{"conversation_id":"` + testConversation + `"}`,
		"empty reason":            `{"conversation_id":"` + testConversation + `","reason":""}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			rec := do(mux, body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestEndpointAnswersARequestWithoutASessionWithUnauthorized(t *testing.T) {
	svc, _ := newTestService(map[string][]string{testConversation: {testToken}})

	r := httptest.NewRequest(http.MethodPost, "/api/reports", strings.NewReader(`{"reason":"other"}`))
	rec := httptest.NewRecorder()
	newTestMux(svc).ServeHTTP(rec, r)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
