package orders

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"thiagoexchange/backend/internal/domain"
)

const paymentWindow = 30 * time.Minute

// BybitTreasury is the slice of the Bybit client the order flow needs to
// actually move real money: pay a buyer out, or verify a seller's deposit
// landed. Defined here (consumer-side) rather than in domain, matching the
// same pattern as usecase/admin.BybitAccount.
type BybitTreasury interface {
	Withdraw(ctx context.Context, asset, chain, address string, amount float64) (withdrawID string, err error)
	VerifyDeposit(ctx context.Context, asset, txID string) (amount float64, confirmed bool, err error)
	DepositAddress(ctx context.Context, asset, chain string) (address, tag string, err error)
}

// Mailer is the narrow email-sending capability this usecase needs — see
// usecase/auth.Mailer for the same pattern.
type Mailer interface {
	Send(to, subject, body string) error
}

type Service struct {
	orders domain.OrderRepository
	ads    domain.AdRepository
	ledger domain.LedgerRepository
	prices domain.PriceFeed
	bybit  BybitTreasury
	users  domain.UserRepository
	mailer Mailer
}

func NewService(orders domain.OrderRepository, ads domain.AdRepository, ledger domain.LedgerRepository, prices domain.PriceFeed, bybit BybitTreasury, users domain.UserRepository, mailer Mailer) *Service {
	return &Service{orders: orders, ads: ads, ledger: ledger, prices: prices, bybit: bybit, users: users, mailer: mailer}
}

// notifyTaker emails whichever party on the order isn't Thiago's own
// merchant account — best-effort, run in a goroutine by callers so a slow
// or failing SMTP send never blocks the trading flow.
func (s *Service) notifyTaker(order *domain.Order, subject, body string) {
	takerID := order.BuyerID
	if takerID == order.MerchantID {
		takerID = order.SellerID
	}
	u, err := s.users.GetByID(context.Background(), takerID)
	if err != nil {
		return
	}
	_ = s.mailer.Send(u.Email, subject, body)
}

// resolveRate returns the fiat-per-unit rate to lock in for an order against
// the given ad. Fixed-rate ads use the rate as posted; floating-margin ads
// price off Bybit's public reference feed, with sell ads priced above market
// and buy ads priced below, matching how the margin is meant to read.
func (s *Service) resolveRate(ctx context.Context, ad *domain.Ad) (float64, error) {
	if ad.RateType == domain.RateTypeFixed {
		return ad.FixedRate, nil
	}
	ref, err := s.prices.ReferencePrice(ctx, ad.Asset, ad.Fiat)
	if err != nil {
		return 0, err
	}
	margin := ad.FloatingMarginPct / 100
	if ad.Side == domain.AdSideSell {
		return ref * (1 + margin), nil
	}
	return ref * (1 - margin), nil
}

type CreateOrderInput struct {
	AdID        uuid.UUID
	TakerID     uuid.UUID
	AssetAmount float64

	// Required when the ad is a sell ad (Thiago selling, taker receiving a
	// real payout) — where the withdrawal goes on release. Ignored for buy
	// ads, where the taker sends crypto to Thiago's own deposit address
	// instead (see DepositInstructions / SubmitDeposit).
	PayoutAddress string
	PayoutChain   string
}

