// Package chat owns realtime room messaging: WebSocket connections,
// message fan-out inside a room, and room lifecycle events.
//
// Boundary: other modules must never import chat's persistence models.
// Cross-module communication goes through the exported interfaces below.
package chat

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
)

// SessionAuthenticator resolves the session a request carries and returns the
// token identifying the participant.
type SessionAuthenticator interface {
	Authenticate(r *http.Request) (string, error)
}

// Handler exposes the chat module's HTTP/WebSocket surface.
type Handler struct {
	upgrader websocket.Upgrader
	sessions SessionAuthenticator
}

// NewHandler builds a chat handler that accepts WebSocket handshakes from
// allowedOrigins, plus those whose Origin matches the host they were sent to.
func NewHandler(sessions SessionAuthenticator, allowedOrigins []string) *Handler {
	return &Handler{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     originChecker(allowedOrigins),
		},
		sessions: sessions,
	}
}

// Register mounts the module's routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /ws/rooms/{roomID}", h.handleRoomSocket)
}

// handleRoomSocket admits a participant to a room over a WebSocket connection.
//
// Room join, message fan-out and disconnect handling are not implemented yet,
// so an authenticated handshake answers 501.
func (h *Handler) handleRoomSocket(w http.ResponseWriter, r *http.Request) {
	if _, err := h.sessions.Authenticate(r); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	http.Error(w, "not implemented", http.StatusNotImplemented)
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
