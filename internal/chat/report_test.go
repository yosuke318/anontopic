package chat

import (
	"context"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestEndReportedEndsTheConversationAndTellsTheRoom(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	store := newFakeStore()
	srv := newTestServer(t, repo, store, nil, nil, testOptions(), tokenAlice, tokenBob)

	alice := dial(t, srv, repo.conv.ID, tokenAlice)
	bob := dial(t, srv, repo.conv.ID, tokenBob)
	await(t, alice, eventJoined)
	await(t, bob, eventJoined)

	// The report is taken by whichever server the request reached, which need
	// not be the one holding the room's connections.
	other := NewService(repo, store, nil, nil, testOptions())
	t.Cleanup(func() { _ = other.Close(context.Background()) })

	if err := other.EndReported(t.Context(), repo.conv.ID); err != nil {
		t.Fatalf("EndReported: %v", err)
	}

	for name, conn := range map[string]*websocket.Conn{"alice": alice, "bob": bob} {
		if ev, _ := await(t, conn, eventEnded); ev.Reason != endReasonReported {
			t.Fatalf("%s was told the conversation ended for %q, want %q", name, ev.Reason, endReasonReported)
		}
	}

	if endedAt, reason := repo.ending(); endedAt.IsZero() || reason != endReasonReported {
		t.Fatalf("the conversation was recorded as ended at %v for %q, want a time and %q",
			endedAt, reason, endReasonReported)
	}
}

func TestEndReportedLeavesAConversationThatIsOverAsItEnded(t *testing.T) {
	repo := newFakeRepository(tokenAlice, tokenBob)
	svc := NewService(repo, newFakeStore(), nil, nil, testOptions())
	t.Cleanup(func() { _ = svc.Close(context.Background()) })

	if _, err := repo.End(t.Context(), repo.conv.ID, endReasonUserLeft, time.Now().UTC()); err != nil {
		t.Fatalf("End: %v", err)
	}

	if err := svc.EndReported(t.Context(), repo.conv.ID); err != nil {
		t.Fatalf("EndReported: %v", err)
	}

	if _, reason := repo.ending(); reason != endReasonUserLeft {
		t.Fatalf("reason = %q, want the %q the conversation ended with", reason, endReasonUserLeft)
	}
}
