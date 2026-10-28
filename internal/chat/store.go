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

	// Join records token as connected to the conversation of seat for ttl and
	// returns every token connected to it. A token that connects again keeps
	// one entry, so that a reconnection does not make the room look fuller.
	Join(ctx context.Context, seat Seat, now time.Time, ttl time.Duration) ([]string, error)

	// Heartbeat keeps token connected for another ttl and returns every token
	// connected to the conversation of seat. A token that has not been kept
	// within the last ttl is dropped, an entry exactly that old included,
	// which is what a server that died leaves behind.
	Heartbeat(ctx context.Context, seat Seat, now time.Time, ttl time.Duration) ([]string, error)

	// Leave drops the token of seat from the participants connected to its
	// conversation and returns the tokens that still are.
	Leave(ctx context.Context, seat Seat, now time.Time, ttl time.Duration) ([]string, error)

	// CountConnected returns how many participants are connected to the
	// conversations of each topic in topicIDs. What Join and Heartbeat record
	// is counted for ttl, the same lifetime a room counts it for. A topic
	// nobody is connected to answers zero.
	CountConnected(ctx context.Context, topicIDs []int, now time.Time, ttl time.Duration) (map[int]int, error)
}

// Seat is the place one participant's connection takes: the conversation and
// the topic it is held under, and the participant's session token.
type Seat struct {
	ConversationID string
	TopicID        int
	Token          string
}

// Subscription is the stream of one room's events as one server sees them.
type Subscription interface {
	// Events yields what is published for the room. The channel is closed
	// once the subscription is.
	Events() <-chan []byte

	// Close ends the subscription.
	Close() error
}
