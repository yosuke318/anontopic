package chat

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// stubAuthenticator accepts the tokens it was built with and rejects
// everything else. It counts its calls so that a test can tell whether the
// session was touched at all.
type stubAuthenticator struct {
	tokens []string

	mu    sync.Mutex
	calls int
}

func (a *stubAuthenticator) Authenticate(r *http.Request) (string, error) {
	a.mu.Lock()
	a.calls++
	a.mu.Unlock()

	c, err := r.Cookie("anontopic_session")
	if err != nil || !slices.Contains(a.tokens, c.Value) {
		return "", errors.New("invalid session")
	}
	return c.Value, nil
}

func (a *stubAuthenticator) callCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()

	return a.calls
}

// fakeStore keeps the rooms in memory. Two services sharing one fakeStore
// stand for two servers reading the same Redis.
type fakeStore struct {
	mu       sync.Mutex
	subs     map[string][]*fakeSubscription
	presence map[string]map[string]time.Time
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		subs:     make(map[string][]*fakeSubscription),
		presence: make(map[string]map[string]time.Time),
	}
}

func (s *fakeStore) Publish(_ context.Context, conversationID string, payload []byte) error {
	s.mu.Lock()
	subs := slices.Clone(s.subs[conversationID])
	s.mu.Unlock()

	for _, sub := range subs {
		sub.deliver(payload)
	}
	return nil
}

func (s *fakeStore) Subscribe(_ context.Context, conversationID string) (Subscription, error) {
	sub := &fakeSubscription{
		store:          s,
		conversationID: conversationID,
		events:         make(chan []byte, 64),
	}

	s.mu.Lock()
	s.subs[conversationID] = append(s.subs[conversationID], sub)
	s.mu.Unlock()

	return sub, nil
}

func (s *fakeStore) Join(_ context.Context, conversationID, token string, now time.Time, ttl time.Duration) ([]string, error) {
	return s.record(conversationID, token, now, ttl, true), nil
}

func (s *fakeStore) Heartbeat(_ context.Context, conversationID, token string, now time.Time, ttl time.Duration) ([]string, error) {
	return s.record(conversationID, token, now, ttl, true), nil
}

func (s *fakeStore) Leave(_ context.Context, conversationID, token string, now time.Time, ttl time.Duration) ([]string, error) {
	return s.record(conversationID, token, now, ttl, false), nil
}

// record keeps one token connected or drops it, and answers with the tokens
// whose entry is still fresh.
func (s *fakeStore) record(conversationID, token string, now time.Time, ttl time.Duration, connected bool) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	room, ok := s.presence[conversationID]
	if !ok {
		room = make(map[string]time.Time)
		s.presence[conversationID] = room
	}

	if connected {
		room[token] = now
	} else {
		delete(room, token)
	}

	tokens := make([]string, 0, len(room))
	for t, at := range room {
		// The Redis store drops an entry that is exactly ttl old, so this one
		// has to drop it as well for a test to exercise the same boundary.
		if now.Sub(at) >= ttl {
			delete(room, t)
			continue
		}
		tokens = append(tokens, t)
	}
	slices.Sort(tokens)

	return tokens
}

// fakeSubscription is one server's stream of a room's events.
type fakeSubscription struct {
	store          *fakeStore
	conversationID string
	events         chan []byte

	mu     sync.Mutex
	closed bool
}

// deliver hands one event to the server holding this subscription.
func (s *fakeSubscription) deliver(payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	select {
	case s.events <- payload:
	default:
	}
}

func (s *fakeSubscription) Events() <-chan []byte { return s.events }

func (s *fakeSubscription) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true
	close(s.events)

	s.store.mu.Lock()
	subs := s.store.subs[s.conversationID]
	s.store.subs[s.conversationID] = slices.DeleteFunc(subs, func(other *fakeSubscription) bool {
		return other == s
	})
	s.store.mu.Unlock()

	return nil
}

// fakeRepository holds one conversation and what was recorded in it.
type fakeRepository struct {
	mu        sync.Mutex
	conv      Conversation
	messages  []recordedMessage
	endedAt   time.Time
	endReason string
}

// recordedMessage is one message as the repository stored it.
type recordedMessage struct {
	senderToken string
	body        string
	flag        int
}

