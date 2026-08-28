package dispute

import (
	"context"

	"github.com/google/uuid"

	"thiagoexchange/backend/internal/domain"
	"thiagoexchange/backend/internal/usecase/orders"
)

type Service struct {
	disputes domain.DisputeRepository
	orders   *orders.Service
}

func NewService(disputes domain.DisputeRepository, ordersSvc *orders.Service) *Service {
	return &Service{disputes: disputes, orders: ordersSvc}
}

func (s *Service) Raise(ctx context.Context, orderID, raisedBy uuid.UUID, reason string) (*domain.Dispute, error) {
	order, err := s.orders.Get(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.BuyerID != raisedBy && order.SellerID != raisedBy {
		return nil, domain.ErrForbidden
	}
	if _, err := s.orders.MarkDisputed(ctx, orderID); err != nil {
		return nil, err
	}
	d := &domain.Dispute{
		ID:       uuid.New(),
		OrderID:  orderID,
		RaisedBy: raisedBy,
		Reason:   reason,
		Status:   domain.DisputeStatusOpen,
	}
	if err := s.disputes.Create(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *Service) ListOpen(ctx context.Context) ([]*domain.Dispute, error) {
	return s.disputes.ListOpen(ctx, 0, 0)
}

func (s *Service) Resolve(ctx context.Context, disputeID, adminID uuid.UUID, resolution domain.DisputeResolution) (*domain.Dispute, error) {
	d, err := s.disputes.GetByID(ctx, disputeID)
	if err != nil {
		return nil, err
	}
	if d.Status != domain.DisputeStatusOpen {
		return nil, domain.ErrInvalidOrderState
	}

	switch resolution {
	case domain.ResolutionReleaseToBuyer:
		if _, err := s.orders.ReleaseFromDispute(ctx, d.OrderID, adminID); err != nil {
			return nil, err
		}
	case domain.ResolutionRefundToSeller:
		if _, err := s.orders.RefundFromDispute(ctx, d.OrderID, adminID); err != nil {
			return nil, err
		}
	default:
		return nil, domain.ErrInvalidInput
	}

	d.Status = domain.DisputeStatusResolved
	d.Resolution = resolution
	d.ResolvedBy = &adminID
	if err := s.disputes.Update(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}
