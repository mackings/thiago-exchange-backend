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
	// ListActive returns orders still needing attention (everything except
	// completed/cancelled), newest first — used by the admin verification
	// queue.
	ListActive(ctx context.Context, limit, offset int) ([]*Order, error)
	// ListAll returns every order regardless of status, newest first — used
	// by the admin dashboard's transactions table.
	ListAll(ctx context.Context, limit, offset int) ([]*Order, error)
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

// WhitelistedAddress records that a payout address has already gone through
// Bybit's one-time, human-confirmed (email link) address-book whitelisting.
// Bybit exposes no API to add or check this itself, so we track it
// ourselves: once an address is marked whitelisted here, its future
// Withdraw calls no longer need an admin to have manually confirmed it on
// Bybit's site first — the first payout to a new address always does.
type WhitelistedAddress struct {
	ID             uuid.UUID
	Address        string
	Chain          string
	Asset          string
	AddedByAdminID uuid.UUID
	CreatedAt      time.Time
}

type WhitelistRepository interface {
	Create(ctx context.Context, w *WhitelistedAddress) error
	IsWhitelisted(ctx context.Context, address string) (bool, error)
	ListAll(ctx context.Context) ([]*WhitelistedAddress, error)
}

// DepositAddress is the address a taker should send crypto to when selling
// into one of our buy ads. Bybit has no private endpoint we can safely call
// without real API keys configured to fetch this live, so instead an admin
// copies it once per asset from Bybit's own deposit page and it's served
// straight out of the database from then on — same reasoning as
// WhitelistedAddress above, just for the inbound side instead of the
// outbound one.
type DepositAddress struct {
	ID             uuid.UUID
	Asset          string
	Chain          string
	Address        string
	Tag            string
	AddedByAdminID uuid.UUID
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type DepositAddressRepository interface {
	// Upsert sets the current deposit address for an asset, replacing
	// whatever was set before it — there's only ever one live address per
	// asset at a time.
	Upsert(ctx context.Context, d *DepositAddress) error
	GetByAsset(ctx context.Context, asset string) (*DepositAddress, error)
	ListAll(ctx context.Context) ([]*DepositAddress, error)
}
