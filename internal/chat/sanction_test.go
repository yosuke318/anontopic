package chat

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// stubSanctions bars everyone while banned is set, answers each blocked
// message with the next of outcomes, and keeps a warning waiting for every
// warning it gave until it is taken.
type stubSanctions struct {
	mu       sync.Mutex
	banned   bool
	outcomes []Sanction
	waiting  int
	counted  []Identity
}

func (s *stubSanctions) IsBanned(context.Context, Identity) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.banned, nil
}

func (s *stubSanctions) RecordBlocked(_ context.Context, id Identity) (Sanction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.counted = append(s.counted, id)
	if len(s.outcomes) == 0 {
		return SanctionNone, nil
	}

	outcome := s.outcomes[0]
	s.outcomes = s.outcomes[1:]
	if outcome == SanctionWarning {
		s.waiting++
	}
	return outcome, nil
}

func (s *stubSanctions) TakeWarning(context.Context, Identity) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.waiting == 0 {
		return false, nil
	}
	s.waiting--
	return true, nil
}

func (s *stubSanctions) countedIdentities() []Identity {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Identity(nil), s.counted...)
}

// blockEverything blocks every message as a meetup.
var blockEverything = moderatorFunc(func(context.Context, string) (Verdict, error) {
	return Verdict{Decision: DecisionBlock, Reason: "meetup"}, nil
})

func TestABannedClientIsRefusedAtTheHandshake(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	sanctions := &stubSanctions{banned: true}
	srv := newSanctionedTestServer(t, repo, newFakeStore(), nil, nil, sanctions, testOptions(), tokenAlice, tokenBob)

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/rooms/" + repo.conv.ID
	header := http.Header{"Cookie": []string{"anontopic_session=" + tokenAlice}}

	ws, res, err := websocket.DefaultDialer.Dial(url, header)
	if res != nil {
		defer res.Body.Close()
	}
	if err == nil {
		_ = ws.Close()
		t.Fatal("the handshake of a banned client was accepted")
	}
	if res == nil || res.StatusCode != http.StatusForbidden {
		t.Fatalf("response = %v, want status %d", res, http.StatusForbidden)
	}
}

func TestABlockedMessageThatLeadsToAWarningShowsIt(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	sanctions := &stubSanctions{outcomes: []Sanction{SanctionWarning}}
	srv := newSanctionedTestServer(t, repo, newFakeStore(), blockEverything, nil, sanctions, testOptions(), tokenAlice, tokenBob)

	alice := dial(t, srv, repo.conv.ID, tokenAlice)
	await(t, alice, eventJoined)

	send(t, alice, clientFrame{Type: frameMessage, Body: "会いませんか"})

	await(t, alice, eventError)
	await(t, alice, eventWarning)

	// The offence is counted against what the client connected with.
	want := Identity{IPHash: "test-ip-hash", Device: "test-device"}
	if got := sanctions.countedIdentities(); len(got) != 1 || got[0] != want {
		t.Fatalf("counted %+v, want %+v once", got, want)
	}
}

func TestAWarningWaitingForAParticipantIsShownWhenTheyEnter(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	sanctions := &stubSanctions{waiting: 1}
	srv := newSanctionedTestServer(t, repo, newFakeStore(), nil, nil, sanctions, testOptions(), tokenAlice, tokenBob)

	alice := dial(t, srv, repo.conv.ID, tokenAlice)
	await(t, alice, eventJoined)
	await(t, alice, eventWarning)

	// Showing the warning takes it, so that it is shown once.
	sanctions.mu.Lock()
	defer sanctions.mu.Unlock()
	if sanctions.waiting != 0 {
		t.Fatalf("warnings waiting = %d, want none", sanctions.waiting)
	}
}

