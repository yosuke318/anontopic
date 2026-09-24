package report

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	testConversation = "5f3a1c2e-7e9d-4b0a-9f3e-1c2e7e9d4b0a"
	testToken        = "reporter-token"
)

// fakeRepository keeps the reports and claims the service records, one report
// per reporter per conversation like the table it stands for.
type fakeRepository struct {
	reports []Report
	claims  []Claim
	err     error
}

func (f *fakeRepository) Add(_ context.Context, r Report) error {
	if f.err != nil {
		return f.err
	}
	for _, stored := range f.reports {
		if stored.ConversationID == r.ConversationID && stored.ReporterToken == r.ReporterToken {
			return nil
		}
	}
	r.ID = int64(len(f.reports) + 1)
	r.Status = StatusOpen
	r.CreatedAt = time.Now().UTC()
	f.reports = append(f.reports, r)
	return nil
}

func (f *fakeRepository) List(_ context.Context, flt Filter) ([]Report, error) {
	var out []Report
	for _, r := range slices.Backward(f.reports) {
		if flt.Status != "" && r.Status != flt.Status {
			continue
		}
		if flt.ConversationID != "" && r.ConversationID != flt.ConversationID {
			continue
		}
		if flt.BeforeID > 0 && r.ID >= flt.BeforeID {
			continue
		}
		out = append(out, r)
	}
	if len(out) > flt.Limit {
		out = out[:flt.Limit]
	}
	return out, nil
}

func (f *fakeRepository) Get(_ context.Context, id int64) (Report, error) {
	for _, r := range f.reports {
		if r.ID == id {
			return r, nil
		}
	}
	return Report{}, ErrNotFound
}

func (f *fakeRepository) SetStatus(_ context.Context, id int64, status string) (Report, error) {
	for i := range f.reports {
		if f.reports[i].ID == id {
			f.reports[i].Status = status
			return f.reports[i], nil
		}
	}
	return Report{}, ErrNotFound
}

func (f *fakeRepository) AddClaim(_ context.Context, c Claim) (Claim, error) {
	if f.err != nil {
		return Claim{}, f.err
	}
	c.ID = int64(len(f.claims) + 1)
	c.CreatedAt = time.Now().UTC()
	f.claims = append(f.claims, c)
	return c, nil
}

func (f *fakeRepository) ListClaims(_ context.Context, flt Filter) ([]Claim, error) {
	var out []Claim
	for _, c := range slices.Backward(f.claims) {
		if flt.Status != "" && c.Status != flt.Status {
			continue
		}
		if flt.BeforeID > 0 && c.ID >= flt.BeforeID {
			continue
		}
		out = append(out, c)
	}
	if len(out) > flt.Limit {
		out = out[:flt.Limit]
	}
	return out, nil
}

func (f *fakeRepository) SetClaimStatus(_ context.Context, id int64, status string) (Claim, error) {
	for i := range f.claims {
		if f.claims[i].ID == id {
			f.claims[i].Status = status
			return f.claims[i], nil
		}
	}
	return Claim{}, ErrNotFound
}

// fakeConversations answers for the conversations it was built with, and
// remembers which were flagged and ended.
type fakeConversations struct {
	members  map[string][]string
	messages []TranscriptMessage
	err      error
	flagErr  error

	flagged map[string]string
	ended   []string
}

func (f *fakeConversations) IsParticipant(_ context.Context, conversationID, token string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return slices.Contains(f.members[conversationID], token), nil
}

func (f *fakeConversations) Flag(_ context.Context, conversationID, reporterToken string) error {
	if f.flagErr != nil {
		return f.flagErr
	}
	if f.flagged == nil {
		f.flagged = make(map[string]string)
	}
	f.flagged[conversationID] = reporterToken
	return nil
}

func (f *fakeConversations) EndReported(_ context.Context, conversationID string) error {
	if !slices.Contains(f.ended, conversationID) {
		f.ended = append(f.ended, conversationID)
	}
	return nil
}

