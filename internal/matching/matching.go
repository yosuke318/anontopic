// Package matching owns the waiting queue and the rule that forms
// 2-3 person rooms out of users waiting on the same topic.
//
// A user joins the queue of one topic and one room type and waits there until
// enough users wait in the same queue. The queue lives in Redis so that every
// server sees the same waiting users, and a room is taken out of it in one
// atomic step, which is what keeps a user out of two rooms at once. The
// reasoning is in docs/adr/0008-hold-the-matching-queue-in-a-redis-sorted-set.md.
//
// A room of three that cannot fill up settles for two participants once the
// oldest of them has waited longer than FallbackAfter; the reasoning is in
// docs/adr/0009-fall-back-to-a-two-person-room.md.
//
// Boundary: it may hand room creation requests to other modules through
// interfaces, but never touches their persistence models directly.
package matching

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"time"
)

const (
	// DefaultWaitTTL is how long a user waits in a queue before the wait is
	// given up on. The waiting screen tells the user this limit and counts
	// down to it; the reasoning is in
	// docs/adr/0050-share-the-distance-covered-while-waiting-through-the-room.md.
	DefaultWaitTTL = 10 * time.Minute

	// DefaultFallbackAfter is how long two users wait for a third before they
	// are given a room of two.
	DefaultFallbackAfter = 60 * time.Second

	// DefaultRoomTTL is how long a formed room is kept for its participants to
	// read. It only has to outlast the interval at which a waiting client asks
	// for its state.
	DefaultRoomTTL = 10 * time.Minute

	// DefaultStatusTTL is how long one answer of QueueStatuses is reused.
	// Every client on the selection screen asks for it, so the queues are
	// read once per interval however many clients ask.
	DefaultStatusTTL = 5 * time.Second

	// roomTypeTwo and roomTypeThree are the room types conversations.room_type
	// holds. The value is the number of participants.
	roomTypeTwo   = 2
	roomTypeThree = 3
)

var (
	// ErrInvalidRoomType is returned for a room type no conversation can have.
	ErrInvalidRoomType = errors.New("matching: invalid room type")

	// ErrUnknownTopic is returned when the topic cannot be queued for, either
	// because it does not exist or because it is not active.
	ErrUnknownTopic = errors.New("matching: unknown topic")

	// ErrBanned is returned when the caller's identifier is on the ban list.
	ErrBanned = errors.New("matching: banned identifier")

	// ErrAlreadyWaiting is returned when the caller waits in another queue.
	ErrAlreadyWaiting = errors.New("matching: already waiting in another queue")

	// ErrAlreadyMatched is returned when the caller already has a conversation.
	ErrAlreadyMatched = errors.New("matching: already matched")

	// ErrDuplicateParticipant is returned when a room is formed with the same
	// session token twice, which would leave the room type of the conversation
	// higher than the number of participants recorded in it.
	ErrDuplicateParticipant = errors.New("matching: duplicate participant")

	// ErrTooManyRequests is returned when an address asks to be matched
	// faster than it is allowed to. The wait the limiter asked for is read
	// out of the error with errors.As on tooManyRequests.
	ErrTooManyRequests = errors.New("matching: asking to be matched too often")
)

// tooManyRequests is ErrTooManyRequests together with the wait before the
// caller may ask again, which the handler answers with.
type tooManyRequests struct {
	after time.Duration
}

func (e tooManyRequests) Error() string { return ErrTooManyRequests.Error() }

// Is makes errors.Is(err, ErrTooManyRequests) hold for this error, so that
// callers match the sentinel rather than the type carrying the wait.
func (e tooManyRequests) Is(target error) bool { return target == ErrTooManyRequests }

// Queue identifies one waiting queue: the topic its users picked and the type
// of room they wait for.
type Queue struct {
	TopicID  int
	RoomType int
}

// Conversation is the room a set of participants was assigned to.
type Conversation struct {
	ID        string
	TopicID   int
	RoomType  int
	StartedAt time.Time
}

// StateKind says whether a user waits, has a room, or is doing neither.
type StateKind int

const (
	// StateIdle is a user who is not in the matching module at all.
	StateIdle StateKind = iota
	// StateWaiting is a user whose room is not settled yet.
	StateWaiting
	// StateMatched is a user who has been assigned to a conversation.
	StateMatched
)

