package matching

import "context"

// Repository stores the conversations formed out of a queue.
type Repository interface {
	// CreateConversation opens a conversation on topicID and records every
	// participant in it. The room type of the conversation is the number of
	// participants, so a token that appears twice is reported as
	// ErrDuplicateParticipant instead of being written.
	CreateConversation(ctx context.Context, topicID int, participants []string) (Conversation, error)
}
