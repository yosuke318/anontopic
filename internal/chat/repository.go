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

	// AddMessage records one message of a conversation and returns it with
	// the id and time the store gave it. flag is the moderation flag the
	// message was judged with.
	AddMessage(ctx context.Context, conversationID, senderToken, body string, flag int) (Message, error)

	// End records that a conversation finished at the given time, and reports
	// whether this call is the one that ended it. A conversation that was
	// already over is left as it is.
	End(ctx context.Context, conversationID, reason string, at time.Time) (bool, error)
}
