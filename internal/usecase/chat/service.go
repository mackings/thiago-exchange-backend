package chat

import (
	"context"

	"github.com/google/uuid"

	"thiagoexchange/backend/internal/domain"
)

type Service struct {
	orders   domain.OrderRepository
	messages domain.OrderMessageRepository
}

func NewService(orders domain.OrderRepository, messages domain.OrderMessageRepository) *Service {
	return &Service{orders: orders, messages: messages}
}

// assertParticipant ensures the caller is either party on the order before
// letting them read or write trade-room messages.
func (s *Service) assertParticipant(ctx context.Context, orderID, userID uuid.UUID) (*domain.Order, error) {
	order, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.BuyerID != userID && order.SellerID != userID {
		return nil, domain.ErrForbidden
	}
	return order, nil
}

func (s *Service) History(ctx context.Context, orderID, userID uuid.UUID) ([]*domain.OrderMessage, error) {
	if _, err := s.assertParticipant(ctx, orderID, userID); err != nil {
		return nil, err
	}
	return s.messages.ListByOrder(ctx, orderID)
}

// Send also returns the order — callers (the WS handler) use it to decide
// whether the message came from the trader side and is worth surfacing as
// an admin notification.
func (s *Service) Send(ctx context.Context, orderID, senderID uuid.UUID, body, attachmentURL string) (*domain.OrderMessage, *domain.Order, error) {
	order, err := s.assertParticipant(ctx, orderID, senderID)
	if err != nil {
		return nil, nil, err
	}
	if body == "" && attachmentURL == "" {
		return nil, nil, domain.ErrInvalidInput
	}
	msg := &domain.OrderMessage{
		ID:            uuid.New(),
		OrderID:       orderID,
		SenderID:      senderID,
		Body:          body,
		AttachmentURL: attachmentURL,
	}
	if err := s.messages.Create(ctx, msg); err != nil {
		return nil, nil, err
	}
	return msg, order, nil
}