// State is what one user is currently doing in the matching module.
type State struct {
	Kind StateKind
	// Queue is the queue the user picked. It is set for StateWaiting and for
	// StateMatched, where the room type can be smaller than the one asked for.
	Queue Queue
	// WaitingSince is when the user entered the queue. It is set for
	// StateWaiting.
	WaitingSince time.Time
	// WaitingUntil is when the wait is given up on, WaitTTL after
	// WaitingSince. It is set for StateWaiting.
	WaitingUntil time.Time
	// Conversation is set for StateMatched.
	Conversation Conversation
}

// SessionAuthenticator resolves the session a request carries and returns the
// token identifying the participant, plus the hashed address and the device
// ID the request came with.
type SessionAuthenticator interface {
	Authenticate(r *http.Request) (string, error)
	IPHash(r *http.Request) string
	DeviceID(r *http.Request) string
}

// Identity is what one user is told apart by for the ban list. Either field
// is empty when it is not known.
type Identity struct {
	IPHash string
	Device string
}

// Attributes is the gender and the age band a user chose to give. Either
// field is empty when the user did not answer it. They are recorded with the
// participants of a room and never read to form one; the reasoning is in
// docs/adr/0044-record-the-gender-and-age-band-a-user-chose-to-give-without-matching-on-them.md.
type Attributes struct {
	Gender  string
	AgeBand string
}

// Location is where the address a user came from is registered. A field is
// empty when it is not known. It is recorded with the participants of a room
// and never read to form one; the reasoning is in
// docs/adr/0045-locate-the-prefecture-of-a-session-with-geolite-downloaded-by-the-server.md.
type Location struct {
	Country    string
	Prefecture string
	ASN        uint32
}

// Participant is one participant of a room being formed.
type Participant struct {
	Token      string
	Identity   Identity
	Attributes Attributes
	Location   Location
}

// Sessions reads what the session behind a token holds about its user, for
// the users who are in a room without a request of their own being served.
// The Token of the returned Participant is not used.
type Sessions interface {
	Participant(ctx context.Context, token string) (Participant, error)
}

// RequestLimiter reports whether an address may ask to be matched again now,
// and how long it has to wait when it may not.
type RequestLimiter interface {
	AllowMatch(ctx context.Context, subject string) (bool, time.Duration, error)
}

// TopicCatalogue reports which topics users can queue for.
type TopicCatalogue interface {
	IsActive(ctx context.Context, id int) (bool, error)
	// ActiveIDs returns the IDs of every topic users can queue for, in the
	// order the selection screen shows them.
	ActiveIDs(ctx context.Context) ([]int, error)
}

// QueueStatus says whether anyone waits in one queue. It carries no count:
// the selection screen only tells a user whether someone is there to be
// matched with, and the reasoning is in
// docs/adr/0048-show-users-whether-anyone-waits-in-a-queue-without-the-count.md.
type QueueStatus struct {
	Queue   Queue
	Waiting bool
}

// BanList reports whether any identifier of a user is barred from the
// service.
type BanList interface {
	IsBanned(ctx context.Context, id Identity) (bool, error)
}

// Service puts users in a queue and forms rooms out of it.
type Service struct {
	store    Store
	repo     Repository
	topics   TopicCatalogue
	bans     BanList
	sessions Sessions
	rate     RequestLimiter

	waitTTL       time.Duration
	fallbackAfter time.Duration
	roomTTL       time.Duration
	statusTTL     time.Duration
	now           func() time.Time

	// statusMu guards the answer of QueueStatuses kept for statusTTL. It is
	// held while the queues are read, so that the callers arriving after the
	// answer expired wait for one read instead of each making their own.
	statusMu        sync.Mutex
	statuses        []QueueStatus
	statusExpiresAt time.Time
}

// Options configures a Service. The zero value of each field selects the
// default described on the field.
type Options struct {
	// WaitTTL defaults to DefaultWaitTTL.
	WaitTTL time.Duration
	// FallbackAfter defaults to DefaultFallbackAfter. It is the wait after
	// which a queue of three settles for two participants.
	FallbackAfter time.Duration
	// RoomTTL defaults to DefaultRoomTTL.
	RoomTTL time.Duration
	// StatusTTL defaults to DefaultStatusTTL.
	StatusTTL time.Duration
}