func (f *fakeConversations) Transcript(_ context.Context, conversationID string) (Transcript, error) {
	return Transcript{
		ConversationID: conversationID,
		TopicID:        1,
		RoomType:       len(f.members[conversationID]),
		StartedAt:      time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC),
		Flagged:        f.flagged[conversationID] != "",
		Participants:   f.members[conversationID],
		Messages:       f.messages,
	}, nil
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

func (f *fakeSessions) IPHash(*http.Request) string {
	return "test-ip-hash"
}

// fakeClaimLimiter lets through as many claims as it has left.
type fakeClaimLimiter struct {
	left int
}

func (f *fakeClaimLimiter) AllowClaim(context.Context, string) (bool, time.Duration, error) {
	if f.left <= 0 {
		return false, 10 * time.Minute, nil
	}
	f.left--
	return true, 0, nil
}

const testAdminToken = "test-admin-token"

type testEnv struct {
	svc   *Service
	repo  *fakeRepository
	convs *fakeConversations
	mux   *http.ServeMux
}

func newTestEnv(members map[string][]string) *testEnv {
	repo := &fakeRepository{}
	convs := &fakeConversations{members: members}
	svc := NewService(repo, convs)

	mux := http.NewServeMux()
	NewHandler(svc, &fakeSessions{token: testToken}, &fakeClaimLimiter{left: 2}, testAdminToken).Register(mux)

	return &testEnv{svc: svc, repo: repo, convs: convs, mux: mux}
}

// do sends a report request carrying a session.
func (e *testEnv) do(body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/reports", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Test-Session", "yes")

	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, r)
	return rec
}

// admin sends a request carrying the admin token.
func (e *testEnv) admin(method, target, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+testAdminToken)

	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, r)
	return rec
}

// claim sends a claim without a session, like someone who never used the
// service would.
func (e *testEnv) claim(body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/api/claims", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, r)
	return rec
}

func TestSubmitRecordsKeepsAndEndsTheReportedConversation(t *testing.T) {
	env := newTestEnv(map[string][]string{testConversation: {"other", testToken}})

	if err := env.svc.Submit(context.Background(), testConversation, testToken, ReasonContact); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	if len(env.repo.reports) != 1 {
		t.Fatalf("reports = %+v, want one report", env.repo.reports)
	}
	got := env.repo.reports[0]
	if got.ConversationID != testConversation || got.ReporterToken != testToken || got.Reason != ReasonContact {
		t.Fatalf("reports[0] = %+v, want the report of %s by %s for %s",
			got, testConversation, testToken, ReasonContact)
	}
	if env.convs.flagged[testConversation] != testToken {
		t.Fatalf("flagged = %v, want %s flagged by the reporter", env.convs.flagged, testConversation)
	}
	if !slices.Equal(env.convs.ended, []string{testConversation}) {
		t.Fatalf("ended = %v, want %s", env.convs.ended, testConversation)
	}
}

func TestSubmitTakesEveryReasonOfTheFixedSet(t *testing.T) {
	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			env := newTestEnv(map[string][]string{testConversation: {testToken}})
			if err := env.svc.Submit(context.Background(), testConversation, testToken, reason); err != nil {
				t.Fatalf("Submit: %v", err)
			}
		})
	}
}

func TestSubmitRefusesAReasonOutsideTheFixedSet(t *testing.T) {
	env := newTestEnv(map[string][]string{testConversation: {testToken}})

	err := env.svc.Submit(context.Background(), testConversation, testToken, "whatever")
	if !errors.Is(err, ErrUnknownReason) {
		t.Fatalf("Submit error = %v, want %v", err, ErrUnknownReason)
	}
	if len(env.repo.reports) != 0 || len(env.convs.ended) != 0 {
		t.Fatalf("reports = %+v, ended = %v, want nothing done", env.repo.reports, env.convs.ended)
	}
}

func TestSubmitRefusesSomeoneWhoWasNotInTheConversation(t *testing.T) {
	env := newTestEnv(map[string][]string{testConversation: {"other"}})

	err := env.svc.Submit(context.Background(), testConversation, testToken, ReasonSpam)
	if !errors.Is(err, ErrNotParticipant) {
		t.Fatalf("Submit error = %v, want %v", err, ErrNotParticipant)
	}
	if len(env.repo.reports) != 0 || len(env.convs.flagged) != 0 || len(env.convs.ended) != 0 {
		t.Fatal("a stranger's report touched the conversation")
	}
}

