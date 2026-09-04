package matching

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"
)

// testClock lets a test move past a wait without sleeping.
type testClock struct {
	current time.Time
}

func (c *testClock) Now() time.Time { return c.current }

func (c *testClock) Advance(d time.Duration) { c.current = c.current.Add(d) }

// entry is one token waiting in a queue.
type entry struct {
	token string
	since time.Time
}

// fakeStore keeps the queues in memory, following the same rules as the Redis
// store: one place per token, participants taken out in arrival order, and a
// queue of three that settles for two once its oldest token waited long enough.
type fakeStore struct {
	mu      sync.Mutex
	queues  map[Queue][]entry
	waiting map[string]entry
	rooms   map[string]State
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		queues:  make(map[Queue][]entry),
		waiting: make(map[string]entry),
		rooms:   make(map[string]State),
	}
}

func (s *fakeStore) Enqueue(_ context.Context, token string, q Queue, now time.Time, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.rooms[token]; ok {
		return ErrAlreadyMatched
	}
	if _, ok := s.waiting[token]; ok {
		if s.queueOf(token) != q {
			return ErrAlreadyWaiting
		}
		return nil
	}

	e := entry{token: token, since: now}
	s.waiting[token] = e
	s.queues[q] = append(s.queues[q], e)
	return nil
}

func (s *fakeStore) FormRoom(_ context.Context, q Queue, now time.Time, ttl, fallbackAfter time.Duration) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// A wait that ran out leaves the queue, and the entry that says where the
	// token waits goes with it, the way its key expires in Redis.
	queue := slices.DeleteFunc(s.queues[q], func(e entry) bool {
		if e.since.After(now.Add(-ttl)) {
			return false
		}
		delete(s.waiting, e.token)
		return true
	})
	s.queues[q] = queue

	take := 0
	switch {
	case len(queue) >= q.RoomType:
		take = q.RoomType
	case q.RoomType == roomTypeThree && len(queue) >= roomTypeTwo &&
		!queue[0].since.After(now.Add(-fallbackAfter)):
		take = roomTypeTwo
	default:
		return nil, nil
	}

	participants := make([]string, 0, take)
	for _, e := range queue[:take] {
		participants = append(participants, e.token)
		delete(s.waiting, e.token)
		s.rooms[e.token] = State{
			Kind:         StateWaiting,
			Queue:        Queue{TopicID: q.TopicID, RoomType: take},
			WaitingSince: e.since,
		}
	}
	s.queues[q] = queue[take:]

	return participants, nil
}

func (s *fakeStore) Assign(_ context.Context, participants []string, conv Conversation, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, token := range participants {
		s.rooms[token] = State{
			Kind:         StateMatched,
			Queue:        Queue{TopicID: conv.TopicID, RoomType: conv.RoomType},
			Conversation: conv,
		}
	}
	return nil
}

func (s *fakeStore) Discard(_ context.Context, participants []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, token := range participants {
		delete(s.rooms, token)
	}
	return nil
}

func (s *fakeStore) Lookup(_ context.Context, token string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if state, ok := s.rooms[token]; ok {
		return state, nil
	}
	if e, ok := s.waiting[token]; ok {
		return State{Kind: StateWaiting, Queue: s.queueOf(token), WaitingSince: e.since}, nil
	}
	return State{Kind: StateIdle}, nil
}

func (s *fakeStore) Remove(_ context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	q := s.queueOf(token)
	s.queues[q] = slices.DeleteFunc(s.queues[q], func(e entry) bool { return e.token == token })
	delete(s.waiting, token)
	return nil
}

// queueOf is the queue a waiting token sits in.
func (s *fakeStore) queueOf(token string) Queue {
	for q, queue := range s.queues {
		if slices.ContainsFunc(queue, func(e entry) bool { return e.token == token }) {
			return q
		}
	}
	return Queue{}
}

// fakeRepository records the conversations a test formed.
type fakeRepository struct {
	mu            sync.Mutex
	conversations []Conversation
	participants  map[string][]string
	err           error
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{participants: make(map[string][]string)}
}