// NewService builds a Service. A nil sessions records nothing but the token
// of each participant of a room, and a nil rate limiter takes requests to be
// matched however fast they arrive.
func NewService(store Store, repo Repository, topics TopicCatalogue, bans BanList, sessions Sessions, rate RequestLimiter, opts Options) *Service {
	if opts.WaitTTL <= 0 {
		opts.WaitTTL = DefaultWaitTTL
	}
	if opts.FallbackAfter <= 0 {
		opts.FallbackAfter = DefaultFallbackAfter
	}
	if opts.RoomTTL <= 0 {
		opts.RoomTTL = DefaultRoomTTL
	}
	if opts.StatusTTL <= 0 {
		opts.StatusTTL = DefaultStatusTTL
	}

	return &Service{
		store:         store,
		repo:          repo,
		topics:        topics,
		bans:          bans,
		sessions:      sessions,
		rate:          rate,
		waitTTL:       opts.WaitTTL,
		fallbackAfter: opts.FallbackAfter,
		roomTTL:       opts.RoomTTL,
		statusTTL:     opts.StatusTTL,
		now:           time.Now,
	}
}

// WaitingByTopic returns how many users wait for a room under each topic in
// topicIDs, whatever type of room they wait for. A user whose wait passed
// WaitTTL is left out.
func (s *Service) WaitingByTopic(ctx context.Context, topicIDs []int) (map[int]int, error) {
	queues := make([]Queue, 0, 2*len(topicIDs))
	for _, id := range topicIDs {
		queues = append(queues, Queue{TopicID: id, RoomType: roomTypeTwo}, Queue{TopicID: id, RoomType: roomTypeThree})
	}

	waiting, err := s.store.CountWaiting(ctx, queues, s.now().UTC(), s.waitTTL)
	if err != nil {
		return nil, err
	}

	counts := make(map[int]int, len(topicIDs))
	for _, id := range topicIDs {
		counts[id] = 0
	}
	for q, n := range waiting {
		counts[q.TopicID] += n
	}
	return counts, nil
}

// QueueStatuses reports, for every queue of every active topic, whether
// anyone waits in it. A user whose wait passed WaitTTL is not counted. The
// statuses come in the order of the topics, each topic with its room of two
// first, and one answer is reused for StatusTTL.
func (s *Service) QueueStatuses(ctx context.Context) ([]QueueStatus, error) {
	s.statusMu.Lock()
	defer s.statusMu.Unlock()

	now := s.now().UTC()
	// A zero expiry means nothing has been read yet, which a catalogue
	// without active topics must not be mistaken for.
	if !s.statusExpiresAt.IsZero() && now.Before(s.statusExpiresAt) {
		return slices.Clone(s.statuses), nil
	}

	ids, err := s.topics.ActiveIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active topics: %w", err)
	}

	queues := make([]Queue, 0, 2*len(ids))
	for _, id := range ids {
		queues = append(queues, Queue{TopicID: id, RoomType: roomTypeTwo}, Queue{TopicID: id, RoomType: roomTypeThree})
	}

	waiting, err := s.store.CountWaiting(ctx, queues, now, s.waitTTL)
	if err != nil {
		return nil, err
	}

	statuses := make([]QueueStatus, len(queues))
	for i, q := range queues {
		statuses[i] = QueueStatus{Queue: q, Waiting: waiting[q] > 0}
	}

	s.statuses = statuses
	s.statusExpiresAt = now.Add(s.statusTTL)
	return slices.Clone(statuses), nil
}

// Join puts the user behind token in the queue q names and tries to form a
// room out of it. The returned State says whether the user got one.
func (s *Service) Join(ctx context.Context, token string, id Identity, q Queue) (State, error) {
	if q.RoomType != roomTypeTwo && q.RoomType != roomTypeThree {
		return State{}, ErrInvalidRoomType
	}

	allowed, retryAfter, err := s.allowMatch(ctx, id.IPHash)
	if err != nil {
		return State{}, fmt.Errorf("read the matching rate: %w", err)
	}
	if !allowed {
		return State{}, tooManyRequests{after: retryAfter}
	}

	banned, err := s.bans.IsBanned(ctx, id)
	if err != nil {
		return State{}, fmt.Errorf("read ban list: %w", err)
	}
	if banned {
		return State{}, ErrBanned
	}

	active, err := s.topics.IsActive(ctx, q.TopicID)
	if err != nil {
		return State{}, fmt.Errorf("read topic: %w", err)
	}
	if !active {
		return State{}, ErrUnknownTopic
	}

	if err := s.store.Enqueue(ctx, token, q, s.now().UTC(), s.waitTTL); err != nil {
		return State{}, err
	}

	if err := s.form(ctx, q); err != nil {
		return State{}, err
	}

	return s.State(ctx, token)
}