func TestSubmitAnswersAnUnknownConversationAsANonParticipant(t *testing.T) {
	env := newTestEnv(nil)

	err := env.svc.Submit(context.Background(), testConversation, testToken, ReasonSpam)
	if !errors.Is(err, ErrNotParticipant) {
		t.Fatalf("Submit error = %v, want %v", err, ErrNotParticipant)
	}
}

func TestSubmitTwiceLeavesOneReport(t *testing.T) {
	env := newTestEnv(map[string][]string{testConversation: {testToken}})

	for range 2 {
		if err := env.svc.Submit(context.Background(), testConversation, testToken, ReasonSexual); err != nil {
			t.Fatalf("Submit: %v", err)
		}
	}

	if len(env.repo.reports) != 1 {
		t.Fatalf("reports = %+v, want one report", env.repo.reports)
	}
}

func TestSubmitThatCouldNotFlagIsCompletedBySendingItAgain(t *testing.T) {
	env := newTestEnv(map[string][]string{testConversation: {testToken}})
	env.convs.flagErr = errors.New("database is down")

	if err := env.svc.Submit(context.Background(), testConversation, testToken, ReasonSpam); err == nil {
		t.Fatal("Submit succeeded, want the flag's error")
	}
	if len(env.convs.ended) != 0 {
		t.Fatal("the conversation ended before it was flagged")
	}

	env.convs.flagErr = nil
	if err := env.svc.Submit(context.Background(), testConversation, testToken, ReasonSpam); err != nil {
		t.Fatalf("Submit again: %v", err)
	}
	if len(env.repo.reports) != 1 || env.convs.flagged[testConversation] == "" || len(env.convs.ended) != 1 {
		t.Fatalf("reports = %d, flagged = %v, ended = %v, want one report, flagged and ended",
			len(env.repo.reports), env.convs.flagged, env.convs.ended)
	}
}

func TestReviewNamesParticipantsByTheirNumber(t *testing.T) {
	env := newTestEnv(map[string][]string{testConversation: {"first", testToken, "third"}})
	env.convs.messages = []TranscriptMessage{
		{SenderToken: "first", Body: "こんにちは", Flag: 2},
		{SenderToken: testToken, Body: "やめてください", Flag: 0},
		{SenderToken: "third", Body: "会いませんか", Flag: 1},
	}
	if err := env.svc.Submit(context.Background(), testConversation, testToken, ReasonDating); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	review, err := env.svc.Review(context.Background(), 1)
	if err != nil {
		t.Fatalf("Review: %v", err)
	}

	if review.Reporter != 2 {
		t.Fatalf("reporter = %d, want 2", review.Reporter)
	}
	var senders []int
	for _, msg := range review.Messages {
		senders = append(senders, msg.Participant)
	}
	if !slices.Equal(senders, []int{1, 2, 3}) {
		t.Fatalf("senders = %v, want [1 2 3]", senders)
	}
}

func TestReviewAnswersAReportThatDoesNotExist(t *testing.T) {
	env := newTestEnv(nil)

	if _, err := env.svc.Review(context.Background(), 42); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Review error = %v, want %v", err, ErrNotFound)
	}
}

func TestUpdateStatusRefusesAStatusOutsideTheFixedSet(t *testing.T) {
	env := newTestEnv(map[string][]string{testConversation: {testToken}})
	if err := env.svc.Submit(context.Background(), testConversation, testToken, ReasonSpam); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	if _, err := env.svc.UpdateStatus(context.Background(), 1, "closed"); !errors.Is(err, ErrUnknownStatus) {
		t.Fatalf("UpdateStatus error = %v, want %v", err, ErrUnknownStatus)
	}
}