func (s *Service) Create(ctx context.Context, in CreateOrderInput) (*domain.Order, error) {
	ad, err := s.ads.GetByID(ctx, in.AdID)
	if err != nil {
		return nil, err
	}
	if ad.Status != domain.AdStatusActive {
		return nil, domain.ErrAdUnavailable
	}
	if ad.OwnerID == in.TakerID {
		return nil, domain.ErrInvalidInput
	}
	if in.AssetAmount <= 0 || in.AssetAmount > ad.AvailableAmount {
		return nil, domain.ErrInvalidInput
	}

	rate, err := s.resolveRate(ctx, ad)
	if err != nil {
		return nil, err
	}
	fiatAmount := in.AssetAmount * rate
	if fiatAmount < ad.MinLimit || fiatAmount > ad.MaxLimit {
		return nil, domain.ErrInvalidInput
	}

	merchantID := ad.OwnerID
	var buyerID, sellerID uuid.UUID
	if ad.Side == domain.AdSideSell {
		sellerID, buyerID = ad.OwnerID, in.TakerID
	} else {
		buyerID, sellerID = ad.OwnerID, in.TakerID
	}

	order := &domain.Order{
		ID: uuid.New(), AdID: ad.ID, MerchantID: merchantID, Side: ad.Side,
		BuyerID: buyerID, SellerID: sellerID, Asset: ad.Asset, Fiat: ad.Fiat,
		Amount: in.AssetAmount, Rate: rate, FiatAmount: fiatAmount,
		Status: domain.OrderStatusAwaitingPayment, PaymentDeadline: time.Now().Add(paymentWindow),
	}

	if ad.Side == domain.AdSideSell {
		// Thiago is selling: the taker needs a real payout, so we need
		// somewhere to send it, and we need to know Thiago actually has
		// this much of the asset allocated (credited via admin funding)
		// before locking it against this order.
		if in.PayoutAddress == "" || in.PayoutChain == "" {
			return nil, domain.ErrInvalidInput
		}
		balances, err := s.ledger.BalancesByUser(ctx, merchantID)
		if err != nil {
			return nil, err
		}
		var merchantAvailable float64
		for _, b := range balances {
			if b.Asset == ad.Asset {
				merchantAvailable = b.Available
				break
			}
		}
		if merchantAvailable < in.AssetAmount {
			return nil, domain.ErrInsufficientBalance
		}
		order.PayoutAddress = in.PayoutAddress
		order.PayoutChain = in.PayoutChain
	}

	if err := s.orders.Create(ctx, order); err != nil {
		return nil, err
	}

	ad.AvailableAmount -= in.AssetAmount
	if ad.AvailableAmount <= 0 {
		ad.Status = domain.AdStatusPaused
	}
	if err := s.ads.Update(ctx, ad); err != nil {
		return nil, err
	}

	if ad.Side == domain.AdSideSell {
		lockEntries := []*domain.LedgerEntry{
			{ID: uuid.New(), UserID: merchantID, Asset: ad.Asset, Bucket: domain.BucketAvailable, Direction: domain.DirectionOut, Amount: in.AssetAmount, Reason: domain.ReasonOrderLock, OrderID: &order.ID},
			{ID: uuid.New(), UserID: merchantID, Asset: ad.Asset, Bucket: domain.BucketLocked, Direction: domain.DirectionIn, Amount: in.AssetAmount, Reason: domain.ReasonOrderLock, OrderID: &order.ID},
		}
		if err := s.ledger.CreateTx(ctx, lockEntries); err != nil {
			return nil, err
		}
	}

	go s.notifyTaker(order, "Order opened on Thiago Exchange",
		fmt.Sprintf("Your order for %v %s (%v %s) is open.\n\nTrack it in the app under Orders.\n\n— Thiago Exchange",
			order.Amount, order.Asset, order.FiatAmount, order.Fiat))

	return order, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	return s.orders.GetByID(ctx, id)
}

func (s *Service) ListMine(ctx context.Context, userID uuid.UUID) ([]*domain.Order, error) {
	return s.orders.ListByUser(ctx, userID, 0, 0)
}

// DepositInstructions returns Thiago's live Bybit deposit address for a
// buy-side order, so the taker (seller) knows where to send the asset.
func (s *Service) DepositInstructions(ctx context.Context, orderID, sellerID uuid.UUID) (address, chain, tag string, err error) {
	order, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return "", "", "", err
	}
	if order.Side != domain.AdSideBuy || order.SellerID != sellerID {
		return "", "", "", domain.ErrForbidden
	}
	address, tag, err = s.bybit.DepositAddress(ctx, order.Asset, "")
	return address, "", tag, err
}

// MarkPaid is the sell-side flow: the buyer submits proof they paid the
// fiat amount off-platform.
func (s *Service) MarkPaid(ctx context.Context, orderID, buyerID uuid.UUID, proofURL string) (*domain.Order, error) {
	order, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.Side != domain.AdSideSell {
		return nil, domain.ErrInvalidOrderState
	}
	if order.BuyerID != buyerID {
		return nil, domain.ErrForbidden
	}
	if !domain.CanTransition(order.Status, domain.OrderStatusPaymentMarked) {
		return nil, domain.ErrInvalidOrderState
	}
	order.Status = domain.OrderStatusPaymentMarked
	order.PaymentProofURL = proofURL
	if err := s.orders.Update(ctx, order); err != nil {
		return nil, err
	}
	return order, nil
}

