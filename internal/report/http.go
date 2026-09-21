package report

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/yosuke318/anontopic/internal/adminauth"
)

const (
	// maxReportBytes caps a report, which holds a conversation id and a
	// reason.
	maxReportBytes = 1 << 10

	// maxClaimBytes caps a claim. Its details fit even when every rune takes
	// four bytes.
	maxClaimBytes = 20 << 10

	// maxStatusBytes caps a status change.
	maxStatusBytes = 1 << 8
)

// SessionAuthenticator resolves the session a request carries and returns the
// token identifying the reporter, plus the hashed address a request came from.
type SessionAuthenticator interface {
	Authenticate(r *http.Request) (string, error)
	IPHash(r *http.Request) string
}

// ClaimLimiter reports whether a hashed address may file another claim now,
// and how long it has to wait when it may not.
type ClaimLimiter interface {
	AllowClaim(ctx context.Context, subject string) (bool, time.Duration, error)
}

// Handler exposes the report module's HTTP surface.
type Handler struct {
	svc      *Service
	sessions SessionAuthenticator
	claims   ClaimLimiter
	admin    adminauth.Guard
}

// NewHandler builds a handler around svc. adminToken is the secret the review
// endpoints require; when it is empty they are not served at all.
func NewHandler(svc *Service, sessions SessionAuthenticator, claims ClaimLimiter, adminToken string) *Handler {
	return &Handler{svc: svc, sessions: sessions, claims: claims, admin: adminauth.New(adminToken)}
}

// Register mounts the module's routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/reports", h.handleSubmit)
	mux.HandleFunc("POST /api/claims", h.handleSubmitClaim)

	if !h.admin.Enabled() {
		return
	}

	mux.Handle("GET /api/admin/reports", h.admin.Require(http.HandlerFunc(h.handleList)))
	mux.Handle("GET /api/admin/reports/{id}", h.admin.Require(http.HandlerFunc(h.handleReview)))
	mux.Handle("PATCH /api/admin/reports/{id}", h.admin.Require(http.HandlerFunc(h.handleUpdate)))
	mux.Handle("GET /api/admin/claims", h.admin.Require(http.HandlerFunc(h.handleListClaims)))
	mux.Handle("PATCH /api/admin/claims/{id}", h.admin.Require(http.HandlerFunc(h.handleUpdateClaim)))
}

// submitRequest is the conversation the caller reports and why.
type submitRequest struct {
	ConversationID string `json:"conversation_id"`
	Reason         string `json:"reason"`
}

// claimRequest is a claim as the form sends it.
type claimRequest struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Right   string `json:"right"`
	Details string `json:"details"`
}

// statusRequest is the status an operator moves a submission to.
type statusRequest struct {
	Status string `json:"status"`
}

// reportResponse is one report as an operator sees it. The reporter's session
// token is left out.
type reportResponse struct {
	ID             int64     `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Reason         string    `json:"reason"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
}

type reportListResponse struct {
	Reports []reportResponse `json:"reports"`
}

// reviewResponse is a report with the conversation it names.
type reviewResponse struct {
	Report       reportResponse       `json:"report"`
	Reporter     int                  `json:"reporter"`
	Conversation conversationResponse `json:"conversation"`
	Messages     []messageResponse    `json:"messages"`
}

type conversationResponse struct {
	ID           string     `json:"id"`
	TopicID      int        `json:"topic_id"`
	RoomType     int        `json:"room_type"`
	Participants int        `json:"participants"`
	StartedAt    time.Time  `json:"started_at"`
	EndedAt      *time.Time `json:"ended_at"`
	EndReason    *string    `json:"end_reason"`
	IsFlagged    bool       `json:"is_flagged"`
}

type messageResponse struct {
	Participant    int       `json:"participant"`
	Body           string    `json:"body"`
	ModerationFlag int       `json:"moderation_flag"`
	CreatedAt      time.Time `json:"created_at"`
}