// allowMatch asks the limiter whether the address is within its rate.
// Without one, every request is taken.
func (s *Service) allowMatch(ctx context.Context, ipHash string) (bool, time.Duration, error) {
	if s.rate == nil {
		return true, 0, nil
	}
	return s.rate.AllowMatch(ctx, ipHash)
}

// State reports what the user behind token is doing, forming a room first if
// the queue they wait in can produce one. Waiting clients call it until they
// are given a conversation.
func (s *Service) State(ctx context.Context, token string) (State, error) {
	state, err := s.store.Lookup(ctx, token)
	if err != nil {
		return State{}, fmt.Errorf("look up state: %w", err)
	}
	if state.Kind != StateWaiting {
		return state, nil
	}

	// Two users waiting for a third are given a room of two once their wait is
	// long enough, and nothing but their own request is there to notice.
	if err := s.form(ctx, state.Queue); err != nil {
		return State{}, err
	}

	state, err = s.store.Lookup(ctx, token)
	if err != nil {
		return State{}, fmt.Errorf("look up state: %w", err)
	}
	if state.Kind == StateWaiting {
		state.WaitingUntil = state.WaitingSince.Add(s.waitTTL)
	}
	return state, nil
}

// Leave gives up both the queue the user behind token waits in and the room
// they hold, so that they can queue again. It is not an error to leave when
// there is neither.
//
// Releasing the room here does not end the conversation: that happens in the
// chat module once the participants are gone for longer than the rejoin
// grace, and the reasoning is in
// docs/adr/0011-end-a-conversation-after-a-rejoin-grace.md.
func (s *Service) Leave(ctx context.Context, token string) error {
	if err := s.store.Remove(ctx, token, s.now().UTC()); err != nil {
		return fmt.Errorf("remove from queue: %w", err)
	}
	if err := s.store.Discard(ctx, []string{token}); err != nil {
		return fmt.Errorf("release room: %w", err)
	}
	return nil
}

// form takes one room out of q, records it in PostgreSQL and hands the
// conversation to its participants. It does nothing when q cannot form a room.
func (s *Service) form(ctx context.Context, q Queue) error {
	participants, err := s.store.FormRoom(ctx, q, s.now().UTC(), s.waitTTL, s.fallbackAfter)
	if err != nil {
		return fmt.Errorf("form room: %w", err)
	}
	if len(participants) == 0 {
		return nil
	}

	conv, err := s.repo.CreateConversation(ctx, q.TopicID, s.identify(ctx, participants))
	if err != nil {
		createErr := fmt.Errorf("create conversation: %w", err)

		// The participants are out of the queue and nothing is going to hand
		// them a conversation, so the room is dropped and they can queue again.
		if discardErr := s.store.Discard(ctx, participants); discardErr != nil {
			return errors.Join(createErr, fmt.Errorf("discard room: %w", discardErr))
		}
		return createErr
	}

	if err := s.store.Assign(ctx, participants, conv, s.roomTTL); err != nil {
		return fmt.Errorf("assign room: %w", err)
	}
	return nil
}

// identify reads what the session of every participant of a room holds. A
// participant whose session cannot be read joins the room all the same,
// without it: the room is already taken out of the queue, and none of it is
// needed to hold the conversation.
func (s *Service) identify(ctx context.Context, tokens []string) []Participant {
	participants := make([]Participant, len(tokens))
	for i, token := range tokens {
		participants[i].Token = token
		if s.sessions == nil {
			continue
		}

		p, err := s.sessions.Participant(ctx, token)
		if err != nil {
			slog.Warn("read the session of a participant", slog.Any("error", err))
			continue
		}
		p.Token = token
		participants[i] = p
	}
	return participants
}
