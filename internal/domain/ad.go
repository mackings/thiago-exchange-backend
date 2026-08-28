package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type AdSide string

const (
	AdSideBuy  AdSide = "buy"  // owner wants to buy crypto (i.e. taker sells to them)
	AdSideSell AdSide = "sell" // owner wants to sell crypto (i.e. taker buys from them)
)

type RateType string

const (
	RateTypeFixed          RateType = "fixed"
	RateTypeFloatingMargin RateType = "floating_margin"
)

type AdStatus string

const (
	AdStatusActive AdStatus = "active"
	AdStatusPaused AdStatus = "paused"
	AdStatusClosed AdStatus = "closed"
)

type Ad struct {
	ID                uuid.UUID
	OwnerID           uuid.UUID
	Side              AdSide
	Asset             string // e.g. USDT, BTC
	Fiat              string
	RateType          RateType
	FixedRate         float64 // NGN per unit, used when RateType = fixed
	FloatingMarginPct float64 // e.g. 1.5 means 1.5% above/below Bybit reference
	MinLimit          float64 // fiat amount
	MaxLimit          float64 // fiat amount
	AvailableAmount   float64 // asset amount still open for orders
	PaymentMethods    string  // comma-separated labels, e.g. "bank_transfer,opay"
	Terms             string
	Status            AdStatus
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type AdFilter struct {
	Side   *AdSide
	Asset  string
	Fiat   string
	Limit  int
	Offset int
}

type AdRepository interface {
	Create(ctx context.Context, a *Ad) error
	GetByID(ctx context.Context, id uuid.UUID) (*Ad, error)
	Update(ctx context.Context, a *Ad) error
	List(ctx context.Context, f AdFilter) ([]*Ad, error)
	ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]*Ad, error)
}
