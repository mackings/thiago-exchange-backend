package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type OrderStatus string

const (
	OrderStatusCreated          OrderStatus = "created"
	OrderStatusAwaitingPayment  OrderStatus = "awaiting_payment"
	OrderStatusPaymentMarked    OrderStatus = "payment_marked"
	OrderStatusPaymentConfirmed OrderStatus = "payment_confirmed"
	OrderStatusReleased         OrderStatus = "released"
	OrderStatusCompleted        OrderStatus = "completed"
	OrderStatusCancelled        OrderStatus = "cancelled"
	OrderStatusDisputed         OrderStatus = "disputed"
)

// validOrderTransitions defines the allowed forward transitions for the order
// state machine. Cancelled/disputed are reachable from most active states and
// are checked separately in the orders usecase.
var validOrderTransitions = map[OrderStatus][]OrderStatus{
	OrderStatusCreated:          {OrderStatusAwaitingPayment},
	OrderStatusAwaitingPayment:  {OrderStatusPaymentMarked},
	OrderStatusPaymentMarked:    {OrderStatusPaymentConfirmed},
	OrderStatusPaymentConfirmed: {OrderStatusReleased},
	OrderStatusReleased:         {OrderStatusCompleted},
}

func CanTransition(from, to OrderStatus) bool {
	for _, s := range validOrderTransitions[from] {
		if s == to {
			return true
		}
	}
	return false
}

// activeStates are order states from which cancel/dispute is still allowed.
var cancellableStates = map[OrderStatus]bool{
	OrderStatusCreated:         true,
	OrderStatusAwaitingPayment: true,
}

func IsCancellable(s OrderStatus) bool { return cancellableStates[s] }

var disputableStates = map[OrderStatus]bool{
	OrderStatusAwaitingPayment:  true,
	OrderStatusPaymentMarked:    true,
	OrderStatusPaymentConfirmed: true,
}

func IsDisputable(s OrderStatus) bool { return disputableStates[s] }

type Order struct {
	ID         uuid.UUID
	AdID       uuid.UUID
	MerchantID uuid.UUID // always the ad owner (Thiago); the other side is whichever of Buyer/SellerID isn't the merchant
	Side       AdSide    // copied from the ad at creation time, so downstream logic doesn't need to re-fetch it
	BuyerID    uuid.UUID
	SellerID   uuid.UUID
	Asset      string
	Fiat       string
	Amount     float64 // asset amount
	Rate       float64 // fiat per unit, locked at order creation
	FiatAmount float64
	Status     OrderStatus

	// Sell-side (Thiago selling, taker is buyer): where to send the real
	// payout. Required at order creation; used for the Bybit withdrawal on
	// release.
	PayoutAddress string
	PayoutChain   string

	// Buy-side (Thiago buying, taker is seller): the taker's on-chain
	// transaction hash and the verified amount Bybit's deposit record shows
	// for it, filled in once SubmitDeposit confirms it.
	DepositTxID   string
	DepositChain  string
	DepositAmount float64

	PaymentDeadline time.Time
	PaymentProofURL string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type OrderRepository interface {
	Create(ctx context.Context, o *Order) error
	GetByID(ctx context.Context, id uuid.UUID) (*Order, error)
	Update(ctx context.Context, o *Order) error
	ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*Order, error)
}

type OrderMessage struct {
	ID            uuid.UUID
	OrderID       uuid.UUID
	SenderID      uuid.UUID
	Body          string
	AttachmentURL string
	CreatedAt     time.Time
}

type OrderMessageRepository interface {
	Create(ctx context.Context, m *OrderMessage) error
	ListByOrder(ctx context.Context, orderID uuid.UUID) ([]*OrderMessage, error)
}

type DisputeStatus string

const (
	DisputeStatusOpen     DisputeStatus = "open"
	DisputeStatusResolved DisputeStatus = "resolved"
)

type DisputeResolution string

const (
	ResolutionReleaseToBuyer DisputeResolution = "release_to_buyer"
	ResolutionRefundToSeller DisputeResolution = "refund_to_seller"
)

type Dispute struct {
	ID         uuid.UUID
	OrderID    uuid.UUID
	RaisedBy   uuid.UUID
	Reason     string
	Status     DisputeStatus
	Resolution DisputeResolution
	ResolvedBy *uuid.UUID
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type DisputeRepository interface {
	Create(ctx context.Context, d *Dispute) error
	GetByID(ctx context.Context, id uuid.UUID) (*Dispute, error)
	GetByOrderID(ctx context.Context, orderID uuid.UUID) (*Dispute, error)
	ListOpen(ctx context.Context, limit, offset int) ([]*Dispute, error)
	Update(ctx context.Context, d *Dispute) error
}
