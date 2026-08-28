package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// LedgerReason is a free-form audit label — it does not drive balance math,
// Bucket + Direction do. Keeping it separate avoids the double-counting bug
// that shows up when a single "type" tries to encode both.
type LedgerReason string

const (
	ReasonAdminFunding    LedgerReason = "admin_funding"    // admin credits a user after confirming a real deposit into Thiago's Bybit account
	ReasonAdminAdjust     LedgerReason = "admin_adjust"     // manual correction
	ReasonOrderLock       LedgerReason = "order_lock"       // seller's asset locked when an order is opened against their ad
	ReasonOrderRelease    LedgerReason = "order_release"    // seller's locked asset leaves the system, buyer is credited
	ReasonOrderCancel     LedgerReason = "order_cancel"     // locked amount returned to seller's available balance
	ReasonOrderDispute    LedgerReason = "order_dispute"    // dispute resolution (release or refund, reuses release/cancel entries)
	ReasonWithdrawal      LedgerReason = "withdrawal"       // admin-assisted withdrawal out of the platform
	ReasonDepositCredited LedgerReason = "deposit_credited" // buy-side order: seller's verified Bybit deposit becomes merchant inventory
)

type LedgerBucket string

const (
	BucketAvailable LedgerBucket = "available"
	BucketLocked    LedgerBucket = "locked"
)

type LedgerDirection string

const (
	DirectionIn  LedgerDirection = "in"
	DirectionOut LedgerDirection = "out"
)

// LedgerEntry is an append-only record. Balances are always derived by summing
// entries for a user+asset+bucket, never stored/mutated directly, so the
// ledger stays auditable. Moving value between buckets (e.g. locking an
// order) is two entries written atomically via CreateTx: an "out" on
// available and an "in" on locked.
type LedgerEntry struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	Asset            string
	Bucket           LedgerBucket
	Direction        LedgerDirection
	Amount           float64 // always positive; Direction determines sign
	Reason           LedgerReason
	OrderID          *uuid.UUID
	CreatedByAdminID *uuid.UUID
	Note             string
	CreatedAt        time.Time
}

type Balance struct {
	Asset     string
	Available float64
	Locked    float64
}

type LedgerRepository interface {
	Create(ctx context.Context, e *LedgerEntry) error
	CreateTx(ctx context.Context, entries []*LedgerEntry) error
	ListByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*LedgerEntry, error)
	BalancesByUser(ctx context.Context, userID uuid.UUID) ([]Balance, error)
}
