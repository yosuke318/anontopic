// Package report owns user-submitted reports of conversations, the claims of
// rights infringement anyone can file, and the review workflow attached to
// both.
//
// A report names a conversation, not a person: participants are anonymous to
// each other and are only told apart by a number that means nothing outside
// the room. What a reporter picks is the kind of behaviour they saw, taken
// from the prohibited uses in the requirements.
//
// A report ends the conversation and keeps it past the retention period. The
// conversation belongs to the chat module, so both are asked of it through
// Conversations; the reasoning is in
// docs/adr/0021-end-and-keep-a-conversation-once-it-is-reported.md.
//
// Boundary: reports reference rooms, messages and users by ID only.
package report

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
)

// The reasons a report can carry. They are a fixed set so that reports of the
// same kind can be counted together, and they follow the prohibited uses in
// the requirements.
const (
	// ReasonDating is use aimed at dating or meeting in person.
	ReasonDating = "dating"
	// ReasonSexual is sexual content.
	ReasonSexual = "sexual"
	// ReasonContact is exchanging contact details or leading elsewhere.
	ReasonContact = "contact"
	// ReasonHarassment is abuse directed at another participant.
	ReasonHarassment = "harassment"
	// ReasonSpam is advertising and other unsolicited postings.
	ReasonSpam = "spam"
	// ReasonOther is everything the reporter could not place.
	ReasonOther = "other"
)

var reasons = []string{
	ReasonDating,
	ReasonSexual,
	ReasonContact,
	ReasonHarassment,
	ReasonSpam,
	ReasonOther,
}

// The statuses the review of a report or a claim moves through.
const (
	// StatusOpen is a submission nobody has looked at yet.
	StatusOpen = "open"
	// StatusReviewing is a submission an operator is looking into.
	StatusReviewing = "reviewing"
	// StatusActioned is a submission that led to an action.
	StatusActioned = "actioned"
	// StatusRejected is a submission that called for no action.
	StatusRejected = "rejected"
)

var statuses = []string{
	StatusOpen,
	StatusReviewing,
	StatusActioned,
	StatusRejected,
}

const (
	// DefaultListLimit is how many submissions a list holds when the caller
	// does not say.
	DefaultListLimit = 50

	// MaxListLimit is the most submissions one list holds.
	MaxListLimit = 200
)

var (
	// ErrUnknownReason is returned for a reason outside the fixed set.
	ErrUnknownReason = errors.New("report: unknown reason")

	// ErrNotParticipant is returned when the reporter's session token is not
	// recorded as a participant of the conversation.
	ErrNotParticipant = errors.New("report: not a participant of the conversation")

	// ErrUnknownStatus is returned for a status outside the fixed set.
	ErrUnknownStatus = errors.New("report: unknown status")

	// ErrNotFound is returned for a report or a claim that does not exist.
	ErrNotFound = errors.New("report: not found")
)

// Report is one submission about one conversation.
type Report struct {
	ID             int64
	ConversationID string
	ReporterToken  string
	Reason         string
	Status         string
	CreatedAt      time.Time
}

// Filter narrows a list of reports or claims. Lists are ordered newest first,
// and an empty field narrows nothing.
type Filter struct {
	Status string
	// ConversationID keeps the reports of one conversation. Claims ignore it.
	ConversationID string
	// BeforeID keeps the submissions older than the one it names, which is how
	// the next page of a list is read.
	BeforeID int64
	// Limit is how many submissions the list holds at most. The service
	// always sets it.
	Limit int
}

// Repository stores the reports and the claims the module owns.
type Repository interface {
	// Add records r, and records nothing when its reporter has already
	// reported the conversation.
	Add(ctx context.Context, r Report) error

	// List returns the reports f keeps.
	List(ctx context.Context, f Filter) ([]Report, error)

	// Get returns the report id names, or ErrNotFound.
	Get(ctx context.Context, id int64) (Report, error)

	// SetStatus moves the report id names to status and returns it as it is
	// afterwards, or ErrNotFound.
	SetStatus(ctx context.Context, id int64, status string) (Report, error)

	// AddClaim records c and returns it with its ID and the time it was
	// recorded.
	AddClaim(ctx context.Context, c Claim) (Claim, error)

	// ListClaims returns the claims f keeps.
	ListClaims(ctx context.Context, f Filter) ([]Claim, error)

	// SetClaimStatus moves the claim id names to status and returns it as it
	// is afterwards, or ErrNotFound.
	SetClaimStatus(ctx context.Context, id int64, status string) (Claim, error)
}

// Transcript is a conversation as the module that owns it recorded it.
type Transcript struct {
	ConversationID string
	TopicID        int
	RoomType       int
	StartedAt      time.Time
	// EndedAt is the zero time while the conversation is in progress.
	EndedAt   time.Time
	EndReason string
	Flagged   bool
	// Participants holds the session token of every participant, in the
	// order the room numbers them from one.
	Participants []string
	Messages     []TranscriptMessage
}

// TranscriptMessage is one message of a Transcript.
type TranscriptMessage struct {
	SenderToken string
	Body        string
	// Flag is the value of messages.moderation_flag: 0 for a message nothing
	// was found in, 1 for one the filter judged, 2 for one recorded before its
	// conversation was reported, by someone other than the reporter.
	Flag      int
	CreatedAt time.Time
}

