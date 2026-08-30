// Package chat owns realtime room messaging: WebSocket connections,
// message fan-out inside a room, and room lifecycle events.
//
// A participant connects to the conversation they were matched into. Every
// server holds only the connections it accepted itself, so messages and room
// events reach the other participants over Redis Pub/Sub, and the
// participants currently connected to a room are counted in Redis as well.
//
// A conversation ends once fewer than two participants are connected for
// longer than the rejoin grace, which leaves a connection that drops room to
// come back; the reasoning is in
// docs/adr/0011-end-a-conversation-after-a-rejoin-grace.md.
//
// This module writes the messages of a conversation and its end. The rows of
// conversation_participants are written when the room is formed, and read
// here to tell a participant from a stranger; the reasoning is in
// docs/adr/0010-split-the-conversation-tables-by-lifecycle-phase.md.
//
// A message reaches its room before it is recorded: messages are buffered and
// written in batches, so that the database is not on the path a message takes
// to the room. The reasoning is in
// docs/adr/0012-buffer-message-writes-into-batched-inserts.md.
//
// Boundary: other modules must never import chat's persistence models.
// Cross-module communication goes through the exported interfaces below.
package chat

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

const (
	// DefaultPresenceInterval is how often a connection records that it is
	// still there and reads who else is connected.
	DefaultPresenceInterval = 5 * time.Second

	// DefaultPresenceTTL is how long a participant counts as connected
	// without recording it again. It outlasts several intervals, so that a
	// server under load does not drop the participants it is holding.
	DefaultPresenceTTL = 30 * time.Second

	// DefaultRejoinGrace is how long a conversation with fewer than two
	// participants connected waits for one to come back before it ends.
	DefaultRejoinGrace = 30 * time.Second

	// DefaultPingInterval is how often the server pings a connection to see
	// whether it is still answering.
	DefaultPingInterval = 30 * time.Second

	// DefaultWriteBatch is how many messages are recorded in one write.
	DefaultWriteBatch = 100

	// DefaultWriteInterval is how long a message waits for a batch to fill
	// before it is recorded with whatever else is buffered.
	DefaultWriteInterval = 200 * time.Millisecond

	// maxMessageRunes caps the body of one message. Counting runes keeps the
	// limit the same whatever script the message is written in.
	maxMessageRunes = 2000

	// maxFrameBytes caps a frame read from a client. A body of
	// maxMessageRunes fits even when every rune takes four bytes.
	maxFrameBytes = 16 << 10

	// pongGrace is how much longer than the ping interval a connection has to
	// answer before it is dropped.
	pongGrace = 30 * time.Second

	// writeWait bounds a single write to a connection.
	writeWait = 10 * time.Second

	// sendBuffer is how many events may wait for one connection before it is
	// closed for holding up its room.
	sendBuffer = 32

	// departureTimeout bounds the work done for a connection that is gone,
	// whose request is over and cannot bound it.
	departureTimeout = 5 * time.Second
)

// The values messages.moderation_flag takes for a message this module writes.
const (
	moderationFlagClean = 0
	moderationFlagNG    = 1
)

// endReasonUserLeft is the conversations.end_reason of a conversation that
// ran out of participants.
const endReasonUserLeft = "user_left"

var (
	// ErrConversationNotFound is returned for a conversation that does not
	// exist.
	ErrConversationNotFound = errors.New("chat: conversation not found")

	// ErrNotParticipant is returned when the caller's session token is not
	// recorded as a participant of the conversation.
	ErrNotParticipant = errors.New("chat: not a participant of the conversation")

	// ErrConversationEnded is returned for a conversation that is over.
	ErrConversationEnded = errors.New("chat: conversation has ended")
)

// Decision is what a Moderator says about the body of one message.
type Decision int

const (
	// DecisionAllow delivers the message and records it as unremarkable.
	DecisionAllow Decision = iota
	// DecisionFlag delivers the message and records the flag on it.
	DecisionFlag
	// DecisionBlock keeps the message from the room and tells its sender.
	DecisionBlock
)

// SessionAuthenticator resolves the session a request carries and returns the
// token identifying the participant.
type SessionAuthenticator interface {
	Authenticate(r *http.Request) (string, error)
}

// Moderator judges the body of a message before the room sees it.
type Moderator interface {
	Moderate(ctx context.Context, body string) (Decision, error)
}

// Conversation is the room a set of participants was assigned to.
type Conversation struct {
	ID        string
	TopicID   int
	RoomType  int
	StartedAt time.Time
	// EndedAt is the zero time while the conversation is in progress.
	EndedAt time.Time
	// Participants holds the session token of every participant, in the order
	// the conversation recorded them.
	Participants []string
}

// Message is one message of a conversation, as it is recorded.
type Message struct {
	ConversationID string
	SenderToken    string
	Body           string
	// Flag is the value of messages.moderation_flag the message was judged
	// with.
	Flag int
	// CreatedAt is when the server took the message. It is both what the room
	// was told and what the message is recorded with.
	CreatedAt time.Time
}

