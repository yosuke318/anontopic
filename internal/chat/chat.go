// Package chat owns realtime room messaging: WebSocket connections,
// message fan-out inside a room, and room lifecycle events.
//
// Boundary: other modules must never import chat's persistence models.
// Cross-module communication goes through the exported interfaces below.
package chat

import (
	"net/http"

	"github.com/gorilla/websocket"
)

// Handler exposes the chat module's HTTP/WebSocket surface.
type Handler struct {
	upgrader websocket.Upgrader
}

// NewHandler builds a chat handler with the default WebSocket upgrader.
func NewHandler() *Handler {
	return &Handler{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
	}
}

// Register mounts the module's routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /ws/rooms/{roomID}", h.handleRoomSocket)
}

// handleRoomSocket upgrades the request to a WebSocket connection.
//
// Room join, message fan-out and disconnect handling are not implemented yet,
// so the endpoint answers 501.
func (h *Handler) handleRoomSocket(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