// SubmitDeposit is the buy-side flow: the taker (seller) reports the
// transaction hash of the crypto they sent to Thiago's Bybit deposit
// address. It's verified against Bybit's real deposit records before the
// order advances — unlike a fiat receipt screenshot, this can't be faked.
// A nil error with domain.ErrDepositNotFound means it's not confirmed on
// Bybit's side yet (still in flight), not that it failed outright.
func (s *Service) SubmitDeposit(ctx context.Context, orderID, sellerID uuid.UUID, txID string) (*domain.Order, error) {
	order, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.Side != domain.AdSideBuy {
		return nil, domain.ErrInvalidOrderState
	}
	if order.SellerID != sellerID {
		return nil, domain.ErrForbidden
	}
	if order.Status != domain.OrderStatusAwaitingPayment {
		return nil, domain.ErrInvalidOrderState
	}
	if txID == "" {
		return nil, domain.ErrInvalidInput
	}

	amount, confirmed, err := s.bybit.VerifyDeposit(ctx, order.Asset, txID)
	if err != nil {
		return nil, fmt.Errorf("verify deposit: %w", err)
	}
	if !confirmed {
		return nil, domain.ErrDepositNotFound
	}

	order.DepositTxID = txID
	order.DepositAmount = amount
	// Bybit has already proven the deposit landed, so this skips straight
	// through payment_marked to payment_confirmed — no human confirmation
	// step needed for something the chain itself already confirmed.
	order.Status = domain.OrderStatusPaymentMarked
	if err := s.orders.Update(ctx, order); err != nil {
		return nil, err
	}
	order.Status = domain.OrderStatusPaymentConfirmed
	if err := s.orders.Update(ctx, order); err != nil {
		return nil, err
	}
	return order, nil
}

// ConfirmPayment is the sell-side flow: Thiago (the merchant/seller)
// confirms the buyer's fiat landed in Thiago's bank account.
func (s *Service) ConfirmPayment(ctx context.Context, orderID, sellerID uuid.UUID) (*domain.Order, error) {
	order, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.Side != domain.AdSideSell {
		return nil, domain.ErrInvalidOrderState
	}
	if order.SellerID != sellerID {
		return nil, domain.ErrForbidden
	}
	if !domain.CanTransition(order.Status, domain.OrderStatusPaymentConfirmed) {
		return nil, domain.ErrInvalidOrderState
	}
	order.Status = domain.OrderStatusPaymentConfirmed
	if err := s.orders.Update(ctx, order); err != nil {
		return nil, err
	}
	return order, nil
}

// completeRelease performs the side-aware settlement: a real Bybit
// withdrawal to the buyer for sell-side orders (only committing the ledger
// debit if the withdrawal actually succeeds), or crediting the merchant's
// available balance with the verified deposit for buy-side orders.
func (s *Service) completeRelease(ctx context.Context, order *domain.Order, adminID uuid.UUID) error {
	if order.Side == domain.AdSideSell {
		if _, err := s.bybit.Withdraw(ctx, order.Asset, order.PayoutChain, order.PayoutAddress, order.Amount); err != nil {
			return fmt.Errorf("bybit withdrawal failed, ledger not touched — likely needs the address whitelisted on Bybit first: %w", err)
		}
		entry := &domain.LedgerEntry{
			ID: uuid.New(), UserID: order.MerchantID, Asset: order.Asset, Bucket: domain.BucketLocked,
			Direction: domain.DirectionOut, Amount: order.Amount, Reason: domain.ReasonOrderRelease,
			OrderID: &order.ID, CreatedByAdminID: &adminID,
		}
		if err := s.ledger.Create(ctx, entry); err != nil {
			return err
		}
	} else {
		entry := &domain.LedgerEntry{
			ID: uuid.New(), UserID: order.MerchantID, Asset: order.Asset, Bucket: domain.BucketAvailable,
			Direction: domain.DirectionIn, Amount: order.DepositAmount, Reason: domain.ReasonDepositCredited,
			OrderID: &order.ID, CreatedByAdminID: &adminID,
		}
		if err := s.ledger.Create(ctx, entry); err != nil {
			return err
		}
	}
	order.Status = domain.OrderStatusCompleted
	if err := s.orders.Update(ctx, order); err != nil {
		return err
	}

	go s.notifyTaker(order, "Thiago Exchange order complete",
		fmt.Sprintf("Your order for %v %s (%v %s) is complete.\n\nThanks for trading with Thiago Exchange.\n\n— Thiago Exchange",
			order.Amount, order.Asset, order.FiatAmount, order.Fiat))

	return nil
}

// Release settles the order: for a sell ad this fires the real Bybit
// withdrawal to the buyer (admin-triggered, per the platform's custody
// model — ops confirms before real funds move); for a buy ad it credits
// Thiago's own ledger with the crypto the seller already sent, making it
// usable inventory for future sell ads. Admin still separately handles the
// off-platform fiat side (paying the buyer's bank, or paying the seller).
func (s *Service) Release(ctx context.Context, orderID uuid.UUID, adminID uuid.UUID) (*domain.Order, error) {
	order, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if !domain.CanTransition(order.Status, domain.OrderStatusReleased) {
		return nil, domain.ErrInvalidOrderState
	}
	if err := s.completeRelease(ctx, order, adminID); err != nil {
		return nil, err
	}
	return order, nil
}

