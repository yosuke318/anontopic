package chat

import "time"

// frameMessage is the type of the only frame a client sends: one message for
// the room.
const frameMessage = "message"

// clientFrame is what a client sends over its connection.
type clientFrame struct {
	Type string `json:"type"`
	Body string `json:"body"`
}

// The types of the events the server sends over a connection.
const (
	// eventJoined greets the connection that was just admitted.
	eventJoined = "joined"
	// eventParticipantJoined and eventParticipantLeft tell a room who is in
	// it. A participant is told of their own arrival as well, because a room
	// is told everything at once.
	eventParticipantJoined = "participant_joined"
	eventParticipantLeft   = "participant_left"
	// eventMessage carries one message to the room, its sender included.
	eventMessage = "message"
	// eventEnded is the last event of a conversation.
	eventEnded = "ended"
	// eventError answers the sender of a frame that was not delivered, and
	// reaches nobody else.
	eventError = "error"
)

// The codes eventError carries.
const (
	codeInvalidFrame = "invalid_frame"
	codeEmptyBody    = "empty_body"
	codeTooLong      = "too_long"
	codeBlocked      = "blocked"
	codeRateLimited  = "rate_limited"
	codeUnavailable  = "unavailable"
)

// serverEvent is what the server sends over a connection. Which fields are
// set depends on Type.
type serverEvent struct {
	Type string `json:"type"`
	// Conversation is set on eventJoined.
	Conversation *conversationInfo `json:"conversation,omitempty"`
	// Participant is who the event is about: the reader of eventJoined, the
	// participant who arrived or left, or the sender of a message.
	Participant int `json:"participant,omitempty"`
	// Present holds the participants connected to the room. It is set on
	// eventJoined, eventParticipantJoined and eventParticipantLeft.
	Present []int `json:"present,omitempty"`
	// Body and SentAt are set on eventMessage.
	Body   string     `json:"body,omitempty"`
	SentAt *time.Time `json:"sent_at,omitempty"`
	// Reason is set on eventEnded and holds a value of
	// conversations.end_reason.
	Reason string `json:"reason,omitempty"`
	// Code and Message are set on eventError.
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// conversationInfo is the room a connection was admitted to.
type conversationInfo struct {
	ID        string    `json:"id"`
	TopicID   int       `json:"topic_id"`
	RoomType  int       `json:"room_type"`
	StartedAt time.Time `json:"started_at"`
}

// errorEvent is the answer to a frame that was not delivered.
func errorEvent(code, message string) serverEvent {
	return serverEvent{Type: eventError, Code: code, Message: message}
}
