package chat

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const (
	tokenAlice = "token-of-the-first-participant"
	tokenBob   = "token-of-the-second-participant"
)

func TestAMessageReachesAParticipantOnAnotherServer(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	store := newFakeStore()

	// Two servers of one deployment: they share the store and the database,
	// and each holds one of the two connections.
	first := newTestServer(t, repo, store, nil, testOptions(), tokenAlice)
	second := newTestServer(t, repo, store, nil, testOptions(), tokenBob)

	alice := dial(t, first, repo.conv.ID, tokenAlice)
	bob := dial(t, second, repo.conv.ID, tokenBob)

	await(t, alice, eventJoined)
	await(t, bob, eventJoined)

	send(t, alice, clientFrame{Type: frameMessage, Body: "はじめまして"})

	ev, payload := await(t, bob, eventMessage)
	if ev.Body != "はじめまして" {
		t.Fatalf("body = %q, want %q", ev.Body, "はじめまして")
	}
	if ev.Participant != 1 {
		t.Fatalf("participant = %d, want 1", ev.Participant)
	}
	// The other participants are told who spoke, never with what identifies
	// the session that spoke.
	if strings.Contains(string(payload), tokenAlice) {
		t.Fatalf("the session token of the sender reached the room: %s", payload)
	}

	// The sender is served the message the same way, so that every
	// participant reads the room in one order.
	if own, _ := await(t, alice, eventMessage); own.Body != ev.Body || own.ID != ev.ID {
		t.Fatalf("the sender was served %+v, want %+v", own, ev)
	}

	recorded := repo.recorded()
	if len(recorded) != 1 {
		t.Fatalf("recorded %d messages, want 1", len(recorded))
	}
	if recorded[0].senderToken != tokenAlice || recorded[0].flag != moderationFlagClean {
		t.Fatalf("recorded %+v, want the message of %q with flag %d",
			recorded[0], tokenAlice, moderationFlagClean)
	}
}

func TestAParticipantIsAnnouncedToTheRoom(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	store := newFakeStore()
	srv := newTestServer(t, repo, store, nil, testOptions(), tokenAlice, tokenBob)

	alice := dial(t, srv, repo.conv.ID, tokenAlice)
	if ev, _ := await(t, alice, eventJoined); ev.Participant != 1 || len(ev.Present) != 1 {
		t.Fatalf("the first participant was greeted with %+v, want participant 1 alone in the room", ev)
	}

	dial(t, srv, repo.conv.ID, tokenBob)

	// Everything a room is told reaches every participant, so the arrival
	// alice reads first is her own.
	ev, _ := await(t, alice, eventParticipantJoined)
	if ev.Participant == 1 {
		ev, _ = await(t, alice, eventParticipantJoined)
	}

	if ev.Participant != 2 {
		t.Fatalf("participant = %d, want 2", ev.Participant)
	}
	if len(ev.Present) != 2 {
		t.Fatalf("present = %v, want both participants", ev.Present)
	}
}

func TestTheRoomIsToldWhenAParticipantLeavesAndTheConversationEnds(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	store := newFakeStore()

	opts := testOptions()
	opts.RejoinGrace = 20 * time.Millisecond
	srv := newTestServer(t, repo, store, nil, opts, tokenAlice, tokenBob)

	alice := dial(t, srv, repo.conv.ID, tokenAlice)
	bob := dial(t, srv, repo.conv.ID, tokenBob)
	await(t, alice, eventJoined)
	await(t, bob, eventJoined)

	_ = bob.Close()

	if ev, _ := await(t, alice, eventParticipantLeft); ev.Participant != 2 {
		t.Fatalf("participant = %d, want 2", ev.Participant)
	}

	// Nobody is left to talk to, so the conversation ends once the wait for
	// the participant to come back is over.
	if ev, _ := await(t, alice, eventEnded); ev.Reason != endReasonUserLeft {
		t.Fatalf("reason = %q, want %q", ev.Reason, endReasonUserLeft)
	}

	endedAt, reason := repo.ending()
	if endedAt.IsZero() || reason != endReasonUserLeft {
		t.Fatalf("the conversation was recorded as ended at %v for %q, want a time and %q",
			endedAt, reason, endReasonUserLeft)
	}
}

