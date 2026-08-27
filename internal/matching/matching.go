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
	"net/http"
	"time"
)

const (
	// DefaultWaitTTL is how long a user waits in a queue before the wait is
	// given up on.
	DefaultWaitTTL = 5 * time.Minute

	// DefaultFallbackAfter is how long two users wait for a third before they
	// are given a room of two.
	DefaultFallbackAfter = 60 * time.Second

	// DefaultRoomTTL is how long a formed room is kept for its participants to
	// read. It only has to outlast the interval at which a waiting client asks
	// for its state.
	DefaultRoomTTL = 10 * time.Minute

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
)

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
	// Conversation is set for StateMatched.
	Conversation Conversation
}

// SessionAuthenticator resolves the session a request carries and returns the
// token identifying the participant, plus the hashed address the session was
// issued to.
type SessionAuthenticator interface {
	Authenticate(r *http.Request) (string, error)
	IPHash(r *http.Request) string
}

// TopicCatalogue reports whether users can queue for a topic.
type TopicCatalogue interface {
	IsActive(ctx context.Context, id int) (bool, error)
}

// BanList reports whether an identifier is barred from the service.
type BanList interface {
	IsBanned(ctx context.Context, ipHash string) (bool, error)
}

// Service puts users in a queue and forms rooms out of it.
type Service struct {
	store  Store
	repo   Repository
	topics TopicCatalogue
	bans   BanList

	waitTTL       time.Duration
	fallbackAfter time.Duration
	roomTTL       time.Duration
	now           func() time.Time
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
}

// NewService builds a Service.
func NewService(store Store, repo Repository, topics TopicCatalogue, bans BanList, opts Options) *Service {
	if opts.WaitTTL <= 0 {
		opts.WaitTTL = DefaultWaitTTL
	}
	if opts.FallbackAfter <= 0 {
		opts.FallbackAfter = DefaultFallbackAfter
	}
	if opts.RoomTTL <= 0 {
		opts.RoomTTL = DefaultRoomTTL
	}

	return &Service{
		store:         store,
		repo:          repo,
		topics:        topics,
		bans:          bans,
		waitTTL:       opts.WaitTTL,
		fallbackAfter: opts.FallbackAfter,
		roomTTL:       opts.RoomTTL,
		now:           time.Now,
	}
}

// Join puts the user behind token in the queue q names and tries to form a
// room out of it. The returned State says whether the user got one.
func (s *Service) Join(ctx context.Context, token, ipHash string, q Queue) (State, error) {
	if q.RoomType != roomTypeTwo && q.RoomType != roomTypeThree {
		return State{}, ErrInvalidRoomType
	}

	banned, err := s.bans.IsBanned(ctx, ipHash)
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
	return state, nil
}

// Leave takes the user behind token out of the queue they wait in. It is not
// an error to leave a queue the user is not in.
func (s *Service) Leave(ctx context.Context, token string) error {
	if err := s.store.Remove(ctx, token); err != nil {
		return fmt.Errorf("remove from queue: %w", err)
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

	conv, err := s.repo.CreateConversation(ctx, q.TopicID, participants)
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
