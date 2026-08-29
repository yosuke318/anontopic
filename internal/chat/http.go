package chat

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

// Handler exposes the chat module's HTTP/WebSocket surface.
type Handler struct {
	svc         *Service
	sessions    SessionAuthenticator
	upgrader    websocket.Upgrader
	checkOrigin func(*http.Request) bool
}

// NewHandler builds a chat handler that accepts WebSocket handshakes from
// allowedOrigins, plus those whose Origin matches the host they were sent to.
func NewHandler(svc *Service, sessions SessionAuthenticator, allowedOrigins []string) *Handler {
	// The handler and the upgrader share one function so that both stages of
	// the handshake accept the same set of sites.
	checkOrigin := originChecker(allowedOrigins)

	return &Handler{
		svc:      svc,
		sessions: sessions,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     checkOrigin,
		},
		checkOrigin: checkOrigin,
	}
}

// Register mounts the module's routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /ws/rooms/{roomID}", h.handleRoomSocket)
}

// handleRoomSocket admits a participant to a room over a WebSocket
// connection, and serves that connection until it closes.
func (h *Handler) handleRoomSocket(w http.ResponseWriter, r *http.Request) {
	// The Origin is checked before the session, because verifying a session
	// extends its lifetime and a page on another site must not be able to keep
	// someone else's session alive.
	if !h.checkOrigin(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	token, err := h.sessions.Authenticate(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	adm, err := h.svc.Admit(r.Context(), r.PathValue("roomID"), token)
	if err != nil {
		writeAdmissionError(w, err)
		return
	}

	ws, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade has answered the client already.
		slog.Info("websocket handshake failed", slog.Any("error", err))
		return
	}

	// The request runs for as long as the connection does: a hijacked
	// connection keeps its request context, and the server does not wait for
	// it when it shuts down.
	h.svc.Serve(r.Context(), ws, adm)
}

// writeAdmissionError answers a handshake the room did not admit. A
// conversation somebody else's is answered the same way as one that does not
// exist, so that a caller cannot learn which conversations are being held.
func writeAdmissionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrConversationNotFound), errors.Is(err, ErrNotParticipant):
		http.Error(w, "forbidden", http.StatusForbidden)
	case errors.Is(err, ErrConversationEnded):
		http.Error(w, "the conversation has ended", http.StatusGone)
	default:
		slog.Error("admit to room", slog.Any("error", err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// originChecker guards the handshake against pages on other sites: the session
// cookie is attached by the browser whatever site opened the connection, so
// the Origin header is what separates our own frontend from someone else's.
func originChecker(allowedOrigins []string) func(*http.Request) bool {
	allowed := make(map[string]struct{}, len(allowedOrigins))
	for _, origin := range allowedOrigins {
		allowed[strings.ToLower(strings.TrimSpace(origin))] = struct{}{}
	}

	return func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			// Clients that are not browsers send no Origin and hold no cookie
			// jar, so the token they present is one they were given.
			return true
		}

		if _, ok := allowed[strings.ToLower(origin)]; ok {
			return true
		}

		u, err := url.Parse(origin)
		return err == nil && strings.EqualFold(u.Host, r.Host)
	}
}