// Conversations is what the report module asks of the module that owns
// conversations.
type Conversations interface {
	// IsParticipant reports whether token belongs to a participant of the
	// conversation.
	IsParticipant(ctx context.Context, conversationID, token string) (bool, error)

	// Flag keeps the conversation past the retention period and marks the
	// messages of every participant other than the reporter.
	Flag(ctx context.Context, conversationID, reporterToken string) error

	// EndReported ends a conversation that is in progress and tells its room.
	EndReported(ctx context.Context, conversationID string) error

	// Transcript reads the conversation and every message recorded in it.
	Transcript(ctx context.Context, conversationID string) (Transcript, error)
}

// Service takes the reports participants file against a conversation and the
// claims anyone files, and carries the review of both.
type Service struct {
	repo          Repository
	conversations Conversations
}

// NewService builds a report service that records into repo and asks
// conversations about the conversations reports name.
func NewService(repo Repository, conversations Conversations) *Service {
	return &Service{repo: repo, conversations: conversations}
}

// Submit records a report of conversationID by the participant behind token,
// then keeps the conversation and ends it.
//
// Reporting the same conversation again records nothing further, so that the
// reports on a conversation count the participants who filed one rather than
// how often a button was pressed. Every step is safe to repeat, so a report
// that failed part of the way is completed by sending it again.
func (s *Service) Submit(ctx context.Context, conversationID, token, reason string) error {
	if !slices.Contains(reasons, reason) {
		return ErrUnknownReason
	}

	// Someone who was not in the room has nothing to report about it, and a
	// conversation that does not exist answers the same way, so that ids
	// cannot be probed by the answers they get.
	member, err := s.conversations.IsParticipant(ctx, conversationID, token)
	if err != nil {
		return fmt.Errorf("read conversation participants: %w", err)
	}
	if !member {
		return ErrNotParticipant
	}

	if err := s.repo.Add(ctx, Report{
		ConversationID: conversationID,
		ReporterToken:  token,
		Reason:         reason,
	}); err != nil {
		return fmt.Errorf("add report: %w", err)
	}

	if err := s.conversations.Flag(ctx, conversationID, token); err != nil {
		return fmt.Errorf("flag reported conversation: %w", err)
	}

	if err := s.conversations.EndReported(ctx, conversationID); err != nil {
		return fmt.Errorf("end reported conversation: %w", err)
	}

	return nil
}

// List returns the reports f keeps.
func (s *Service) List(ctx context.Context, f Filter) ([]Report, error) {
	f, err := normalizeFilter(f)
	if err != nil {
		return nil, err
	}
	return s.repo.List(ctx, f)
}

// Review is a report together with the conversation it names, as an operator
// reads it. Participants are named by their number in the room, so that no
// session token reaches the operator.
type Review struct {
	Report Report
	// Reporter is the number of the participant who filed the report.
	Reporter     int
	Conversation Transcript
	Messages     []ReviewMessage
}

// ReviewMessage is one message of a Review.
type ReviewMessage struct {
	// Participant is the number of the sender in the room.
	Participant int
	Body        string
	Flag        int
	CreatedAt   time.Time
}

// Review reads the report id names and the conversation it is about.
func (s *Service) Review(ctx context.Context, id int64) (Review, error) {
	rep, err := s.repo.Get(ctx, id)
	if err != nil {
		return Review{}, err
	}

	tr, err := s.conversations.Transcript(ctx, rep.ConversationID)
	if err != nil {
		return Review{}, fmt.Errorf("read transcript: %w", err)
	}

	review := Review{
		Report:       rep,
		Reporter:     participantNumber(tr.Participants, rep.ReporterToken),
		Conversation: tr,
		Messages:     make([]ReviewMessage, 0, len(tr.Messages)),
	}
	for _, msg := range tr.Messages {
		review.Messages = append(review.Messages, ReviewMessage{
			Participant: participantNumber(tr.Participants, msg.SenderToken),
			Body:        msg.Body,
			Flag:        msg.Flag,
			CreatedAt:   msg.CreatedAt,
		})
	}

	return review, nil
}

// UpdateStatus moves the report id names to status.
func (s *Service) UpdateStatus(ctx context.Context, id int64, status string) (Report, error) {
	if !slices.Contains(statuses, status) {
		return Report{}, ErrUnknownStatus
	}
	return s.repo.SetStatus(ctx, id, status)
}

// normalizeFilter refuses a status outside the fixed set and bounds the size
// of the list.
func normalizeFilter(f Filter) (Filter, error) {
	if f.Status != "" && !slices.Contains(statuses, f.Status) {
		return Filter{}, ErrUnknownStatus
	}
	if f.Limit <= 0 {
		f.Limit = DefaultListLimit
	}
	f.Limit = min(f.Limit, MaxListLimit)
	return f, nil
}

// participantNumber is the number the room names token by, counted from one,
// or zero for a token the conversation does not know.
func participantNumber(participants []string, token string) int {
	return slices.Index(participants, token) + 1
}