// Admission is the place a participant holds in the conversation they may
// connect to.
type Admission struct {
	Conversation Conversation
	Token        string
	// Participant identifies the participant inside their conversation. It is
	// the position of their token in Conversation.Participants counted from
	// one, so that every server names a participant the same way and no
	// session token reaches the other participants.
	Participant int
}

// Service admits participants to a conversation and carries what they send.
type Service struct {
	repo      Repository
	store     Store
	moderator Moderator
	hub       *hub
	writer    *messageWriter

	presenceInterval time.Duration
	presenceTTL      time.Duration
	rejoinGrace      time.Duration
	pingInterval     time.Duration
	now              func() time.Time
}

// Options configures a Service. The zero value of each field selects the
// default described on the field.
type Options struct {
	// PresenceInterval defaults to DefaultPresenceInterval.
	PresenceInterval time.Duration
	// PresenceTTL defaults to DefaultPresenceTTL.
	PresenceTTL time.Duration
	// RejoinGrace defaults to DefaultRejoinGrace.
	RejoinGrace time.Duration
	// PingInterval defaults to DefaultPingInterval.
	PingInterval time.Duration
	// WriteBatch defaults to DefaultWriteBatch.
	WriteBatch int
	// WriteInterval defaults to DefaultWriteInterval.
	WriteInterval time.Duration
}

// NewService builds a Service. A nil moderator delivers every message as it
// was written. The messages the service takes are recorded by a writer of its
// own, which Close stops.
func NewService(repo Repository, store Store, moderator Moderator, opts Options) *Service {
	if opts.PresenceInterval <= 0 {
		opts.PresenceInterval = DefaultPresenceInterval
	}
	if opts.PresenceTTL <= 0 {
		opts.PresenceTTL = DefaultPresenceTTL
	}
	if opts.RejoinGrace <= 0 {
		opts.RejoinGrace = DefaultRejoinGrace
	}
	if opts.PingInterval <= 0 {
		opts.PingInterval = DefaultPingInterval
	}
	if opts.WriteBatch <= 0 {
		opts.WriteBatch = DefaultWriteBatch
	}
	if opts.WriteInterval <= 0 {
		opts.WriteInterval = DefaultWriteInterval
	}

	return &Service{
		repo:             repo,
		store:            store,
		moderator:        moderator,
		hub:              newHub(store),
		writer:           newMessageWriter(repo, opts.WriteBatch, opts.WriteInterval),
		presenceInterval: opts.PresenceInterval,
		presenceTTL:      opts.PresenceTTL,
		rejoinGrace:      opts.RejoinGrace,
		pingInterval:     opts.PingInterval,
		now:              time.Now,
	}
}

// Close stops recording messages once the ones already taken are written. A
// service that was closed answers a message with an error instead of taking
// it, so that nothing is delivered that will not be recorded.
func (s *Service) Close(ctx context.Context) error {
	return s.writer.close(ctx)
}

// Admit reports the place token holds in the conversation, and refuses anyone
// the conversation was not formed for.
func (s *Service) Admit(ctx context.Context, conversationID, token string) (Admission, error) {
	conv, err := s.repo.Conversation(ctx, conversationID)
	if err != nil {
		return Admission{}, err
	}
	if !conv.EndedAt.IsZero() {
		return Admission{}, ErrConversationEnded
	}

	at := slices.Index(conv.Participants, token)
	if at < 0 {
		return Admission{}, ErrNotParticipant
	}

	return Admission{Conversation: conv, Token: token, Participant: at + 1}, nil
}

// Serve carries one connection until it closes: it announces the participant
// to the room, delivers what the room publishes, and records the departure.
func (s *Service) Serve(ctx context.Context, ws *websocket.Conn, adm Admission) {
	c := newConn(s, ws, adm)
	defer c.close()

	// Writing starts before anything else, so that a room this connection
	// cannot be served is answered rather than dropped in silence.
	go c.writeLoop()

	// The room is joined before the arrival is recorded, so that nothing
	// published in between is missed.
	if err := s.hub.attach(ctx, adm.Conversation.ID, c); err != nil {
		slog.Error("subscribe to room",
			slog.String("conversation_id", adm.Conversation.ID), slog.Any("error", err))
		c.send(serverEvent{Type: eventError, Code: codeUnavailable, Message: "the room is unavailable"})
		return
	}
	defer s.depart(ctx, c)

	connected, err := s.store.Join(ctx, adm.Conversation.ID, adm.Token, s.now().UTC(), s.presenceTTL)
	if err != nil {
		slog.Error("record arrival",
			slog.String("conversation_id", adm.Conversation.ID), slog.Any("error", err))
		c.send(serverEvent{Type: eventError, Code: codeUnavailable, Message: "the room is unavailable"})
		return
	}

	present := adm.Conversation.numbersOf(connected)
	c.send(serverEvent{
		Type: eventJoined,
		Conversation: &conversationInfo{
			ID:        adm.Conversation.ID,
			TopicID:   adm.Conversation.TopicID,
			RoomType:  adm.Conversation.RoomType,
			StartedAt: adm.Conversation.StartedAt,
		},
		Participant: adm.Participant,
		Present:     present,
	})
	s.publish(ctx, adm.Conversation.ID, serverEvent{
		Type:        eventParticipantJoined,
		Participant: adm.Participant,
		Present:     present,
	})

	go c.heartbeatLoop(ctx)
	c.readLoop(ctx)
}

