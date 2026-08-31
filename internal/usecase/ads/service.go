package ads

import (
	"context"

	"github.com/google/uuid"

	"thiagoexchange/backend/internal/domain"
)

type Service struct {
	ads   domain.AdRepository
	users domain.UserRepository
}

func NewService(ads domain.AdRepository, users domain.UserRepository) *Service {
	return &Service{ads: ads, users: users}
}

type CreateAdInput struct {
	OwnerID           uuid.UUID
	Side              domain.AdSide
	Asset             string
	Fiat              string
	RateType          domain.RateType
	FixedRate         float64
	FloatingMarginPct float64
	MinLimit          float64
	MaxLimit          float64
	AvailableAmount   float64
	PaymentMethods    string
	Terms             string
}

// Create is restricted to admin accounts: Thiago Exchange is always the
// counterparty on every trade ("trade with us"), not a peer marketplace
// where any verified user can list ads. Buyers and sellers still transact
// freely against Thiago's own ads — this only gates who may post one.
func (s *Service) Create(ctx context.Context, in CreateAdInput) (*domain.Ad, error) {
	owner, err := s.users.GetByID(ctx, in.OwnerID)
	if err != nil {
		return nil, err
	}
	if owner.Role != domain.RoleAdmin {
		return nil, domain.ErrForbidden
	}
	if in.Asset == "" || in.AvailableAmount <= 0 || in.MinLimit <= 0 || in.MaxLimit < in.MinLimit {
		return nil, domain.ErrInvalidInput
	}
	if in.Fiat == "" {
		in.Fiat = "NGN"
	}
	ad := &domain.Ad{
		ID:                uuid.New(),
		OwnerID:           in.OwnerID,
		Side:              in.Side,
		Asset:             in.Asset,
		Fiat:              in.Fiat,
		RateType:          in.RateType,
		FixedRate:         in.FixedRate,
		FloatingMarginPct: in.FloatingMarginPct,
		MinLimit:          in.MinLimit,
		MaxLimit:          in.MaxLimit,
		AvailableAmount:   in.AvailableAmount,
		PaymentMethods:    in.PaymentMethods,
		Terms:             in.Terms,
		Status:            domain.AdStatusActive,
	}
	if err := s.ads.Create(ctx, ad); err != nil {
		return nil, err
	}
	return ad, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*domain.Ad, error) {
	return s.ads.GetByID(ctx, id)
}

func (s *Service) List(ctx context.Context, f domain.AdFilter) ([]*domain.Ad, error) {
	return s.ads.List(ctx, f)
}

func (s *Service) ListMine(ctx context.Context, ownerID uuid.UUID) ([]*domain.Ad, error) {
	return s.ads.ListByOwner(ctx, ownerID)
}

type UpdateAdInput struct {
	Status            *domain.AdStatus
	AvailableAmount   *float64
	FixedRate         *float64
	FloatingMarginPct *float64
	MinLimit          *float64
	MaxLimit          *float64
	Terms             *string
}

func (s *Service) Update(ctx context.Context, id uuid.UUID, ownerID uuid.UUID, in UpdateAdInput) (*domain.Ad, error) {
	ad, err := s.ads.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if ad.OwnerID != ownerID {
		return nil, domain.ErrForbidden
	}
	if in.Status != nil {
		ad.Status = *in.Status
	}
	if in.AvailableAmount != nil {
		ad.AvailableAmount = *in.AvailableAmount
	}
	if in.FixedRate != nil {
		ad.FixedRate = *in.FixedRate
	}
	if in.FloatingMarginPct != nil {
		ad.FloatingMarginPct = *in.FloatingMarginPct
	}
	if in.MinLimit != nil {
		ad.MinLimit = *in.MinLimit
	}
	if in.MaxLimit != nil {
		ad.MaxLimit = *in.MaxLimit
	}
	if in.Terms != nil {
		ad.Terms = *in.Terms
	}
	if ad.MinLimit <= 0 || ad.MaxLimit < ad.MinLimit {
		return nil, domain.ErrInvalidInput
	}
	if err := s.ads.Update(ctx, ad); err != nil {
		return nil, err
	}
	return ad, nil
}
