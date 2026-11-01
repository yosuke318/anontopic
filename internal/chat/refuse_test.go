package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// failingStore is a fakeStore whose Subscribe or Join fails, which is what a
// Redis that cannot be reached does to a room.
type failingStore struct {
	*fakeStore
	subscribe bool
	join      bool
}

func (s failingStore) Subscribe(ctx context.Context, conversationID string) (Subscription, error) {
	if s.subscribe {
		return nil, errors.New("redis is unreachable")
	}
	return s.fakeStore.Subscribe(ctx, conversationID)
}

func (s failingStore) Join(ctx context.Context, seat Seat, now time.Time, ttl time.Duration) ([]string, error) {
	if s.join {
		return nil, errors.New("redis is unreachable")
	}
	return s.fakeStore.Join(ctx, seat, now, ttl)
}

func TestARoomThatCannotBeServedTellsTheClientWhyBeforeClosing(t *testing.T) {
	for name, store := range map[string]failingStore{
		"subscribe": {fakeStore: newFakeStore(), subscribe: true},
		"join":      {fakeStore: newFakeStore(), join: true},
	} {
		t.Run(name, func(t *testing.T) {
			repo := newFakeRepository(tokenAlice, tokenBob)
			srv := newTestServer(t, repo, store, nil, nil, testOptions(), tokenAlice)

			ws := dial(t, srv, repo.conv.ID, tokenAlice)

			ev, _ := readEvent(t, ws)
			if ev.Type != eventError || ev.Code != codeUnavailable {
				t.Fatalf("event = %+v, want an error with code %s", ev, codeUnavailable)
			}

			_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))
			if _, _, err := ws.ReadMessage(); !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				t.Fatalf("read after the error = %v, want the connection closed", err)
			}
		})
	}
}
