package matching

import (
	"context"
	"time"
)

// Store keeps the waiting queues and the rooms formed out of them.
//
// A user is in exactly one of three states: absent from the store, waiting in
// one queue, or holding a room. Implementations move a user between them in
// one atomic step, so that two servers forming a room at the same time cannot
// put the same user in both.
type Store interface {
	// Enqueue puts token at the end of the queue q names and keeps it there
	// for at most ttl. Enqueueing a token that already waits in q keeps the
	// place it holds; a token waiting elsewhere is reported as
	// ErrAlreadyWaiting, and one that already holds a room as ErrAlreadyMatched.
	Enqueue(ctx context.Context, token string, q Queue, now time.Time, ttl time.Duration) error

	// FormRoom takes the participants of one room out of q and marks them as
	// holding a room whose conversation does not exist yet. It drops users
	// whose wait passed ttl, and settles for two participants when q is a
	// queue of three whose oldest user has waited longer than fallbackAfter.
	// It returns no participants when q cannot form a room.
	FormRoom(ctx context.Context, q Queue, now time.Time, ttl, fallbackAfter time.Duration) ([]string, error)

	// Assign records conv as the conversation of every participant and keeps
	// it readable for ttl.
	Assign(ctx context.Context, participants []string, conv Conversation, ttl time.Duration) error

	// Discard drops a room FormRoom took out of the queue, which returns its
	// participants to the state they had before they queued.
	Discard(ctx context.Context, participants []string) error

	// Lookup returns what token is currently doing.
	Lookup(ctx context.Context, token string) (State, error)

	// Remove takes token out of the queue it waits in. Removing a token that
	// waits nowhere is not an error.
	Remove(ctx context.Context, token string) error
}