func (r *fakeRepository) CreateConversation(_ context.Context, topicID int, participants []string) (Conversation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.err != nil {
		return Conversation{}, r.err
	}
	if token, ok := duplicate(participants); ok {
		return Conversation{}, fmt.Errorf("%w: %s", ErrDuplicateParticipant, token)
	}

	conv := Conversation{
		ID:        "conversation-" + strconv.Itoa(len(r.conversations)+1),
		TopicID:   topicID,
		RoomType:  len(participants),
		StartedAt: time.Unix(0, 0).UTC(),
	}
	r.conversations = append(r.conversations, conv)
	r.participants[conv.ID] = slices.Clone(participants)

	return conv, nil
}

// fakeTopics answers for a fixed set of active topics.
type fakeTopics struct {
	active []int
	err    error
}

func (t fakeTopics) IsActive(_ context.Context, id int) (bool, error) {
	if t.err != nil {
		return false, t.err
	}
	return slices.Contains(t.active, id), nil
}

// fakeRate holds back every request once allow is false, records the subjects
// it was asked about, and names retryAfter as the wait it wants.
type fakeRate struct {
	allow      bool
	retryAfter time.Duration
	subjects   []string
}

func (r *fakeRate) AllowMatch(_ context.Context, subject string) (bool, time.Duration, error) {
	r.subjects = append(r.subjects, subject)
	if r.allow {
		return true, 0, nil
	}
	return false, r.retryAfter, nil
}

// fakeBans answers for a fixed set of banned hashes.
type fakeBans struct {
	banned []string
	err    error
}

func (b fakeBans) IsBanned(_ context.Context, ipHash string) (bool, error) {
	if b.err != nil {
		return false, b.err
	}
	return slices.Contains(b.banned, ipHash), nil
}

// newTestService builds a Service whose clock the test drives.
func newTestService(t *testing.T, store Store, repo Repository) (*Service, *testClock) {
	t.Helper()

	svc := NewService(store, repo, fakeTopics{active: []int{1, 2}}, fakeBans{}, nil, Options{})
	clock := &testClock{current: time.Unix(1_700_000_000, 0).UTC()}
	svc.now = clock.Now

	return svc, clock
}

// join puts a token in a queue and fails the test when the request is refused.
func join(t *testing.T, svc *Service, token string, q Queue) State {
	t.Helper()

	state, err := svc.Join(context.Background(), token, "ip-hash-"+token, q)
	if err != nil {
		t.Fatalf("Join(%s): %v", token, err)
	}
	return state
}

func TestTheSecondUserOfATwoPersonQueueFormsTheRoom(t *testing.T) {
	svc, _ := newTestService(t, newFakeStore(), newFakeRepository())
	q := Queue{TopicID: 1, RoomType: 2}

	first := join(t, svc, "alice", q)
	if first.Kind != StateWaiting {
		t.Fatalf("first user kind = %v, want %v", first.Kind, StateWaiting)
	}

	second := join(t, svc, "bob", q)
	if second.Kind != StateMatched {
		t.Fatalf("second user kind = %v, want %v", second.Kind, StateMatched)
	}
	if second.Conversation.RoomType != 2 {
		t.Fatalf("room type = %d, want 2", second.Conversation.RoomType)
	}

	// The user who was already waiting reads the same conversation.
	waiting, err := svc.State(context.Background(), "alice")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if waiting.Kind != StateMatched || waiting.Conversation.ID != second.Conversation.ID {
		t.Fatalf("waiting user state = %+v, want conversation %s", waiting, second.Conversation.ID)
	}
}

func TestAThreePersonQueueWaitsForItsThirdUser(t *testing.T) {
	repo := newFakeRepository()
	svc, _ := newTestService(t, newFakeStore(), repo)
	q := Queue{TopicID: 1, RoomType: 3}

	join(t, svc, "alice", q)
	join(t, svc, "bob", q)
	if len(repo.conversations) != 0 {
		t.Fatalf("conversations = %d, want none before the third user", len(repo.conversations))
	}

	third := join(t, svc, "carol", q)
	if third.Kind != StateMatched || third.Conversation.RoomType != 3 {
		t.Fatalf("third user state = %+v, want a room of three", third)
	}
	if got := repo.participants[third.Conversation.ID]; len(got) != 3 {
		t.Fatalf("participants = %v, want three", got)
	}
}

