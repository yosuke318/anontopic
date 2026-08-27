package matching

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// maxRequestBytes caps a join request, which holds a topic and a room type.
const maxRequestBytes = 1 << 10

// Handler exposes the matching module's HTTP surface.
type Handler struct {
	svc      *Service
	sessions SessionAuthenticator
}

// NewHandler builds a handler around svc.
func NewHandler(svc *Service, sessions SessionAuthenticator) *Handler {
	return &Handler{svc: svc, sessions: sessions}
}

// Register mounts the module's routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/matching", h.handleJoin)
	mux.HandleFunc("GET /api/matching", h.handleState)
	mux.HandleFunc("DELETE /api/matching", h.handleLeave)
}

// joinRequest is the queue the client asks to wait in. It holds the fields of
// Queue, so it converts into one.
type joinRequest struct {
	TopicID  int `json:"topic_id"`
	RoomType int `json:"room_type"`
}

// stateResponse tells a client whether it is still waiting and, once it is
// not, which conversation it joins.
type stateResponse struct {
	State        string     `json:"state"`
	TopicID      int        `json:"topic_id"`
	RoomType     int        `json:"room_type"`
	WaitingSince *time.Time `json:"waiting_since,omitempty"`
	Conversation *room      `json:"conversation,omitempty"`
}

// room is the conversation a matched client joins.
type room struct {
	ID        string    `json:"id"`
	StartedAt time.Time `json:"started_at"`
}

// The values of stateResponse.State.
const (
	stateWaiting = "waiting"
	stateMatched = "matched"
)

// handleJoin puts the caller in a queue and answers with the room they got,
// or with the wait they are in until one can be formed.
func (h *Handler) handleJoin(w http.ResponseWriter, r *http.Request) {
	token, ok := h.authenticate(w, r)
	if !ok {
		return
	}

	var req joinRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	state, err := h.svc.Join(r.Context(), token, h.sessions.IPHash(r), Queue(req))
	if err != nil {
		writeError(w, "join queue", err)
		return
	}

	status := http.StatusAccepted
	if state.Kind == StateMatched {
		status = http.StatusOK
	}
	writeJSON(w, status, newStateResponse(state))
}

// handleState answers with the state of the caller's wait. A client that is
// waiting asks for it until it is given a conversation.
func (h *Handler) handleState(w http.ResponseWriter, r *http.Request) {
	token, ok := h.authenticate(w, r)
	if !ok {
		return
	}

	state, err := h.svc.State(r.Context(), token)
	if err != nil {
		writeError(w, "read matching state", err)
		return
	}
	if state.Kind == StateIdle {
		http.Error(w, "not waiting", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, newStateResponse(state))
}

// handleLeave gives up the wait. Leaving without waiting is the same outcome
// as leaving a queue, so both answer 204.
func (h *Handler) handleLeave(w http.ResponseWriter, r *http.Request) {
	token, ok := h.authenticate(w, r)
	if !ok {
		return
	}

	if err := h.svc.Leave(r.Context(), token); err != nil {
		writeError(w, "leave queue", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// authenticate resolves the session the request carries.
func (h *Handler) authenticate(w http.ResponseWriter, r *http.Request) (string, bool) {
	token, err := h.sessions.Authenticate(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return "", false
	}
	return token, true
}

// newStateResponse is the answer for one state.
func newStateResponse(state State) stateResponse {
	body := stateResponse{
		State:    stateWaiting,
		TopicID:  state.Queue.TopicID,
		RoomType: state.Queue.RoomType,
	}

	if state.Kind == StateMatched {
		body.State = stateMatched
		body.Conversation = &room{
			ID:        state.Conversation.ID,
			StartedAt: state.Conversation.StartedAt,
		}
		return body
	}

	body.WaitingSince = &state.WaitingSince
	return body
}

// decodeJSON reads the request body into dst and answers 400 if it cannot.
// The body must hold exactly one JSON value: anything after it is a request
// the client did not mean to send the way this handler would read it.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil || dec.More() {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return false
	}
	return true
}

// writeError turns a service error into the answer the client gets. op names
// the failed operation in the log of an unexpected one.
func writeError(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, ErrInvalidRoomType):
		http.Error(w, "invalid room type", http.StatusBadRequest)
	case errors.Is(err, ErrUnknownTopic):
		http.Error(w, "unknown topic", http.StatusBadRequest)
	case errors.Is(err, ErrBanned):
		http.Error(w, "forbidden", http.StatusForbidden)
	case errors.Is(err, ErrAlreadyWaiting):
		http.Error(w, "already waiting in another queue", http.StatusConflict)
	case errors.Is(err, ErrAlreadyMatched):
		http.Error(w, "already matched", http.StatusConflict)
	default:
		slog.Error(op, slog.Any("error", err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
