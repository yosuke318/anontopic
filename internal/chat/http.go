package chat

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

// retryAfterSeconds is what a client refused for capacity is asked to wait
// before it tries again. It is long enough that the refused clients do not
// come back as one wave, and short enough that a seat freed by someone
// leaving is taken soon after.
const retryAfterSeconds = "10"

// Handler exposes the chat module's HTTP/WebSocket surface.
type Handler struct {
	svc         *Service
	sessions    SessionAuthenticator
	connections ConnectionLimiter
	upgrader    websocket.Upgrader
	checkOrigin func(*http.Request) bool
}

// NewHandler builds a chat handler that accepts WebSocket handshakes from
// allowedOrigins, plus those whose Origin matches the host they were sent to.
// A nil connections limiter accepts as many connections as arrive.
func NewHandler(svc *Service, sessions SessionAuthenticator, connections ConnectionLimiter, allowedOrigins []string) *Handler {
	// The handler and the upgrader share one function so that both stages of
	// the handshake accept the same set of sites.
	checkOrigin := originChecker(allowedOrigins)

	return &Handler{
		svc:         svc,
		sessions:    sessions,
		connections: connections,
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

	// The connection is counted before the session is read, so that the
	// connections the service is refusing cost it a single Redis call each.
	// The place is held until this handler returns, which is when the
	// connection it counts is closed.
	release, err := h.acquire(r)
	if err != nil {
		writeCapacityError(w, err)
		return
	}
	defer release()

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

// acquire counts the connection r is asking for and returns the function that
// gives its place back. Without a limiter nothing is counted: every handshake
// goes through, and the function it returns does nothing.
func (h *Handler) acquire(r *http.Request) (func(), error) {
	if h.connections == nil {
		return func() {}, nil
	}
	return h.connections.Acquire(r.Context(), h.sessions.IPHash(r))
}

// writeCapacityError answers a handshake there was no room for. A service
// holding every connection it may hold answers 503, and an address holding
// every connection it may hold answers 429, so that a client can tell a busy
// service from its own doing.
func writeCapacityError(w http.ResponseWriter, err error) {
	var refused Refusal
	if !errors.As(err, &refused) {
		slog.Error("count the connection", slog.Any("error", err))
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Retry-After", retryAfterSeconds)
	if refused.AtCapacity() {
		http.Error(w, "the service is busy", http.StatusServiceUnavailable)
		return
	}
	http.Error(w, "too many connections", http.StatusTooManyRequests)
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
