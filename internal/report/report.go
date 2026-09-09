// Package report owns user-submitted reports of conversations and the
// review workflow attached to them.
//
// A report names a conversation, not a person: participants are anonymous to
// each other and are only told apart by a number that means nothing outside
// the room. What a reporter picks is the kind of behaviour they saw, taken
// from the prohibited uses in the requirements.
//
// Boundary: reports reference rooms, messages and users by ID only.
package report

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
)

// The reasons a report can carry. They are a fixed set so that reports of the
// same kind can be counted together, and they follow the prohibited uses in
// the requirements.
const (
	// ReasonSexual is sexual content, or use aimed at dating or meeting.
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
	ReasonSexual,
	ReasonContact,
	ReasonHarassment,
	ReasonSpam,
	ReasonOther,
}

var (
	// ErrUnknownReason is returned for a reason outside the fixed set.
	ErrUnknownReason = errors.New("report: unknown reason")

	// ErrNotParticipant is returned when the reporter's session token is not
	// recorded as a participant of the conversation.
	ErrNotParticipant = errors.New("report: not a participant of the conversation")
)

// Report is one submission about one conversation.
type Report struct {
	ConversationID string
	ReporterToken  string
	Reason         string
}

// Repository stores the reports the module owns.
type Repository interface {
	// Add records r, and records nothing when its reporter has already
	// reported the conversation.
	Add(ctx context.Context, r Report) error
}

// Participation reports whether a session token belongs to a participant of a
// conversation.
type Participation interface {
	IsParticipant(ctx context.Context, conversationID, token string) (bool, error)
}

// SessionAuthenticator resolves the session a request carries and returns the
// token identifying the reporter.
type SessionAuthenticator interface {
	Authenticate(r *http.Request) (string, error)
}

// Service takes the reports participants file against a conversation.
type Service struct {
	repo  Repository
	rooms Participation
}

// NewService builds a report service that records into repo and asks rooms
// whether a reporter was in the conversation they name.
func NewService(repo Repository, rooms Participation) *Service {
	return &Service{repo: repo, rooms: rooms}
}

// Submit records a report of conversationID by the participant behind token.
//
// Reporting the same conversation again records nothing further, so that the
// reports on a conversation count the participants who filed one rather than
// how often a button was pressed.
func (s *Service) Submit(ctx context.Context, conversationID, token, reason string) error {
	if !slices.Contains(reasons, reason) {
		return ErrUnknownReason
	}

	// Someone who was not in the room has nothing to report about it, and a
	// conversation that does not exist answers the same way, so that ids
	// cannot be probed by the answers they get.
	member, err := s.rooms.IsParticipant(ctx, conversationID, token)
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

	return nil
}
