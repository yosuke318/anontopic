package chat

import (
	"context"
	"time"
)

// Repository reads the conversation a participant connects to and stores what
// happens in it.
type Repository interface {
	// Conversation returns the conversation id names, together with the
	// session token of every participant it was formed for. It reports
	// ErrConversationNotFound when there is no such conversation.
	Conversation(ctx context.Context, id string) (Conversation, error)

	// AddMessages records messages of conversations, in the order they are
	// given. The slice is only the caller's for the length of the call, so an
	// implementation that keeps the messages has to copy them.
	AddMessages(ctx context.Context, messages []Message) error

	// End records that a conversation finished at the given time, and reports
	// whether this call is the one that ended it. A conversation that was
	// already over is left as it is.
	End(ctx context.Context, conversationID, reason string, at time.Time) (bool, error)
}