func TestABannedParticipantIsCutOffWhileTheRoomCarriesOn(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	sanctions := &stubSanctions{outcomes: []Sanction{SanctionBan}}
	srv := newSanctionedTestServer(t, repo, newFakeStore(), blockEverything, nil, sanctions, testOptions(), tokenAlice, tokenBob)

	alice := dial(t, srv, repo.conv.ID, tokenAlice)
	bob := dial(t, srv, repo.conv.ID, tokenBob)
	await(t, alice, eventJoined)
	await(t, bob, eventJoined)

	send(t, alice, clientFrame{Type: frameMessage, Body: "会いませんか"})

	if ev, _ := await(t, alice, eventEnded); ev.Reason != endReasonBanned {
		t.Fatalf("reason = %q, want %q", ev.Reason, endReasonBanned)
	}
	if _, _, err := alice.ReadMessage(); err == nil {
		t.Fatal("the connection of the banned participant stayed open")
	}

	if ev, _ := await(t, bob, eventParticipantLeft); ev.Participant != 1 {
		t.Fatalf("participant = %d, want 1", ev.Participant)
	}

	// The ban ends the conversation for the banned participant alone.
	if endedAt, _ := repo.ending(); !endedAt.IsZero() {
		t.Fatalf("the conversation was recorded as ended at %v", endedAt)
	}
}

func TestABanImposedWhileConnectedCutsTheParticipantOffAtTheirNextFrame(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	sanctions := &stubSanctions{}

	// The heartbeat is left out of the way, so that it is the frame that
	// finds the ban.
	opts := testOptions()
	opts.PresenceInterval = time.Hour
	srv := newSanctionedTestServer(t, repo, newFakeStore(), nil, nil, sanctions, opts, tokenAlice, tokenBob)

	alice := dial(t, srv, repo.conv.ID, tokenAlice)
	bob := dial(t, srv, repo.conv.ID, tokenBob)
	await(t, alice, eventJoined)
	await(t, bob, eventJoined)

	// A frame sent before the ban goes through.
	send(t, alice, clientFrame{Type: frameMessage, Body: "こんにちは"})
	await(t, bob, eventMessage)

	sanctions.mu.Lock()
	sanctions.banned = true
	sanctions.mu.Unlock()

	send(t, alice, clientFrame{Type: frameMessage, Body: "まだ話せますか"})

	if ev, _ := await(t, alice, eventEnded); ev.Reason != endReasonBanned {
		t.Fatalf("reason = %q, want %q", ev.Reason, endReasonBanned)
	}
	if _, _, err := alice.ReadMessage(); err == nil {
		t.Fatal("the connection of the banned participant stayed open")
	}

	// The frame that found the ban is not delivered, and the room carries on.
	if ev, _ := await(t, bob, eventParticipantLeft); ev.Participant != 1 {
		t.Fatalf("participant = %d, want 1", ev.Participant)
	}
	if got := awaitRecorded(t, repo, 1); len(got) != 1 || got[0].body != "こんにちは" {
		t.Fatalf("recorded %+v, want only the message sent before the ban", got)
	}
}

func TestABanImposedWhileConnectedCutsOffAParticipantWhoOnlyReads(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	sanctions := &stubSanctions{}
	srv := newSanctionedTestServer(t, repo, newFakeStore(), nil, nil, sanctions, testOptions(), tokenAlice, tokenBob)

	alice := dial(t, srv, repo.conv.ID, tokenAlice)
	bob := dial(t, srv, repo.conv.ID, tokenBob)
	await(t, alice, eventJoined)
	await(t, bob, eventJoined)

	// Alice sends nothing at all, so only the heartbeat can find the ban.
	sanctions.mu.Lock()
	sanctions.banned = true
	sanctions.mu.Unlock()

	if ev, _ := await(t, alice, eventEnded); ev.Reason != endReasonBanned {
		t.Fatalf("reason = %q, want %q", ev.Reason, endReasonBanned)
	}
	if _, _, err := alice.ReadMessage(); err == nil {
		t.Fatal("the connection of the banned participant stayed open")
	}
}