// MarkDisputed moves an order into the disputed branch. Called by the
// dispute usecase when a party raises a dispute.
func (s *Service) MarkDisputed(ctx context.Context, orderID uuid.UUID) (*domain.Order, error) {
	order, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if !domain.IsDisputable(order.Status) {
		return nil, domain.ErrInvalidOrderState
	}
	order.Status = domain.OrderStatusDisputed
	if err := s.orders.Update(ctx, order); err != nil {
		return nil, err
	}
	return order, nil
}

// ReleaseFromDispute resolves a dispute the same way a normal release
// would (see completeRelease) — used when admin decides the trade should
// go through despite the dispute.
func (s *Service) ReleaseFromDispute(ctx context.Context, orderID, adminID uuid.UUID) (*domain.Order, error) {
	order, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.Status != domain.OrderStatusDisputed {
		return nil, domain.ErrInvalidOrderState
	}
	if err := s.completeRelease(ctx, order, adminID); err != nil {
		return nil, err
	}
	return order, nil
}

// RefundFromDispute unwinds the order back to Thiago (sell-side: unlocks
// the merchant's held crypto back to available; buy-side: there was never
// a lock, so this is a no-op on the ledger) — used when admin decides the
// trade should not complete.
func (s *Service) RefundFromDispute(ctx context.Context, orderID, adminID uuid.UUID) (*domain.Order, error) {
	order, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.Status != domain.OrderStatusDisputed {
		return nil, domain.ErrInvalidOrderState
	}
	if order.Side == domain.AdSideSell {
		entries := []*domain.LedgerEntry{
			{ID: uuid.New(), UserID: order.MerchantID, Asset: order.Asset, Bucket: domain.BucketLocked, Direction: domain.DirectionOut, Amount: order.Amount, Reason: domain.ReasonOrderDispute, OrderID: &order.ID, CreatedByAdminID: &adminID},
			{ID: uuid.New(), UserID: order.MerchantID, Asset: order.Asset, Bucket: domain.BucketAvailable, Direction: domain.DirectionIn, Amount: order.Amount, Reason: domain.ReasonOrderDispute, OrderID: &order.ID, CreatedByAdminID: &adminID},
		}
		if err := s.ledger.CreateTx(ctx, entries); err != nil {
			return nil, err
		}
	}
	s.restoreAdCapacity(ctx, order)
	order.Status = domain.OrderStatusCancelled
	if err := s.orders.Update(ctx, order); err != nil {
		return nil, err
	}
	return order, nil
}

func (s *Service) restoreAdCapacity(ctx context.Context, order *domain.Order) {
	ad, err := s.ads.GetByID(ctx, order.AdID)
	if err != nil {
		return
	}
	ad.AvailableAmount += order.Amount
	if ad.Status == domain.AdStatusPaused {
		ad.Status = domain.AdStatusActive
	}
	_ = s.ads.Update(ctx, ad)
}

func (s *Service) Cancel(ctx context.Context, orderID, actorID uuid.UUID) (*domain.Order, error) {
	order, err := s.orders.GetByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.BuyerID != actorID && order.SellerID != actorID {
		return nil, domain.ErrForbidden
	}
	if !domain.IsCancellable(order.Status) {
		return nil, domain.ErrInvalidOrderState
	}

	if order.Side == domain.AdSideSell {
		unlockEntries := []*domain.LedgerEntry{
			{ID: uuid.New(), UserID: order.MerchantID, Asset: order.Asset, Bucket: domain.BucketLocked, Direction: domain.DirectionOut, Amount: order.Amount, Reason: domain.ReasonOrderCancel, OrderID: &order.ID},
			{ID: uuid.New(), UserID: order.MerchantID, Asset: order.Asset, Bucket: domain.BucketAvailable, Direction: domain.DirectionIn, Amount: order.Amount, Reason: domain.ReasonOrderCancel, OrderID: &order.ID},
		}
		if err := s.ledger.CreateTx(ctx, unlockEntries); err != nil {
			return nil, err
		}
	}

	s.restoreAdCapacity(ctx, order)

	order.Status = domain.OrderStatusCancelled
	if err := s.orders.Update(ctx, order); err != nil {
		return nil, err
	}
	return order, nil
}
