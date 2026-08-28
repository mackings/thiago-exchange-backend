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
	upgrader websocket.Upgrader
}

func NewChatHandler(svc *chat.Service, hub *ws.Hub, allowedOrigin string) *ChatHandler {
	return &ChatHandler{
		svc: svc, hub: hub,
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
		msg, err := h.svc.Send(context.Background(), client.OrderID, client.UserID, in.Body, in.AttachmentURL)
		if err != nil {
			log.Printf("chat: send failed: %v", err)
			continue
		}
		payload, err := json.Marshal(toMessageDTO(msg))
		if err != nil {
			continue
		}
		h.hub.Broadcast(client.OrderID, payload)
	}
}

func (h *ChatHandler) writePump(client *ws.Client) {
	for msg := range client.Send {
		if err := client.Conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			return
		}
	}
}