func TestListBoundsTheNumberOfReports(t *testing.T) {
	tests := map[string]struct {
		limit int
		want  int
	}{
		"no limit":       {limit: 0, want: DefaultListLimit},
		"a small limit":  {limit: 3, want: 3},
		"too big to use": {limit: MaxListLimit + 1, want: MaxListLimit},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			f, err := normalizeFilter(Filter{Limit: tt.limit})
			if err != nil {
				t.Fatalf("normalizeFilter: %v", err)
			}
			if f.Limit != tt.want {
				t.Fatalf("limit = %d, want %d", f.Limit, tt.want)
			}
		})
	}
}

func TestSubmitClaimRecordsAnOpenClaimWithTrimmedFields(t *testing.T) {
	env := newTestEnv(nil)

	created, err := env.svc.SubmitClaim(context.Background(), Claim{
		Name:    "  山田 太郎 ",
		Email:   " taro@example.com ",
		Right:   RightPrivacy,
		Details: "  私の住所が書き込まれていました。 ",
	})
	if err != nil {
		t.Fatalf("SubmitClaim: %v", err)
	}

	if created.Name != "山田 太郎" || created.Email != "taro@example.com" || created.Details != "私の住所が書き込まれていました。" {
		t.Fatalf("created = %+v, want the fields without surrounding spaces", created)
	}
	if created.Status != StatusOpen {
		t.Fatalf("status = %q, want %q", created.Status, StatusOpen)
	}
}

func TestListClaimsReadsThePagesOfTheClaimsNewestFirst(t *testing.T) {
	env := newTestEnv(nil)
	for _, name := range []string{"一人目", "二人目", "三人目"} {
		if _, err := env.svc.SubmitClaim(context.Background(), Claim{
			Name:    name,
			Email:   "taro@example.com",
			Right:   RightOther,
			Details: "内容",
		}); err != nil {
			t.Fatalf("SubmitClaim: %v", err)
		}
	}

	page, err := env.svc.ListClaims(context.Background(), Filter{Limit: 2})
	if err != nil {
		t.Fatalf("ListClaims: %v", err)
	}
	if len(page) != 2 || page[0].Name != "三人目" || page[1].Name != "二人目" {
		t.Fatalf("page = %+v, want the two newest claims", page)
	}

	next, err := env.svc.ListClaims(context.Background(), Filter{BeforeID: page[1].ID, Limit: 2})
	if err != nil {
		t.Fatalf("ListClaims before: %v", err)
	}
	if len(next) != 1 || next[0].Name != "一人目" {
		t.Fatalf("next = %+v, want the oldest claim alone", next)
	}
}

func TestSubmitClaimRefusesAClaimOutOfBounds(t *testing.T) {
	valid := Claim{Name: "山田 太郎", Email: "taro@example.com", Right: RightDefamation, Details: "内容"}

	tests := map[string]func(c *Claim){
		"no name":              func(c *Claim) { c.Name = " " },
		"a name too long":      func(c *Claim) { c.Name = strings.Repeat("あ", maxClaimNameRunes+1) },
		"no email":             func(c *Claim) { c.Email = "" },
		"an email without @":   func(c *Claim) { c.Email = "taro.example.com" },
		"an email with a name": func(c *Claim) { c.Email = "Taro <taro@example.com>" },
		"an unknown right":     func(c *Claim) { c.Right = "trademark" },
		"no details":           func(c *Claim) { c.Details = "" },
		"details too long":     func(c *Claim) { c.Details = strings.Repeat("あ", maxClaimDetailsRunes+1) },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			env := newTestEnv(nil)
			c := valid
			change(&c)

			if _, err := env.svc.SubmitClaim(context.Background(), c); !errors.Is(err, ErrInvalidClaim) {
				t.Fatalf("SubmitClaim error = %v, want %v", err, ErrInvalidClaim)
			}
			if len(env.repo.claims) != 0 {
				t.Fatalf("claims = %+v, want nothing recorded", env.repo.claims)
			}
		})
	}
}

