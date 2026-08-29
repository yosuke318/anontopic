package chat

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// conn is one participant's WebSocket connection. Reading, writing and the
// heartbeat each run in their own goroutine; every write to the socket goes
// through writeLoop, because a connection takes one writer at a time.
type conn struct {
	svc            *Service
	ws             *websocket.Conn
	conversation   Conversation
	conversationID string
	token          string
	participant    int

	out       chan outgoing
	done      chan struct{}
	closeOnce sync.Once
}

// outgoing is one event on its way to a client.
type outgoing struct {
	payload []byte
	// last marks the event after which nothing else can be said, which is
	// what ends a conversation for the connections holding it.
	last bool
}

func newConn(svc *Service, ws *websocket.Conn, adm Admission) *conn {
	return &conn{
		svc:            svc,
		ws:             ws,
		conversation:   adm.Conversation,
		conversationID: adm.Conversation.ID,
		token:          adm.Token,
		participant:    adm.Participant,
		out:            make(chan outgoing, sendBuffer),
		done:           make(chan struct{}),
	}
}

// close ends the connection. Closing the socket is what stops readLoop, which
// blocks until the client or the server hangs up.
func (c *conn) close() {
	c.closeOnce.Do(func() {
		close(c.done)
		_ = c.ws.Close()
	})
}

// send queues one event for this connection alone.
func (c *conn) send(ev serverEvent) {
	payload, err := json.Marshal(ev)
	if err != nil {
		slog.Error("encode event", slog.String("event", ev.Type), slog.Any("error", err))
		return
	}

	c.enqueue(outgoing{payload: payload, last: ev.Type == eventEnded})
}

// enqueue hands one event to writeLoop. A connection that is too far behind
// to take it is closed, because a room cannot wait for one participant
// without holding up the others.
func (c *conn) enqueue(msg outgoing) {
	select {
	case c.out <- msg:
	case <-c.done:
	default:
		slog.Warn("closing a connection that is not keeping up",
			slog.String("conversation_id", c.conversationID))
		c.close()
	}
}

// readLoop reads the frames a client sends until the connection ends.
func (c *conn) readLoop(ctx context.Context) {
	readWait := c.svc.pingInterval + pongGrace

	c.ws.SetReadLimit(maxFrameBytes)
	_ = c.ws.SetReadDeadline(time.Now().Add(readWait))
	c.ws.SetPongHandler(func(string) error {
		return c.ws.SetReadDeadline(time.Now().Add(readWait))
	})

	for {
		messageType, data, err := c.ws.ReadMessage()
		if err != nil {
			return
		}
		_ = c.ws.SetReadDeadline(time.Now().Add(readWait))

		if messageType != websocket.TextMessage {
			continue
		}
		c.svc.handleFrame(ctx, c, data)
	}
}

// writeLoop writes what the room has for this connection, and pings a client
// that has been quiet to find out whether it is still there.
func (c *conn) writeLoop() {
	ping := time.NewTicker(c.svc.pingInterval)
	defer ping.Stop()

	for {
		select {
		case msg := <-c.out:
			if err := c.write(websocket.TextMessage, msg.payload); err != nil {
				c.close()
				return
			}
			if msg.last {
				_ = c.write(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				c.close()
				return
			}
		case <-ping.C:
			if err := c.write(websocket.PingMessage, nil); err != nil {
				c.close()
				return
			}
		case <-c.done:
			return
		}
	}
}

// heartbeatLoop keeps this participant counted as connected, and ends the
// conversation once it has had nobody to talk to for longer than the rejoin
// grace.
func (c *conn) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(c.svc.presenceInterval)
	defer ticker.Stop()

	var aloneSince time.Time
	for {
		select {
		case <-ticker.C:
			now := c.svc.now().UTC()

			connected, err := c.svc.store.Heartbeat(ctx, c.conversationID, c.token, now, c.svc.presenceTTL)
			if err != nil {
				slog.Error("keep the participant connected",
					slog.String("conversation_id", c.conversationID), slog.Any("error", err))
				continue
			}

			if len(connected) > 1 {
				aloneSince = time.Time{}
				continue
			}
			if aloneSince.IsZero() {
				aloneSince = now
				continue
			}
			if now.Sub(aloneSince) < c.svc.rejoinGrace {
				continue
			}

			c.svc.end(ctx, c.conversationID, endReasonUserLeft)
			return
		case <-c.done:
			return
		}
	}
}

// write sends one frame, giving up on a client that cannot take it.
func (c *conn) write(messageType int, payload []byte) error {
	if err := c.ws.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		return err
	}
	return c.ws.WriteMessage(messageType, payload)
}
