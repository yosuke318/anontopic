package chat

import (
	"context"
	"time"
)

// Store carries the events of a room between the servers its participants are
// connected to, and holds which participants are connected.
//
// A server holds only the connections it accepted itself, so what one
// participant sends reaches the others through Publish, and a server learns
// that someone arrived or left from the tokens Join, Heartbeat and Leave
// return.
type Store interface {
	// Publish hands payload to every server subscribed to conversationID,
	// the publisher included.
	Publish(ctx context.Context, conversationID string, payload []byte) error

	// Subscribe delivers what is published for conversationID until the
	// returned Subscription is closed.
	Subscribe(ctx context.Context, conversationID string) (Subscription, error)

	// Join records token as connected to conversationID for ttl and returns
	// every token connected to it. A token that connects again keeps one
	// entry, so that a reconnection does not make the room look fuller.
	Join(ctx context.Context, conversationID, token string, now time.Time, ttl time.Duration) ([]string, error)

	// Heartbeat keeps token connected for another ttl and returns every token
	// connected to conversationID. A token that has not been kept within the
	// last ttl is dropped, an entry exactly that old included, which is what a
	// server that died leaves behind.
	Heartbeat(ctx context.Context, conversationID, token string, now time.Time, ttl time.Duration) ([]string, error)

	// Leave drops token from the participants connected to conversationID
	// and returns the tokens that still are.
	Leave(ctx context.Context, conversationID, token string, now time.Time, ttl time.Duration) ([]string, error)
}

// Subscription is the stream of one room's events as one server sees them.
type Subscription interface {
	// Events yields what is published for the room. The channel is closed
	// once the subscription is.
	Events() <-chan []byte

	// Close ends the subscription.
	Close() error
}