// claimResponse is one claim as an operator sees it. It holds the fields of
// Claim, so a Claim converts into it.
type claimResponse struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Right     string    `json:"right"`
	Details   string    `json:"details"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type claimListResponse struct {
	Claims []claimResponse `json:"claims"`
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
	if !decodeJSON(w, r, &req, maxReportBytes) {
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

// handleSubmitClaim takes one claim from anyone, whether or not they hold a
// session, within the rate one address may file them at.
func (h *Handler) handleSubmitClaim(w http.ResponseWriter, r *http.Request) {
	allowed, wait, err := h.claims.AllowClaim(r.Context(), h.sessions.IPHash(r))
	if err != nil {
		slog.Error("read the claim rate", slog.Any("error", err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Round(time.Second).Seconds())))
		http.Error(w, "too many claims", http.StatusTooManyRequests)
		return
	}

	var req claimRequest
	if !decodeJSON(w, r, &req, maxClaimBytes) {
		return
	}

	_, err = h.svc.SubmitClaim(r.Context(), Claim{
		Name:    req.Name,
		Email:   req.Email,
		Right:   req.Right,
		Details: req.Details,
	})
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, ErrInvalidClaim):
		http.Error(w, err.Error(), http.StatusBadRequest)
	default:
		slog.Error("submit claim", slog.Any("error", err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// handleList answers with the reports the query keeps.
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	f, ok := readFilter(w, r)
	if !ok {
		return
	}

	if id := r.URL.Query().Get("conversation_id"); id != "" {
		if !isUUID(id) {
			http.Error(w, "invalid conversation_id", http.StatusBadRequest)
			return
		}
		f.ConversationID = id
	}

	reports, err := h.svc.List(r.Context(), f)
	if err != nil {
		writeError(w, "list reports", err)
		return
	}

	body := reportListResponse{Reports: make([]reportResponse, 0, len(reports))}
	for _, rep := range reports {
		body.Reports = append(body.Reports, toReportResponse(rep))
	}
	writeJSON(w, http.StatusOK, body)
}

// handleReview answers with one report and the conversation it names.
func (h *Handler) handleReview(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	review, err := h.svc.Review(r.Context(), id)
	if err != nil {
		writeError(w, "review report", err)
		return
	}

	conv := review.Conversation
	body := reviewResponse{
		Report:   toReportResponse(review.Report),
		Reporter: review.Reporter,
		Conversation: conversationResponse{
			ID:           conv.ConversationID,
			TopicID:      conv.TopicID,
			RoomType:     conv.RoomType,
			Participants: len(conv.Participants),
			StartedAt:    conv.StartedAt,
			IsFlagged:    conv.Flagged,
		},
		Messages: make([]messageResponse, 0, len(review.Messages)),
	}
	if !conv.EndedAt.IsZero() {
		body.Conversation.EndedAt = &conv.EndedAt
	}
	if conv.EndReason != "" {
		body.Conversation.EndReason = &conv.EndReason
	}
	for _, msg := range review.Messages {
		body.Messages = append(body.Messages, messageResponse{
			Participant:    msg.Participant,
			Body:           msg.Body,
			ModerationFlag: msg.Flag,
			CreatedAt:      msg.CreatedAt,
		})
	}

	writeJSON(w, http.StatusOK, body)
}

// handleUpdate moves one report to another status.
func (h *Handler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	var req statusRequest
	if !decodeJSON(w, r, &req, maxStatusBytes) {
		return
	}

	rep, err := h.svc.UpdateStatus(r.Context(), id, req.Status)
	if err != nil {
		writeError(w, "update report", err)
		return
	}
	writeJSON(w, http.StatusOK, toReportResponse(rep))
}

// handleListClaims answers with the claims the query keeps.
func (h *Handler) handleListClaims(w http.ResponseWriter, r *http.Request) {
	f, ok := readFilter(w, r)
	if !ok {
		return
	}

	claims, err := h.svc.ListClaims(r.Context(), f)
	if err != nil {
		writeError(w, "list claims", err)
		return
	}

	body := claimListResponse{Claims: make([]claimResponse, 0, len(claims))}
	for _, c := range claims {
		body.Claims = append(body.Claims, toClaimResponse(c))
	}
	writeJSON(w, http.StatusOK, body)
}

// handleUpdateClaim moves one claim to another status.
func (h *Handler) handleUpdateClaim(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	var req statusRequest
	if !decodeJSON(w, r, &req, maxStatusBytes) {
		return
	}

	c, err := h.svc.UpdateClaimStatus(r.Context(), id, req.Status)
	if err != nil {
		writeError(w, "update claim", err)
		return
	}
	writeJSON(w, http.StatusOK, toClaimResponse(c))
}

func toReportResponse(rep Report) reportResponse {
	return reportResponse{
		ID:             rep.ID,
		ConversationID: rep.ConversationID,
		Reason:         rep.Reason,
		Status:         rep.Status,
		CreatedAt:      rep.CreatedAt,
	}
}

func toClaimResponse(c Claim) claimResponse {
	return claimResponse(c)
}

// readFilter reads the query every list takes: status, before and limit.
func readFilter(w http.ResponseWriter, r *http.Request) (Filter, bool) {
	q := r.URL.Query()
	f := Filter{Status: q.Get("status")}

	if v := q.Get("before"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			http.Error(w, "invalid before", http.StatusBadRequest)
			return Filter{}, false
		}
		f.BeforeID = n
	}

	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			http.Error(w, "invalid limit", http.StatusBadRequest)
			return Filter{}, false
		}
		f.Limit = n
	}

	return f, true
}

// isUUID reports whether s is a UUID in its hyphenated text form, which is
// the only form a conversation id is handed out in.
func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
				return false
			}
		}
	}
	return true
}

// pathID reads the ID from the request path.
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

// writeError turns a service error into the answer an operator gets. op names
// the failed operation in the log of an unexpected one.
func writeError(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, ErrUnknownStatus):
		http.Error(w, "unknown status", http.StatusBadRequest)
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	default:
		slog.Error(op, slog.Any("error", err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// decodeJSON reads the request body into dst and answers 400 if it cannot.
// The body must hold exactly one JSON value of at most limit bytes: anything
// after it is a request the client did not mean to send the way this handler
// would read it, so the body has to be at its end once the value is read.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any, limit int64) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
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

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
