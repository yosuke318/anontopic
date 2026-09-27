package report

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func (s *fakeSanctionStore) reportedIn(identifierType, identifier string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.reported[identifierType+":"+identifier])
}

func TestBanParticipantBansTheIdentifierOfTheNumberGiven(t *testing.T) {
	env := newTestEnv(map[string][]string{testConversation: {"first", "second"}})

	ban, err := env.svc.BanParticipant(context.Background(), testConversation, 2, Imposition{
		IdentifierType: IdentifierIPHash,
		Sanction:       SanctionSuspension,
		Duration:       72 * time.Hour,
		Reason:         "外部への誘導を繰り返している",
	})
	if err != nil {
		t.Fatalf("BanParticipant: %v", err)
	}
	if ban.Identifier != "ip-hash-second" || ban.Source != SourceOperator || ban.ConversationID != testConversation {
		t.Fatalf("ban = %+v, want the address of the second participant, by an operator", ban)
	}
	if !ban.BannedUntil.Equal(env.clock.Now().Add(72 * time.Hour)) {
		t.Fatalf("ban ends at %v, want in 72 hours", ban.BannedUntil)
	}

	if _, err := env.svc.BanParticipant(context.Background(), testConversation, 3, Imposition{
		IdentifierType: IdentifierIPHash, Sanction: SanctionPermanent,
	}); !errors.Is(err, ErrUnknownParticipant) {
		t.Fatalf("err for a third participant = %v, want %v", err, ErrUnknownParticipant)
	}
}

func TestSubmitCountsAgainstEveryParticipantButTheReporter(t *testing.T) {
	env := newTestEnv(map[string][]string{testConversation: {"other", testToken, "third"}})

	if err := env.svc.Submit(context.Background(), testConversation, testToken, ReasonHarassment); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	for token, want := range map[string]int{"other": 1, "third": 1, testToken: 0} {
		if got := env.store.reportedIn(IdentifierDevice, "device-"+token); got != want {
			t.Fatalf("conversations counted against %s = %d, want %d", token, got, want)
		}
	}
}

func TestAdminBanEndpointsImposeListAndLift(t *testing.T) {
	env := newTestEnv(map[string][]string{testConversation: {"first", "second"}})

	rec := env.admin(http.MethodPost, "/api/admin/bans",
		`{"conversation_id":"`+testConversation+`","participant":1,"identifier_type":"device_fingerprint","sanction":"permanent","reason":"荒らし"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body)
	}
	// The identifier is a hash that tells an operator nothing, and stays out.
	if strings.Contains(rec.Body.String(), "device-first") {
		t.Fatalf("response %s holds the identifier", rec.Body)
	}

	rec = env.admin(http.MethodGet, "/api/admin/bans?active=true", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"sanction":"permanent"`) {
		t.Fatalf("GET status = %d, body %s, want the permanent ban", rec.Code, rec.Body)
	}

	rec = env.admin(http.MethodDelete, "/api/admin/bans/1", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"lifted_at":"`) {
		t.Fatalf("DELETE status = %d, body %s, want the lifted ban", rec.Code, rec.Body)
	}

	rec = env.admin(http.MethodGet, "/api/admin/bans?active=true", "")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"bans":[]}` {
		t.Fatalf("GET after the lift = %d, %s, want no active ban", rec.Code, rec.Body)
	}

	for name, c := range map[string]struct {
		method, target, body string
		want                 int
	}{
		"lifting twice":         {http.MethodDelete, "/api/admin/bans/1", "", http.StatusConflict},
		"lifting a ban of none": {http.MethodDelete, "/api/admin/bans/99", "", http.StatusNotFound},
		"an unknown participant": {http.MethodPost, "/api/admin/bans",
			`{"conversation_id":"` + testConversation + `","participant":5,"identifier_type":"ip_hash","sanction":"warning"}`, http.StatusBadRequest},
		"a suspension without a length": {http.MethodPost, "/api/admin/bans",
			`{"conversation_id":"` + testConversation + `","participant":1,"identifier_type":"ip_hash","sanction":"suspension"}`, http.StatusBadRequest},
		"a conversation id that is no id": {http.MethodPost, "/api/admin/bans",
			`{"conversation_id":"1","participant":1,"identifier_type":"ip_hash","sanction":"warning"}`, http.StatusBadRequest},
		"an active flag it cannot read": {http.MethodGet, "/api/admin/bans?active=maybe", "", http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			if rec := env.admin(c.method, c.target, c.body); rec.Code != c.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, c.want, rec.Body)
			}
		})
	}
}
