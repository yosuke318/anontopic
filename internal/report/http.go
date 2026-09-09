package report

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
)

// maxRequestBytes caps a report, which holds a conversation id and a reason.
const maxRequestBytes = 1 << 10

// Handler exposes the report module's HTTP surface.
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
	mux.HandleFunc("POST /api/reports", h.handleSubmit)
}

// submitRequest is the conversation the caller reports and why.
type submitRequest struct {
	ConversationID string `json:"conversation_id"`
	Reason         string `json:"reason"`
}

// handleSubmit takes one report from a participant of the conversation it
// names.
func (h *Handler) handleSubmit(w http.ResponseWriter, r *http.Request) {
	token, err := h.sessions.Authenticate(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req submitRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	// A missing conversation id would otherwise read as reporting a stranger's
	// conversation, and answer 403 instead of naming the client's mistake.
	if req.ConversationID == "" || req.Reason == "" {
		http.Error(w, "conversation_id and reason are required", http.StatusBadRequest)
		return
	}

	switch err := h.svc.Submit(r.Context(), req.ConversationID, token, req.Reason); {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrUnknownReason):
		http.Error(w, "unknown reason", http.StatusBadRequest)
	case errors.Is(err, ErrNotParticipant):
		http.Error(w, "forbidden", http.StatusForbidden)
	default:
		slog.Error("submit report", slog.Any("error", err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// decodeJSON reads the request body into dst and answers 400 if it cannot.
// The body must hold exactly one JSON value: anything after it is a request
// the client did not mean to send the way this handler would read it, so the
// body has to be at its end once the value is read.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return false
	}

	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return false
	}
	return true
}