func TestAThreePersonQueueSettlesForTwoAfterTheFallbackWait(t *testing.T) {
	svc, clock := newTestService(t, newFakeStore(), newFakeRepository())
	q := Queue{TopicID: 1, RoomType: 3}

	join(t, svc, "alice", q)
	join(t, svc, "bob", q)

	clock.Advance(DefaultFallbackAfter)

	state, err := svc.State(context.Background(), "alice")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if state.Kind != StateMatched {
		t.Fatalf("kind = %v, want %v", state.Kind, StateMatched)
	}
	if state.Conversation.RoomType != 2 {
		t.Fatalf("room type = %d, want 2", state.Conversation.RoomType)
	}
}

func TestAUserAloneInAQueueKeepsWaiting(t *testing.T) {
	svc, clock := newTestService(t, newFakeStore(), newFakeRepository())

	join(t, svc, "alice", Queue{TopicID: 1, RoomType: 3})
	clock.Advance(DefaultFallbackAfter)

	state, err := svc.State(context.Background(), "alice")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if state.Kind != StateWaiting {
		t.Fatalf("kind = %v, want %v", state.Kind, StateWaiting)
	}
}

func TestAWaitThatRanOutLeavesTheQueue(t *testing.T) {
	svc, clock := newTestService(t, newFakeStore(), newFakeRepository())
	q := Queue{TopicID: 1, RoomType: 2}

	join(t, svc, "alice", q)
	clock.Advance(DefaultWaitTTL)

	// The next user of the queue finds nobody to be matched with.
	second := join(t, svc, "bob", q)
	if second.Kind != StateWaiting {
		t.Fatalf("kind = %v, want %v", second.Kind, StateWaiting)
	}
}

func TestAWaitThatRanOutIsNoLongerWaiting(t *testing.T) {
	store := newFakeStore()
	svc, clock := newTestService(t, store, newFakeRepository())
	q := Queue{TopicID: 1, RoomType: 2}

	join(t, svc, "alice", q)
	clock.Advance(DefaultWaitTTL)
	join(t, svc, "bob", q)

	state, err := store.Lookup(context.Background(), "alice")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if state.Kind != StateIdle {
		t.Fatalf("kind = %v, want %v", state.Kind, StateIdle)
	}
}

func TestJoinRefusesARoomTypeNoConversationCanHave(t *testing.T) {
	svc, _ := newTestService(t, newFakeStore(), newFakeRepository())

	_, err := svc.Join(context.Background(), "alice", "ip-hash", Queue{TopicID: 1, RoomType: 4})
	if !errors.Is(err, ErrInvalidRoomType) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidRoomType)
	}
}

func TestJoinRefusesATopicThatIsNotOffered(t *testing.T) {
	svc, _ := newTestService(t, newFakeStore(), newFakeRepository())

	_, err := svc.Join(context.Background(), "alice", "ip-hash", Queue{TopicID: 99, RoomType: 2})
	if !errors.Is(err, ErrUnknownTopic) {
		t.Fatalf("err = %v, want %v", err, ErrUnknownTopic)
	}
}

func TestJoinKeepsABannedIdentifierOutOfTheQueue(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, newFakeRepository(), fakeTopics{active: []int{1}},
		fakeBans{banned: []string{"banned-hash"}}, nil, Options{})

	_, err := svc.Join(context.Background(), "alice", "banned-hash", Queue{TopicID: 1, RoomType: 2})
	if !errors.Is(err, ErrBanned) {
		t.Fatalf("err = %v, want %v", err, ErrBanned)
	}

	state, err := store.Lookup(context.Background(), "alice")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if state.Kind != StateIdle {
		t.Fatalf("kind = %v, want %v", state.Kind, StateIdle)
	}
}

func TestJoinRefusesAUserWaitingInAnotherQueue(t *testing.T) {
	svc, _ := newTestService(t, newFakeStore(), newFakeRepository())

	join(t, svc, "alice", Queue{TopicID: 1, RoomType: 2})

	_, err := svc.Join(context.Background(), "alice", "ip-hash", Queue{TopicID: 2, RoomType: 2})
	if !errors.Is(err, ErrAlreadyWaiting) {
		t.Fatalf("err = %v, want %v", err, ErrAlreadyWaiting)
	}
}