func newFakeRepository(participants ...string) *fakeRepository {
	return &fakeRepository{
		conv: Conversation{
			ID:           "7f1fca8f-ca30-40c7-a752-ef5ffa6bac2a",
			TopicID:      1,
			RoomType:     len(participants),
			StartedAt:    time.Now().UTC(),
			Participants: participants,
		},
	}
}

func (r *fakeRepository) Conversation(_ context.Context, id string) (Conversation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if id != r.conv.ID {
		return Conversation{}, ErrConversationNotFound
	}

	conv := r.conv
	conv.Participants = slices.Clone(r.conv.Participants)
	conv.EndedAt = r.endedAt

	return conv, nil
}

func (r *fakeRepository) AddMessage(_ context.Context, _, senderToken, body string, flag int) (Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.messages = append(r.messages, recordedMessage{senderToken: senderToken, body: body, flag: flag})

	return Message{
		ID:          int64(len(r.messages)),
		SenderToken: senderToken,
		Body:        body,
		CreatedAt:   time.Now().UTC(),
	}, nil
}

func (r *fakeRepository) End(_ context.Context, _, reason string, at time.Time) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.endedAt.IsZero() {
		return false, nil
	}

	r.endedAt = at
	r.endReason = reason

	return true, nil
}

func (r *fakeRepository) recorded() []recordedMessage {
	r.mu.Lock()
	defer r.mu.Unlock()

	return slices.Clone(r.messages)
}

func (r *fakeRepository) ending() (time.Time, string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.endedAt, r.endReason
}

// moderatorFunc is a Moderator written as a function.
type moderatorFunc func(ctx context.Context, body string) (Decision, error)

func (f moderatorFunc) Moderate(ctx context.Context, body string) (Decision, error) {
	return f(ctx, body)
}

// testOptions keeps the timings short enough for a test to wait for them.
func testOptions() Options {
	return Options{
		PresenceInterval: 10 * time.Millisecond,
		PresenceTTL:      2 * time.Second,
		// The wait for a participant to come back outlasts a test, so that
		// only the tests about the end of a conversation shorten it.
		RejoinGrace:  3 * time.Second,
		PingInterval: time.Second,
	}
}

// newTestServer starts one server on the given repository and store. Starting
// two of them on the same pair stands for two servers of one deployment.
func newTestServer(t *testing.T, repo Repository, store Store, moderator Moderator, opts Options, tokens ...string) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	NewHandler(NewService(repo, store, moderator, opts), &stubAuthenticator{tokens: tokens}, nil).Register(mux)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv
}

// dial connects one participant to a room.
func dial(t *testing.T, srv *httptest.Server, conversationID, token string) *websocket.Conn {
	t.Helper()

	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/rooms/" + conversationID
	header := http.Header{"Cookie": []string{"anontopic_session=" + token}}

	ws, res, err := websocket.DefaultDialer.Dial(url, header)
	if res != nil {
		defer res.Body.Close()
	}
	if err != nil {
		status := 0
		if res != nil {
			status = res.StatusCode
		}
		t.Fatalf("dial %s: %v (status %d)", url, err, status)
	}
	t.Cleanup(func() { _ = ws.Close() })

	return ws
}

// send writes one frame as a client would.
func send(t *testing.T, ws *websocket.Conn, frame clientFrame) {
	t.Helper()

	if err := ws.WriteJSON(frame); err != nil {
		t.Fatalf("write frame: %v", err)
	}
}

// readEvent reads the next event, together with the bytes it arrived as.
func readEvent(t *testing.T, ws *websocket.Conn) (serverEvent, []byte) {
	t.Helper()

	_ = ws.SetReadDeadline(time.Now().Add(3 * time.Second))

	_, payload, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("read event: %v", err)
	}

	var ev serverEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		t.Fatalf("decode event %q: %v", payload, err)
	}

	return ev, payload
}

// await reads until the event of the wanted type arrives.
func await(t *testing.T, ws *websocket.Conn, eventType string) (serverEvent, []byte) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		ev, payload := readEvent(t, ws)
		if ev.Type == eventType {
			return ev, payload
		}
	}

	t.Fatalf("no %s event arrived", eventType)
	return serverEvent{}, nil
}
