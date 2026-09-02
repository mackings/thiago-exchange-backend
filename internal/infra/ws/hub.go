// Package ws provides a minimal per-order-room WebSocket broadcast hub for
// trade-room chat. Connection lifecycle and message persistence live in the
// HTTP delivery layer (chat_handler.go) — this package only tracks which
// connections belong to which room and fans out broadcasts.
package ws

import (
	"sync"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Client struct {
	Conn    *websocket.Conn
	OrderID uuid.UUID
	UserID  uuid.UUID
	Send    chan []byte
}

type Hub struct {
	mu     sync.RWMutex
	rooms  map[uuid.UUID]map[*Client]bool
	admins map[*Client]bool
}

func NewHub() *Hub {
	return &Hub{rooms: make(map[uuid.UUID]map[*Client]bool), admins: make(map[*Client]bool)}
}

func (h *Hub) Register(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.rooms[c.OrderID] == nil {
		h.rooms[c.OrderID] = make(map[*Client]bool)
	}
	h.rooms[c.OrderID][c] = true
}

func (h *Hub) Unregister(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if room, ok := h.rooms[c.OrderID]; ok {
		delete(room, c)
		close(c.Send)
		if len(room) == 0 {
			delete(h.rooms, c.OrderID)
		}
	}
}

// Broadcast sends a message to every connection currently in orderID's room.
func (h *Hub) Broadcast(orderID uuid.UUID, message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.rooms[orderID] {
		select {
		case c.Send <- message:
		default:
			// slow consumer, drop rather than block the broadcaster
		}
	}
}

// RegisterAdmin/UnregisterAdmin/BroadcastAdmins back a separate, order-agnostic
// channel: admin's console holds one persistent connection here for as long
// as they're on the admin pages, so they get notified of a new trader
// message on ANY order — not just the one room they might currently have
// open — which is what actually lets them notice and respond promptly.
func (h *Hub) RegisterAdmin(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.admins[c] = true
}

func (h *Hub) UnregisterAdmin(c *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.admins[c] {
		delete(h.admins, c)
		close(c.Send)
	}
}

func (h *Hub) BroadcastAdmins(message []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.admins {
		select {
		case c.Send <- message:
		default:
		}
	}
}
