package topic

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// adminAuthScheme prefixes the admin token in the Authorization header.
	adminAuthScheme = "Bearer "

	// maxRequestBytes caps an administration request body, which holds a name
	// and a flag.
	maxRequestBytes = 4 << 10
)

// Handler exposes the topic module's HTTP surface.
type Handler struct {
	svc        *Service
	adminToken string
}

// NewHandler builds a handler around svc. adminToken is the secret the
// administration endpoints require; when it is empty they are not served at
// all, so a deployment without a secret has no write surface.
func NewHandler(svc *Service, adminToken string) *Handler {
	return &Handler{svc: svc, adminToken: adminToken}
}

// Register mounts the module's routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/topics", h.handleList)

	if h.adminToken == "" {
		return
	}

	mux.Handle("GET /api/admin/topics", h.requireAdmin(http.HandlerFunc(h.handleAdminList)))
	mux.Handle("POST /api/admin/topics", h.requireAdmin(http.HandlerFunc(h.handleCreate)))
	mux.Handle("PATCH /api/admin/topics/{id}", h.requireAdmin(http.HandlerFunc(h.handleUpdate)))
	mux.Handle("DELETE /api/admin/topics/{id}", h.requireAdmin(http.HandlerFunc(h.handleDelete)))
}

// topicResponse is what the selection screen needs to draw one choice.
type topicResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// listResponse holds the topics a user can pick.
type listResponse struct {
	Topics []topicResponse `json:"topics"`
}

// adminTopicResponse also carries the state the administration works with. It
// holds the fields of Topic, so a Topic converts into it.
type adminTopicResponse struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

// adminListResponse holds every topic, whether active or not.
type adminListResponse struct {
	Topics []adminTopicResponse `json:"topics"`
}

// createRequest is the body of a topic creation.
type createRequest struct {
	Name string `json:"name"`
}

// updateRequest is the body of a topic change. An omitted field is left as it
// is, which is how a topic is deactivated without touching its name. It holds
// the fields of Update, so it converts into one.
type updateRequest struct {
	Name     *string `json:"name"`
	IsActive *bool   `json:"is_active"`
}

// handleList answers the selection screen with the active topics.
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	topics, err := h.svc.ListActive(r.Context())
	if err != nil {
		slog.Error("list active topics", slog.Any("error", err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	body := listResponse{Topics: make([]topicResponse, 0, len(topics))}
	for _, t := range topics {
		body.Topics = append(body.Topics, topicResponse{ID: t.ID, Name: t.Name})
	}

	writeJSON(w, http.StatusOK, body)
}

// handleAdminList answers with every topic.
func (h *Handler) handleAdminList(w http.ResponseWriter, r *http.Request) {
	topics, err := h.svc.List(r.Context())
	if err != nil {
		slog.Error("list topics", slog.Any("error", err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	body := adminListResponse{Topics: make([]adminTopicResponse, 0, len(topics))}
	for _, t := range topics {
		body.Topics = append(body.Topics, adminTopicResponse(t))
	}

	writeJSON(w, http.StatusOK, body)
}

// handleCreate adds a topic to the catalogue.
func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	created, err := h.svc.Create(r.Context(), req.Name)
	if err != nil {
		writeError(w, "create topic", err)
		return
	}

	writeJSON(w, http.StatusCreated, adminTopicResponse(created))
}

// handleUpdate renames a topic, activates it or takes it off the selection
// screen.
func (h *Handler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := topicID(w, r)
	if !ok {
		return
	}

	var req updateRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	updated, err := h.svc.Update(r.Context(), id, Update(req))
	if err != nil {
		writeError(w, "update topic", err)
		return
	}

	writeJSON(w, http.StatusOK, adminTopicResponse(updated))
}

// handleDelete removes a topic no conversation references.
func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	id, ok := topicID(w, r)
	if !ok {
		return
	}

	if err := h.svc.Delete(r.Context(), id); err != nil {
		writeError(w, "delete topic", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// requireAdmin rejects requests that do not carry the admin token.
func (h *Handler) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		presented, ok := strings.CutPrefix(r.Header.Get("Authorization"), adminAuthScheme)
		// The comparison takes the same time whatever the token is, so that
		// the answer does not tell a caller how much of it was right.
		if !ok || subtle.ConstantTimeCompare([]byte(presented), []byte(h.adminToken)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// topicID reads the ID from the request path.
func topicID(w http.ResponseWriter, r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id <= 0 {
		http.Error(w, "invalid topic id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

// decodeJSON reads the request body into dst and answers 400 if it cannot.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return false
	}
	return true
}

// writeError turns a service error into the answer the client gets. op names
// the failed operation in the log of an unexpected one.
func writeError(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, ErrInvalidName):
		http.Error(w, "invalid topic name", http.StatusBadRequest)
	case errors.Is(err, ErrNotFound):
		http.Error(w, "topic not found", http.StatusNotFound)
	case errors.Is(err, ErrInUse):
		http.Error(w, "topic is referenced by conversations", http.StatusConflict)
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