func TestJoiningTheSameQueueTwiceKeepsOnePlace(t *testing.T) {
	repo := newFakeRepository()
	svc, _ := newTestService(t, newFakeStore(), repo)
	q := Queue{TopicID: 1, RoomType: 2}

	join(t, svc, "alice", q)
	join(t, svc, "alice", q)

	if len(repo.conversations) != 0 {
		t.Fatalf("conversations = %d, want none: one user cannot fill a room of two", len(repo.conversations))
	}
}

func TestJoinRefusesAUserWhoAlreadyHasAConversation(t *testing.T) {
	svc, _ := newTestService(t, newFakeStore(), newFakeRepository())
	q := Queue{TopicID: 1, RoomType: 2}

	join(t, svc, "alice", q)
	join(t, svc, "bob", q)

	_, err := svc.Join(context.Background(), "alice", "ip-hash", q)
	if !errors.Is(err, ErrAlreadyMatched) {
		t.Fatalf("err = %v, want %v", err, ErrAlreadyMatched)
	}
}

func TestLeaveTakesTheUserOutOfTheQueue(t *testing.T) {
	svc, _ := newTestService(t, newFakeStore(), newFakeRepository())
	q := Queue{TopicID: 1, RoomType: 2}

	join(t, svc, "alice", q)
	if err := svc.Leave(context.Background(), "alice"); err != nil {
		t.Fatalf("Leave: %v", err)
	}

	state, err := svc.State(context.Background(), "alice")
	if err != nil {
		t.Fatalf("State: %v", err)
	}
	if state.Kind != StateIdle {
		t.Fatalf("kind = %v, want %v", state.Kind, StateIdle)
	}

	second := join(t, svc, "bob", q)
	if second.Kind != StateWaiting {
		t.Fatalf("kind = %v, want %v: the user who left must not fill the room", second.Kind, StateWaiting)
	}
}

func TestAConversationThatCannotBeWrittenReturnsItsParticipants(t *testing.T) {
	store := newFakeStore()
	repo := newFakeRepository()
	repo.err = errors.New("postgres is down")
	svc, _ := newTestService(t, store, repo)
	q := Queue{TopicID: 1, RoomType: 2}

	join(t, svc, "alice", q)

	if _, err := svc.Join(context.Background(), "bob", "ip-hash", q); err == nil {
		t.Fatal("Join: want the failure of the conversation to be reported")
	}

	for _, token := range []string{"alice", "bob"} {
		state, err := store.Lookup(context.Background(), token)
		if err != nil {
			t.Fatalf("Lookup(%s): %v", token, err)
		}
		if state.Kind != StateIdle {
			t.Fatalf("%s kind = %v, want %v", token, state.Kind, StateIdle)
		}
	}
}

func TestJoinHoldsBackAnAddressAskingTooOften(t *testing.T) {
	store := newFakeStore()
	rate := &fakeRate{allow: true}
	svc := NewService(store, newFakeRepository(), fakeTopics{active: []int{1}}, fakeBans{}, rate, Options{})

	if _, err := svc.Join(context.Background(), "alice", "one-network", Queue{TopicID: 1, RoomType: 2}); err != nil {
		t.Fatalf("Join: %v", err)
	}

	rate.allow = false
	_, err := svc.Join(context.Background(), "bob", "one-network", Queue{TopicID: 1, RoomType: 2})
	if !errors.Is(err, ErrTooManyRequests) {
		t.Fatalf("err = %v, want %v", err, ErrTooManyRequests)
	}

	// The request the rate held back put nobody in a queue.
	state, err := store.Lookup(context.Background(), "bob")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if state.Kind != StateIdle {
		t.Fatalf("state = %d, want %d", state.Kind, StateIdle)
	}

	// The rate is counted per address, so the hashed address is what it is
	// asked about.
	want := []string{"one-network", "one-network"}
	if len(rate.subjects) != len(want) || rate.subjects[0] != want[0] || rate.subjects[1] != want[1] {
		t.Fatalf("subjects = %v, want %v", rate.subjects, want)
	}
}
