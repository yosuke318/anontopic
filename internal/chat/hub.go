package chat

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
)

// hub groups the connections one server holds by the conversation they belong
// to, and hands each room what is published for it.
type hub struct {
	store Store

	mu    sync.Mutex
	rooms map[string]*room
}

func newHub(store Store) *hub {
	return &hub{store: store, rooms: make(map[string]*room)}
}

// room is the set of connections one server holds for one conversation,
// together with the subscription that brings it what the other servers
// publish.
type room struct {
	conversationID string
	sub            Subscription

	mu    sync.RWMutex
	conns map[*conn]struct{}
}

// attach adds c to the room of conversationID, subscribing to it when this is
// the first connection this server holds for it.
func (h *hub) attach(ctx context.Context, conversationID string, c *conn) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	r, ok := h.rooms[conversationID]
	if !ok {
		// The room keeps its subscription for as long as this server holds a
		// connection to it, which outlasts the request that opened it.
		sub, err := h.store.Subscribe(context.WithoutCancel(ctx), conversationID)
		if err != nil {
			return err
		}

		r = &room{conversationID: conversationID, sub: sub, conns: make(map[*conn]struct{})}
		h.rooms[conversationID] = r
		go r.run()
	}

	r.mu.Lock()
	r.conns[c] = struct{}{}
	r.mu.Unlock()

	return nil
}

// detach removes c, and closes the subscription once this server holds no
// connection to the room.
func (h *hub) detach(conversationID string, c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()

	r, ok := h.rooms[conversationID]
	if !ok {
		return
	}

	r.mu.Lock()
	delete(r.conns, c)
	empty := len(r.conns) == 0
	r.mu.Unlock()

	if !empty {
		return
	}

	delete(h.rooms, conversationID)
	if err := r.sub.Close(); err != nil {
		slog.Error("close room subscription",
			slog.String("conversation_id", conversationID), slog.Any("error", err))
	}
}

// run hands every event published for the room to the connections this server
// holds, until the subscription is closed.
func (r *room) run() {
	for payload := range r.sub.Events() {
		var ev serverEvent
		if err := json.Unmarshal(payload, &ev); err != nil {
			slog.Error("decode room event",
				slog.String("conversation_id", r.conversationID), slog.Any("error", err))
			continue
		}

		r.deliver(outgoing{payload: payload, last: ev.Type == eventEnded})
	}
}

// deliver queues one event on every connection of the room.
func (r *room) deliver(msg outgoing) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for c := range r.conns {
		c.enqueue(msg)
	}
}
