package handlers

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"thiagoexchange/backend/internal/delivery/http/middleware"
	"thiagoexchange/backend/internal/domain"
	"thiagoexchange/backend/internal/infra/ws"
	"thiagoexchange/backend/internal/usecase/chat"
)

type ChatHandler struct {
	svc      *chat.Service
	hub      *ws.Hub
	users    domain.UserRepository
	upgrader websocket.Upgrader
}

func NewChatHandler(svc *chat.Service, hub *ws.Hub, users domain.UserRepository, allowedOrigin string) *ChatHandler {
	return &ChatHandler{
		svc: svc, hub: hub, users: users,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") == allowedOrigin },
		},
	}
}

type messageDTO struct {
	ID            string `json:"id"`
	OrderID       string `json:"orderId"`
	SenderID      string `json:"senderId"`
	Body          string `json:"body"`
	AttachmentURL string `json:"attachmentUrl,omitempty"`
	CreatedAt     string `json:"createdAt"`
}

func toMessageDTO(m *domain.OrderMessage) messageDTO {
	return messageDTO{
		ID: m.ID.String(), OrderID: m.OrderID.String(), SenderID: m.SenderID.String(),
		Body: m.Body, AttachmentURL: m.AttachmentURL, CreatedAt: m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (h *ChatHandler) History(c *gin.Context) {
	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	msgs, err := h.svc.History(c.Request.Context(), orderID, middleware.CurrentUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	out := make([]messageDTO, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, toMessageDTO(m))
	}
	c.JSON(http.StatusOK, out)
}

type incomingMessage struct {
	Body          string `json:"body"`
	AttachmentURL string `json:"attachmentUrl"`
}

// Stream upgrades to a WebSocket for the given order's trade room. Access is
// checked up front via History (which enforces the caller is a participant)
// before the handshake completes.
func (h *ChatHandler) Stream(c *gin.Context) {
	orderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	userID := middleware.CurrentUserID(c)
	if _, err := h.svc.History(c.Request.Context(), orderID, userID); err != nil {
		respondError(c, err)
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := &ws.Client{Conn: conn, OrderID: orderID, UserID: userID, Send: make(chan []byte, 16)}
	h.hub.Register(client)

	go h.writePump(client)
	h.readPump(client)
}

func (h *ChatHandler) readPump(client *ws.Client) {
	defer func() {
		h.hub.Unregister(client)
		client.Conn.Close()
	}()
	for {
		_, raw, err := client.Conn.ReadMessage()
		if err != nil {
			return
		}
		var in incomingMessage
		if err := json.Unmarshal(raw, &in); err != nil {
			continue
		}
		msg, order, err := h.svc.Send(context.Background(), client.OrderID, client.UserID, in.Body, in.AttachmentURL)
		if err != nil {
			log.Printf("chat: send failed: %v", err)
			continue
		}
		payload, err := json.Marshal(toMessageDTO(msg))
		if err != nil {
			continue
		}
		h.hub.Broadcast(client.OrderID, payload)

		// The merchant side of every order is Thiago's own admin account —
		// a message from the OTHER side is always the trader, which is the
		// case admin actually needs paging for (their own messages, sent
		// from the compact chat embedded in the admin console, obviously
		// don't need to notify admin).
		if client.UserID != order.MerchantID {
			h.notifyAdmins(order, msg)
		}
	}
}

func (h *ChatHandler) notifyAdmins(order *domain.Order, msg *domain.OrderMessage) {
	senderName := "Trader"
	if u, err := h.users.GetByID(context.Background(), msg.SenderID); err == nil {
		senderName = u.FullName
	}
	preview := msg.Body
	if preview == "" && msg.AttachmentURL != "" {
		preview = "Sent an attachment"
	}
	payload, err := json.Marshal(adminNotificationDTO{
		Type: "new_message", OrderID: order.ID.String(), SenderName: senderName,
		Preview: preview, Asset: order.Asset, Amount: order.Amount,
	})
	if err != nil {
		return
	}
	h.hub.BroadcastAdmins(payload)
}

func (h *ChatHandler) writePump(client *ws.Client) {
	for msg := range client.Send {
		if err := client.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}

type adminNotificationDTO struct {
	Type       string  `json:"type"`
	OrderID    string  `json:"orderId"`
	SenderName string  `json:"senderName"`
	Preview    string  `json:"preview"`
	Asset      string  `json:"asset"`
	Amount     float64 `json:"amount"`
}

// StreamNotifications is a single, order-agnostic WebSocket admin holds open
// for as long as they're on the admin console — see Hub's admin-broadcast
// comment for why this is a separate channel from the per-order rooms.
// Role is already enforced by the router's admin group middleware.
func (h *ChatHandler) StreamNotifications(c *gin.Context) {
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	client := &ws.Client{Conn: conn, UserID: middleware.CurrentUserID(c), Send: make(chan []byte, 16)}
	h.hub.RegisterAdmin(client)

	go h.writePump(client)
	// No inbound messages expected on this channel — just block reading
	// until the connection drops, so we notice and unregister.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			h.hub.UnregisterAdmin(client)
			conn.Close()
			return
		}
	}
}
