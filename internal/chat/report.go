package chat

import (
	"context"
	"fmt"
)

// Transcript is a conversation as it was recorded, for the operators who
// review a report of it.
type Transcript struct {
	Conversation Conversation
	// EndReason is the conversations.end_reason of a conversation that is
	// over, and empty while it is in progress.
	EndReason string
	// Flagged reports whether the conversation was reported, which keeps it
	// past the retention period.
	Flagged bool
	// Messages holds every recorded message, oldest first, including those the
	// filter kept from the room.
	Messages []Message
}

// Flag marks the conversation as reported by the participant behind
// reporterToken. The reasoning for which messages it marks is in
// docs/adr/0021-end-and-keep-a-conversation-once-it-is-reported.md.
//
// A message still waiting in the writer's buffer is recorded without the mark,
// and is kept all the same, because the conversation it belongs to is.
func (s *Service) Flag(ctx context.Context, conversationID, reporterToken string) error {
	if err := s.repo.Flag(ctx, conversationID, reporterToken); err != nil {
		return fmt.Errorf("flag conversation: %w", err)
	}
	return nil
}

// EndReported ends a conversation because one of its participants reported
// it, and tells the room. A conversation that was already over is left as it
// ended, and its room is told nothing more.
func (s *Service) EndReported(ctx context.Context, conversationID string) error {
	ended, err := s.repo.End(ctx, conversationID, endReasonReported, s.now().UTC())
	if err != nil {
		return fmt.Errorf("end conversation: %w", err)
	}
	if ended {
		s.publish(ctx, conversationID, serverEvent{Type: eventEnded, Reason: endReasonReported})
	}
	return nil
}

// Transcript reads the conversation id names with everything recorded in it.
func (s *Service) Transcript(ctx context.Context, id string) (Transcript, error) {
	return s.repo.Transcript(ctx, id)
}