func TestEndpointAnswersARecordedReportWithNoContent(t *testing.T) {
	env := newTestEnv(map[string][]string{testConversation: {testToken}})

	rec := env.do(`{"conversation_id":"` + testConversation + `","reason":"` + ReasonHarassment + `"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if len(env.repo.reports) != 1 {
		t.Fatalf("reports = %+v, want one report", env.repo.reports)
	}
}

func TestEndpointAnswersAnUnknownReasonWithBadRequest(t *testing.T) {
	env := newTestEnv(map[string][]string{testConversation: {testToken}})

	rec := env.do(`{"conversation_id":"` + testConversation + `","reason":"nope"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestEndpointAnswersANonParticipantWithForbidden(t *testing.T) {
	env := newTestEnv(map[string][]string{testConversation: {"other"}})

	rec := env.do(`{"conversation_id":"` + testConversation + `","reason":"` + ReasonOther + `"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestEndpointAnswersAnUnknownFieldWithBadRequest(t *testing.T) {
	env := newTestEnv(map[string][]string{testConversation: {testToken}})

	rec := env.do(`{"conversation_id":"` + testConversation + `","participant":2}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestEndpointAnswersAMissingRequiredFieldWithBadRequest(t *testing.T) {
	env := newTestEnv(map[string][]string{testConversation: {testToken}})

	tests := map[string]string{
		"missing conversation_id": `{"reason":"` + ReasonOther + `"}`,
		"empty conversation_id":   `{"conversation_id":"","reason":"` + ReasonOther + `"}`,
		"missing reason":          `{"conversation_id":"` + testConversation + `"}`,
		"empty reason":            `{"conversation_id":"` + testConversation + `","reason":""}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			rec := env.do(body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestEndpointAnswersARequestWithoutASessionWithUnauthorized(t *testing.T) {
	env := newTestEnv(map[string][]string{testConversation: {testToken}})

	r := httptest.NewRequest(http.MethodPost, "/api/reports", strings.NewReader(`{"reason":"other"}`))
	rec := httptest.NewRecorder()
	env.mux.ServeHTTP(rec, r)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestAdminEndpointsAreNotServedWithoutAToken(t *testing.T) {
	mux := http.NewServeMux()
	svc := NewService(&fakeRepository{}, &fakeConversations{})
	NewHandler(svc, &fakeSessions{}, &fakeClaimLimiter{}, "").Register(mux)

	r := httptest.NewRequest(http.MethodGet, "/api/admin/reports", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAdminEndpointsRefuseARequestWithoutTheToken(t *testing.T) {
	env := newTestEnv(nil)

	for _, target := range []string{"/api/admin/reports", "/api/admin/reports/1", "/api/admin/claims"} {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		r.Header.Set("Authorization", "Bearer wrong")
		rec := httptest.NewRecorder()
		env.mux.ServeHTTP(rec, r)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("GET %s status = %d, want %d", target, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestAdminListLeavesTheReporterTokenOut(t *testing.T) {
	env := newTestEnv(map[string][]string{testConversation: {testToken}})
	if err := env.svc.Submit(context.Background(), testConversation, testToken, ReasonSpam); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	rec := env.admin(http.MethodGet, "/api/admin/reports?status=open", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), testConversation) {
		t.Fatalf("body = %s, want the report of %s", rec.Body, testConversation)
	}
	if strings.Contains(rec.Body.String(), testToken) {
		t.Fatalf("body = %s, want no session token", rec.Body)
	}
}

func TestAdminListAnswersAQueryItCannotReadWithBadRequest(t *testing.T) {
	env := newTestEnv(nil)

	for _, query := range []string{"status=closed", "before=x", "limit=-1", "conversation_id=1c8f"} {
		rec := env.admin(http.MethodGet, "/api/admin/reports?"+query, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("?%s status = %d, want %d", query, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestAdminReviewAnswersWithTheConversationLog(t *testing.T) {
	env := newTestEnv(map[string][]string{testConversation: {"other", testToken}})
	env.convs.messages = []TranscriptMessage{{SenderToken: "other", Body: "会いませんか", Flag: 2}}
	if err := env.svc.Submit(context.Background(), testConversation, testToken, ReasonDating); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	rec := env.admin(http.MethodGet, "/api/admin/reports/1", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	for _, want := range []string{`"reporter":2`, `"participant":1`, `"body":"会いませんか"`, `"moderation_flag":2`, `"is_flagged":true`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("body = %s, want it to hold %s", rec.Body, want)
		}
	}
	if strings.Contains(rec.Body.String(), testToken) {
		t.Fatalf("body = %s, want no session token", rec.Body)
	}

	if rec := env.admin(http.MethodGet, "/api/admin/reports/9", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("an unknown report answered %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestAdminUpdateMovesAReportToAnotherStatus(t *testing.T) {
	env := newTestEnv(map[string][]string{testConversation: {testToken}})
	if err := env.svc.Submit(context.Background(), testConversation, testToken, ReasonSpam); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	tests := []struct {
		target string
		body   string
		want   int
	}{
		{"/api/admin/reports/1", `{"status":"reviewing"}`, http.StatusOK},
		{"/api/admin/reports/1", `{"status":"closed"}`, http.StatusBadRequest},
		{"/api/admin/reports/9", `{"status":"actioned"}`, http.StatusNotFound},
		{"/api/admin/reports/x", `{"status":"actioned"}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		if rec := env.admin(http.MethodPatch, tt.target, tt.body); rec.Code != tt.want {
			t.Fatalf("PATCH %s %s status = %d, want %d", tt.target, tt.body, rec.Code, tt.want)
		}
	}

	if env.repo.reports[0].Status != StatusReviewing {
		t.Fatalf("status = %q, want %q", env.repo.reports[0].Status, StatusReviewing)
	}
}

func TestClaimEndpointTakesAClaimWithoutASession(t *testing.T) {
	env := newTestEnv(nil)

	rec := env.claim(`{"name":"山田 太郎","email":"taro@example.com","right":"privacy","details":"住所が書かれていた"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusNoContent, rec.Body)
	}
	if len(env.repo.claims) != 1 {
		t.Fatalf("claims = %+v, want one claim", env.repo.claims)
	}
}

func TestClaimEndpointAnswersAnInvalidClaimWithBadRequest(t *testing.T) {
	env := newTestEnv(nil)

	rec := env.claim(`{"name":"山田 太郎","email":"not an address","right":"privacy","details":"内容"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestClaimEndpointHoldsBackAnAddressOverItsRate(t *testing.T) {
	env := newTestEnv(nil)
	body := `{"name":"山田 太郎","email":"taro@example.com","right":"other","details":"内容"}`

	for range 2 {
		if rec := env.claim(body); rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
		}
	}

	rec := env.claim(body)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if got := rec.Header().Get("Retry-After"); got != "600" {
		t.Fatalf("Retry-After = %q, want 600", got)
	}
}

func TestClaimEndpointKeepsTheAllowanceOfABodyItCannotRead(t *testing.T) {
	// The limiter of this environment lets two claims through.
	env := newTestEnv(nil)

	for _, body := range []string{`{"name":`, `{"name":"山田 太郎","surname":"太郎"}`} {
		if rec := env.claim(body); rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d for %s", rec.Code, http.StatusBadRequest, body)
		}
	}

	valid := `{"name":"山田 太郎","email":"taro@example.com","right":"other","details":"内容"}`
	for range 2 {
		if rec := env.claim(valid); rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want %d: the allowance was spent on a body that held no claim",
				rec.Code, http.StatusNoContent)
		}
	}
}

func TestAdminClaimEndpointsListAndMoveClaims(t *testing.T) {
	env := newTestEnv(nil)
	if rec := env.claim(`{"name":"山田 太郎","email":"taro@example.com","right":"copyright","details":"内容"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("submit status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	rec := env.admin(http.MethodGet, "/api/admin/claims?status=open", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"email":"taro@example.com"`) {
		t.Fatalf("list = %d %s, want the claim", rec.Code, rec.Body)
	}

	if rec := env.admin(http.MethodPatch, "/api/admin/claims/1", `{"status":"actioned"}`); rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d", rec.Code, http.StatusOK)
	}
	if env.repo.claims[0].Status != StatusActioned {
		t.Fatalf("status = %q, want %q", env.repo.claims[0].Status, StatusActioned)
	}
}
