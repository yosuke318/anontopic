package session

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// cookieName carries the token. The cookie is HttpOnly, so the token never
// reaches page scripts, and browsers attach it to the WebSocket handshake on
// their own.
const cookieName = "anontopic_session"

// contextKey scopes the token stored on a request context to this package.
type contextKey struct{}

// Handler exposes the session module's HTTP surface.
type Handler struct {
	svc *Service
}

// NewHandler builds a handler around svc.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Register mounts the module's routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/session", h.handleIssue)
	mux.HandleFunc("DELETE /api/session", h.handleRevoke)
}

// sessionResponse tells the client how long its session lasts. The token
// itself stays in the cookie.
type sessionResponse struct {
	ExpiresAt time.Time `json:"expires_at"`
}

// handleIssue hands the caller a usable session: it extends the one the
// request carries, or creates a new one.
func (h *Handler) handleIssue(w http.ResponseWriter, r *http.Request) {
	if token, ok := tokenFromRequest(r); ok {
		sess, err := h.svc.Verify(r.Context(), token)
		switch {
		case err == nil:
			h.svc.writeCookie(w, sess)
			writeJSON(w, http.StatusOK, sessionResponse{ExpiresAt: sess.ExpiresAt})
			return
		case !errors.Is(err, ErrInvalidSession):
			slog.Error("verify session", slog.Any("error", err))
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	sess, err := h.svc.Issue(r.Context(), r)
	if err != nil {
		slog.Error("issue session", slog.Any("error", err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	h.svc.writeCookie(w, sess)
	writeJSON(w, http.StatusCreated, sessionResponse{ExpiresAt: sess.ExpiresAt})
}

// handleRevoke ends the session the request carries. Leaving without a valid
// session is the same outcome as leaving with one, so both answer 204.
func (h *Handler) handleRevoke(w http.ResponseWriter, r *http.Request) {
	if token, ok := tokenFromRequest(r); ok {
		if err := h.svc.Revoke(r.Context(), token); err != nil && !errors.Is(err, ErrInvalidSession) {
			slog.Error("revoke session", slog.Any("error", err))
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	h.svc.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

// RequireSession rejects requests without a valid session and passes the token
// of accepted ones to next through the request context.
func (s *Service) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := s.Authenticate(r)
		switch {
		case errors.Is(err, ErrInvalidSession):
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		case err != nil:
			slog.Error("authenticate session", slog.Any("error", err))
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		next.ServeHTTP(w, r.WithContext(WithToken(r.Context(), token)))
	})
}

// WithToken puts token on ctx.
func WithToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, contextKey{}, token)
}

// TokenFrom returns the token RequireSession accepted for this request.
func TokenFrom(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(contextKey{}).(string)
	return token, ok
}

// tokenFromRequest reads the token a request carries.
func tokenFromRequest(r *http.Request) (string, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return "", false
	}
	return c.Value, true
}

// writeCookie makes the client send sess back until it expires.
func (s *Service) writeCookie(w http.ResponseWriter, sess Session) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    sess.Token,
		Path:     "/",
		MaxAge:   int(sess.ExpiresAt.Sub(s.now()).Seconds()),
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: s.cookieSameSite,
	})
}

// clearCookie drops the cookie from the client.
func (s *Service) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.cookieSecure,
		SameSite: s.cookieSameSite,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
