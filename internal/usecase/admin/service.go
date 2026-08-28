package admin

import (
	"context"

	"github.com/google/uuid"

	"thiagoexchange/backend/internal/domain"
)

// BybitAccount is the narrow slice of the Bybit client admin needs: a live
// balance check so ops can verify real funds before releasing an order.
// Defined here (consumer-side) rather than in domain, since it's an
// operational concern, not part of the marketplace's core model.
type BybitAccount interface {
	WalletBalance(ctx context.Context) (map[string]float64, error)
}

type Service struct {
	users domain.UserRepository
	bybit BybitAccount
}

func NewService(users domain.UserRepository, bybit BybitAccount) *Service {
	return &Service{users: users, bybit: bybit}
}

func (s *Service) ListUsers(ctx context.Context) ([]*domain.User, error) {
	return s.users.List(ctx, 0, 0)
}

func (s *Service) SetDisabled(ctx context.Context, userID uuid.UUID, disabled bool) error {
	u, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return err
	}
	u.Disabled = disabled
	return s.users.Update(ctx, u)
}

// BybitBalance returns the platform's real Bybit account balance, used as
// the operational backing check before confirming order releases. Returns
// an error if BYBIT_API_KEY/SECRET aren't configured.
func (s *Service) BybitBalance(ctx context.Context) (map[string]float64, error) {
	return s.bybit.WalletBalance(ctx)
}