// depart records that a connection is gone and tells the room. A conversation
// nobody is connected to ends here, because no connection is left to notice
// that it ran out of participants.
func (s *Service) depart(ctx context.Context, c *conn) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), departureTimeout)
	defer cancel()

	s.hub.detach(c.conversationID, c)

	connected, err := s.store.Leave(ctx, c.conversationID, c.token, s.now().UTC(), s.presenceTTL)
	if err != nil {
		slog.Error("record departure",
			slog.String("conversation_id", c.conversationID), slog.Any("error", err))
		return
	}

	s.publish(ctx, c.conversationID, serverEvent{
		Type:        eventParticipantLeft,
		Participant: c.participant,
		Present:     c.conversation.numbersOf(connected),
	})

	if len(connected) == 0 {
		s.end(ctx, c.conversationID, endReasonUserLeft)
	}
}

// end records that the conversation is over and tells the room. Every server
// that finds the conversation finished publishes the event, so that a
// connection is not left open by a publication another server lost.
func (s *Service) end(ctx context.Context, conversationID, reason string) {
	if _, err := s.repo.End(ctx, conversationID, reason, s.now().UTC()); err != nil {
		slog.Error("end conversation",
			slog.String("conversation_id", conversationID), slog.Any("error", err))
		return
	}

	s.publish(ctx, conversationID, serverEvent{Type: eventEnded, Reason: reason})
}

// handleFrame acts on one frame a client sent.
func (s *Service) handleFrame(ctx context.Context, c *conn, data []byte) {
	var frame clientFrame
	if err := json.Unmarshal(data, &frame); err != nil {
		c.send(errorEvent(codeInvalidFrame, "the frame is not a JSON object this room reads"))
		return
	}

	switch frame.Type {
	case frameMessage:
		s.sendMessage(ctx, c, frame.Body)
	default:
		c.send(errorEvent(codeInvalidFrame, "unknown frame type"))
	}
}

// sendMessage moderates one message, records it, and hands it to the room,
// its sender included.
func (s *Service) sendMessage(ctx context.Context, c *conn, body string) {
	body = strings.TrimSpace(body)
	switch {
	case body == "":
		c.send(errorEvent(codeEmptyBody, "the message is empty"))
		return
	case utf8.RuneCountInString(body) > maxMessageRunes:
		c.send(errorEvent(codeTooLong, "the message is too long"))
		return
	}

	decision, err := s.moderate(ctx, body)
	if err != nil {
		slog.Error("moderate message",
			slog.String("conversation_id", c.conversationID), slog.Any("error", err))
		c.send(errorEvent(codeUnavailable, "the message could not be checked"))
		return
	}
	if decision == DecisionBlock {
		c.send(errorEvent(codeBlocked, "the message was not delivered"))
		return
	}

	flag := moderationFlagClean
	if decision == DecisionFlag {
		flag = moderationFlagNG
	}

	// The message is taken for recording before the room reads it, so that a
	// message the server cannot keep is not delivered either.
	msg := Message{
		ConversationID: c.conversationID,
		SenderToken:    c.token,
		Body:           body,
		Flag:           flag,
		CreatedAt:      s.now().UTC(),
	}
	if err := s.writer.add(ctx, msg); err != nil {
		slog.Error("record message",
			slog.String("conversation_id", c.conversationID), slog.Any("error", err))
		c.send(errorEvent(codeUnavailable, "the message could not be recorded"))
		return
	}

	sentAt := msg.CreatedAt
	s.publish(ctx, c.conversationID, serverEvent{
		Type:        eventMessage,
		Participant: c.participant,
		Body:        msg.Body,
		SentAt:      &sentAt,
	})
}

// moderate asks the moderator about a body. Without one, every message is
// delivered as it was written.
func (s *Service) moderate(ctx context.Context, body string) (Decision, error) {
	if s.moderator == nil {
		return DecisionAllow, nil
	}
	return s.moderator.Moderate(ctx, body)
}

// publish hands one event to every server holding a connection to the room.
func (s *Service) publish(ctx context.Context, conversationID string, ev serverEvent) {
	payload, err := json.Marshal(ev)
	if err != nil {
		slog.Error("encode room event", slog.String("event", ev.Type), slog.Any("error", err))
		return
	}

	if err := s.store.Publish(ctx, conversationID, payload); err != nil {
		slog.Error("publish room event",
			slog.String("conversation_id", conversationID),
			slog.String("event", ev.Type),
			slog.Any("error", err))
	}
}

// numbersOf turns session tokens into the participant numbers the room speaks
// in, in ascending order. A token the conversation does not know is left out.
func (c Conversation) numbersOf(tokens []string) []int {
	numbers := make([]int, 0, len(tokens))
	for _, token := range tokens {
		if at := slices.Index(c.Participants, token); at >= 0 {
			numbers = append(numbers, at+1)
		}
	}

	slices.Sort(numbers)
	return numbers
}
