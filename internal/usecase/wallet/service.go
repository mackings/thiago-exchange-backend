package wallet

import (
	"context"

	"github.com/google/uuid"

	"thiagoexchange/backend/internal/domain"
)

type Service struct {
	ledger domain.LedgerRepository
}

func NewService(ledger domain.LedgerRepository) *Service {
	return &Service{ledger: ledger}
}

func (s *Service) Balances(ctx context.Context, userID uuid.UUID) ([]domain.Balance, error) {
	return s.ledger.BalancesByUser(ctx, userID)
}

func (s *Service) History(ctx context.Context, userID uuid.UUID) ([]*domain.LedgerEntry, error) {
	return s.ledger.ListByUser(ctx, userID, 0, 0)
}

// AdminCredit records a manual funding entry — used once ops has confirmed a
// user genuinely moved crypto into Thiago's Bybit account, so that crypto
// becomes available for the user to back a sell ad with in-app.
func (s *Service) AdminCredit(ctx context.Context, userID, adminID uuid.UUID, asset string, amount float64, note string) error {
	if amount <= 0 {
		return domain.ErrInvalidInput
	}
	entry := &domain.LedgerEntry{
		ID:               uuid.New(),
		UserID:           userID,
		Asset:            asset,
		Bucket:           domain.BucketAvailable,
		Direction:        domain.DirectionIn,
		Amount:           amount,
		Reason:           domain.ReasonAdminFunding,
		CreatedByAdminID: &adminID,
		Note:             note,
	}
	return s.ledger.Create(ctx, entry)
}