func TestAConversationNobodyIsConnectedToEndsAtOnce(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	store := newFakeStore()
	srv := newTestServer(t, repo, store, nil, testOptions(), tokenAlice)

	alice := dial(t, srv, repo.conv.ID, tokenAlice)
	await(t, alice, eventJoined)

	_ = alice.Close()

	// The last connection to leave records the end itself, because no
	// connection is left to wait out the grace.
	deadline := time.Now().Add(3 * time.Second)
	for {
		if endedAt, reason := repo.ending(); !endedAt.IsZero() {
			if reason != endReasonUserLeft {
				t.Fatalf("reason = %q, want %q", reason, endReasonUserLeft)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the conversation was left open after its last connection went away")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestConnectingAgainKeepsOneParticipant(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	store := newFakeStore()

	srv := newTestServer(t, repo, store, nil, testOptions(), tokenAlice, tokenBob)

	alice := dial(t, srv, repo.conv.ID, tokenAlice)
	bob := dial(t, srv, repo.conv.ID, tokenBob)
	await(t, alice, eventJoined)
	await(t, bob, eventJoined)

	_ = alice.Close()
	await(t, bob, eventParticipantLeft)

	again := dial(t, srv, repo.conv.ID, tokenAlice)
	ev, _ := await(t, again, eventJoined)

	if ev.Participant != 1 {
		t.Fatalf("participant = %d, want 1", ev.Participant)
	}
	if len(ev.Present) != 2 {
		t.Fatalf("present = %v, want the two participants of the room", ev.Present)
	}
	if len(repo.conv.Participants) != 2 {
		t.Fatalf("the conversation holds %d participants, want 2", len(repo.conv.Participants))
	}
}

func TestABlockedMessageIsAnsweredWithoutReachingTheRoom(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	store := newFakeStore()

	blocked := "会いませんか"
	moderator := moderatorFunc(func(_ context.Context, body string) (Decision, error) {
		if body == blocked {
			return DecisionBlock, nil
		}
		return DecisionAllow, nil
	})
	srv := newTestServer(t, repo, store, moderator, testOptions(), tokenAlice, tokenBob)

	alice := dial(t, srv, repo.conv.ID, tokenAlice)
	bob := dial(t, srv, repo.conv.ID, tokenBob)
	await(t, alice, eventJoined)
	await(t, bob, eventJoined)

	send(t, alice, clientFrame{Type: frameMessage, Body: blocked})
	send(t, alice, clientFrame{Type: frameMessage, Body: "こんばんは"})

	if ev, _ := await(t, alice, eventError); ev.Code != codeBlocked {
		t.Fatalf("code = %q, want %q", ev.Code, codeBlocked)
	}

	// The message the room does receive is the one that was not blocked, so
	// the blocked one reached nobody.
	if ev, _ := await(t, bob, eventMessage); ev.Body != "こんばんは" {
		t.Fatalf("body = %q, want %q", ev.Body, "こんばんは")
	}

	recorded := repo.recorded()
	if len(recorded) != 1 || recorded[0].body != "こんばんは" {
		t.Fatalf("recorded %+v, want the message that was not blocked alone", recorded)
	}
}

func TestAFlaggedMessageIsDeliveredAndRecordedWithItsFlag(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	store := newFakeStore()

	moderator := moderatorFunc(func(_ context.Context, _ string) (Decision, error) {
		return DecisionFlag, nil
	})
	srv := newTestServer(t, repo, store, moderator, testOptions(), tokenAlice)

	alice := dial(t, srv, repo.conv.ID, tokenAlice)
	await(t, alice, eventJoined)

	send(t, alice, clientFrame{Type: frameMessage, Body: "怪しい話"})
	await(t, alice, eventMessage)

	recorded := repo.recorded()
	if len(recorded) != 1 || recorded[0].flag != moderationFlagNG {
		t.Fatalf("recorded %+v, want one message with flag %d", recorded, moderationFlagNG)
	}
}

func TestAFrameTheRoomCannotReadIsAnswered(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	store := newFakeStore()
	srv := newTestServer(t, repo, store, nil, testOptions(), tokenAlice)

	alice := dial(t, srv, repo.conv.ID, tokenAlice)
	await(t, alice, eventJoined)

	tests := []struct {
		name  string
		frame clientFrame
		want  string
	}{
		{"an empty message", clientFrame{Type: frameMessage, Body: "   "}, codeEmptyBody},
		{"a message over the limit", clientFrame{Type: frameMessage, Body: strings.Repeat("あ", maxMessageRunes+1)}, codeTooLong},
		{"a frame of an unknown type", clientFrame{Type: "shout", Body: "hello"}, codeInvalidFrame},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			send(t, alice, tc.frame)

			if ev, _ := await(t, alice, eventError); ev.Code != tc.want {
				t.Fatalf("code = %q, want %q", ev.Code, tc.want)
			}
		})
	}

	if recorded := repo.recorded(); len(recorded) != 0 {
		t.Fatalf("recorded %+v, want nothing", recorded)
	}
}

func TestAdmitRefusesAnyoneTheConversationWasNotFormedFor(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	svc := NewService(repo, newFakeStore(), nil, testOptions())

	adm, err := svc.Admit(context.Background(), repo.conv.ID, tokenBob)
	if err != nil {
		t.Fatalf("Admit(a participant) = %v, want an admission", err)
	}
	if adm.Participant != 2 {
		t.Fatalf("participant = %d, want 2", adm.Participant)
	}

	if _, err := svc.Admit(context.Background(), repo.conv.ID, "someone-elses-token"); !errors.Is(err, ErrNotParticipant) {
		t.Fatalf("Admit(a stranger) = %v, want %v", err, ErrNotParticipant)
	}
	if _, err := svc.Admit(context.Background(), "1c8f", tokenAlice); !errors.Is(err, ErrConversationNotFound) {
		t.Fatalf("Admit(an unknown conversation) = %v, want %v", err, ErrConversationNotFound)
	}

	if _, err := repo.End(context.Background(), repo.conv.ID, endReasonUserLeft, time.Now().UTC()); err != nil {
		t.Fatalf("end the conversation: %v", err)
	}
	if _, err := svc.Admit(context.Background(), repo.conv.ID, tokenAlice); !errors.Is(err, ErrConversationEnded) {
		t.Fatalf("Admit(an ended conversation) = %v, want %v", err, ErrConversationEnded)
	}
}

func TestNumbersOfNamesParticipantsByTheirPlaceInTheConversation(t *testing.T) {
	conv := Conversation{Participants: []string{"first", "second", "third"}}

	tests := map[string]struct {
		tokens []string
		want   []int
	}{
		"the whole room":          {tokens: []string{"third", "first", "second"}, want: []int{1, 2, 3}},
		"one participant":         {tokens: []string{"second"}, want: []int{2}},
		"nobody":                  {tokens: nil, want: []int{}},
		"a token of another room": {tokens: []string{"first", "stranger"}, want: []int{1}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := conv.numbersOf(tc.tokens)
			if len(got) != len(tc.want) {
				t.Fatalf("numbersOf(%v) = %v, want %v", tc.tokens, got, tc.want)
			}
			for i, number := range got {
				if number != tc.want[i] {
					t.Fatalf("numbersOf(%v) = %v, want %v", tc.tokens, got, tc.want)
				}
			}
		})
	}
}
