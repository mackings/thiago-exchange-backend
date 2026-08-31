package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"thiagoexchange/backend/internal/delivery/http/middleware"
	"thiagoexchange/backend/internal/domain"
	"thiagoexchange/backend/internal/usecase/orders"
)

type OrderHandler struct {
	svc   *orders.Service
	users domain.UserRepository
}

func NewOrderHandler(svc *orders.Service, users domain.UserRepository) *OrderHandler {
	return &OrderHandler{svc: svc, users: users}
}

type orderDTO struct {
	ID              string  `json:"id"`
	AdID            string  `json:"adId"`
	Side            string  `json:"side"`
	BuyerID         string  `json:"buyerId"`
	SellerID        string  `json:"sellerId"`
	BuyerName       string  `json:"buyerName,omitempty"`
	SellerName      string  `json:"sellerName,omitempty"`
	Asset           string  `json:"asset"`
	Fiat            string  `json:"fiat"`
	Amount          float64 `json:"amount"`
	Rate            float64 `json:"rate"`
	FiatAmount      float64 `json:"fiatAmount"`
	Status          string  `json:"status"`
	PayoutAddress   string  `json:"payoutAddress,omitempty"`
	PayoutChain     string  `json:"payoutChain,omitempty"`
	DepositTxID     string  `json:"depositTxId,omitempty"`
	PaymentDeadline string  `json:"paymentDeadline"`
	PaymentProofURL string  `json:"paymentProofUrl"`
	CreatedAt       string  `json:"createdAt"`
}

// toOrderDTO looks up the buyer's and seller's display names — "who am I
// trading with" is what a trader actually wants to see, not the other
// party's wallet address or payout chain. nameCache is optional (nil is
// fine for a single order) and lets list endpoints avoid repeat lookups
// when the same merchant/trader appears across many orders.
func (h *OrderHandler) toOrderDTO(ctx context.Context, o *domain.Order, nameCache map[uuid.UUID]string) orderDTO {
	nameFor := func(id uuid.UUID) string {
		if nameCache != nil {
			if name, ok := nameCache[id]; ok {
				return name
			}
		}
		name := ""
		if u, err := h.users.GetByID(ctx, id); err == nil {
			name = u.FullName
		}
		if nameCache != nil {
			nameCache[id] = name
		}
		return name
	}
	return orderDTO{
		ID: o.ID.String(), AdID: o.AdID.String(), Side: string(o.Side),
		BuyerID: o.BuyerID.String(), SellerID: o.SellerID.String(),
		BuyerName: nameFor(o.BuyerID), SellerName: nameFor(o.SellerID),
		Asset: o.Asset, Fiat: o.Fiat, Amount: o.Amount, Rate: o.Rate, FiatAmount: o.FiatAmount,
		Status: string(o.Status), PayoutAddress: o.PayoutAddress, PayoutChain: o.PayoutChain,
		DepositTxID: o.DepositTxID, PaymentDeadline: o.PaymentDeadline.Format("2006-01-02T15:04:05Z07:00"),
		PaymentProofURL: o.PaymentProofURL, CreatedAt: o.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

type createOrderRequest struct {
	AdID          string  `json:"adId" binding:"required"`
	AssetAmount   float64 `json:"assetAmount" binding:"required,gt=0"`
	PayoutAddress string  `json:"payoutAddress"`
	PayoutChain   string  `json:"payoutChain"`
}

func (h *OrderHandler) Create(c *gin.Context) {
	var req createOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	adID, err := uuid.Parse(req.AdID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid adId"})
		return
	}
	order, err := h.svc.Create(c.Request.Context(), orders.CreateOrderInput{
		AdID: adID, TakerID: middleware.CurrentUserID(c), AssetAmount: req.AssetAmount,
		PayoutAddress: req.PayoutAddress, PayoutChain: req.PayoutChain,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, h.toOrderDTO(c.Request.Context(), order, nil))
}

// DepositInstructions returns Thiago's live Bybit deposit address for a
// buy-side order, so the seller (taker) knows where to send the asset.
func (h *OrderHandler) DepositInstructions(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	address, chain, tag, err := h.svc.DepositInstructions(c.Request.Context(), id, middleware.CurrentUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"address": address, "chain": chain, "tag": tag})
}

// PaymentInstructions returns the merchant admin's bank account for a
// sell-ad order, so the buyer sees a structured "pay to this account"
// instruction instead of relying on it being typed into chat.
func (h *OrderHandler) PaymentInstructions(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	bankName, accountNumber, accountName, err := h.svc.PaymentInstructions(c.Request.Context(), id, middleware.CurrentUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"bankName": bankName, "accountNumber": accountNumber, "accountName": accountName})
}

type submitDepositRequest struct {
	TxID string `json:"txId" binding:"required"`
}

// SubmitDeposit is the buy-side counterpart to MarkPaid: the seller reports
// the on-chain transaction hash instead of a fiat receipt, and it's
// verified against Bybit's real deposit records before the order advances.
func (h *OrderHandler) SubmitDeposit(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req submitDepositRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	order, err := h.svc.SubmitDeposit(c.Request.Context(), id, middleware.CurrentUserID(c), req.TxID)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.toOrderDTO(c.Request.Context(), order, nil))
}

func (h *OrderHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	order, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		respondError(c, err)
		return
	}
	userID := middleware.CurrentUserID(c)
	if order.BuyerID != userID && order.SellerID != userID && middleware.CurrentRole(c) != domain.RoleAdmin {
		respondError(c, domain.ErrForbidden)
		return
	}
	c.JSON(http.StatusOK, h.toOrderDTO(c.Request.Context(), order, nil))
}

func (h *OrderHandler) ListMine(c *gin.Context) {
	list, err := h.svc.ListMine(c.Request.Context(), middleware.CurrentUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	names := map[uuid.UUID]string{}
	out := make([]orderDTO, 0, len(list))
	for _, o := range list {
		out = append(out, h.toOrderDTO(c.Request.Context(), o, names))
	}
	c.JSON(http.StatusOK, out)
}

// AdminList is the admin verification queue: every order still in flight.
func (h *OrderHandler) AdminList(c *gin.Context) {
	list, err := h.svc.ListActionable(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	names := map[uuid.UUID]string{}
	out := make([]orderDTO, 0, len(list))
	for _, o := range list {
		out = append(out, h.toOrderDTO(c.Request.Context(), o, names))
	}
	c.JSON(http.StatusOK, out)
}

// AdminListAll is the admin dashboard's transactions table: every order
// regardless of status.
func (h *OrderHandler) AdminListAll(c *gin.Context) {
	list, err := h.svc.ListAllOrders(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	names := map[uuid.UUID]string{}
	out := make([]orderDTO, 0, len(list))
	for _, o := range list {
		out = append(out, h.toOrderDTO(c.Request.Context(), o, names))
	}
	c.JSON(http.StatusOK, out)
}

type markPaidRequest struct {
	ProofURL string `json:"proofUrl"`
}

func (h *OrderHandler) MarkPaid(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req markPaidRequest
	_ = c.ShouldBindJSON(&req)
	order, err := h.svc.MarkPaid(c.Request.Context(), id, middleware.CurrentUserID(c), req.ProofURL)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.toOrderDTO(c.Request.Context(), order, nil))
}

func (h *OrderHandler) ConfirmPayment(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	order, err := h.svc.ConfirmPayment(c.Request.Context(), id, middleware.CurrentUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.toOrderDTO(c.Request.Context(), order, nil))
}

// Release is admin-only: it's the point where ops confirms the real Bybit
// transfer happened before the ledger reflects it.
func (h *OrderHandler) Release(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	order, err := h.svc.Release(c.Request.Context(), id, middleware.CurrentUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.toOrderDTO(c.Request.Context(), order, nil))
}

func (h *OrderHandler) Cancel(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	order, err := h.svc.Cancel(c.Request.Context(), id, middleware.CurrentUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, h.toOrderDTO(c.Request.Context(), order, nil))
}

type whitelistedAddressDTO struct {
	ID             string `json:"id"`
	Address        string `json:"address"`
	Chain          string `json:"chain"`
	Asset          string `json:"asset"`
	AddedByAdminID string `json:"addedByAdminId"`
	CreatedAt      string `json:"createdAt"`
}

func toWhitelistedAddressDTO(w *domain.WhitelistedAddress) whitelistedAddressDTO {
	return whitelistedAddressDTO{
		ID: w.ID.String(), Address: w.Address, Chain: w.Chain, Asset: w.Asset,
		AddedByAdminID: w.AddedByAdminID.String(), CreatedAt: w.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

type markWhitelistedRequest struct {
	Address string `json:"address" binding:"required"`
	Chain   string `json:"chain"`
	Asset   string `json:"asset"`
}

// MarkWhitelisted records that admin has manually whitelisted address on
// Bybit's own site — this doesn't call Bybit itself (there's no API for
// that), it just tells our own release gate to stop blocking on it.
func (h *OrderHandler) MarkWhitelisted(c *gin.Context) {
	var req markWhitelistedRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	entry, err := h.svc.MarkAddressWhitelisted(c.Request.Context(), req.Address, req.Chain, req.Asset, middleware.CurrentUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toWhitelistedAddressDTO(entry))
}

func (h *OrderHandler) ListWhitelist(c *gin.Context) {
	list, err := h.svc.ListWhitelist(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	out := make([]whitelistedAddressDTO, 0, len(list))
	for _, w := range list {
		out = append(out, toWhitelistedAddressDTO(w))
	}
	c.JSON(http.StatusOK, out)
}

type depositAddressDTO struct {
	ID             string `json:"id"`
	Asset          string `json:"asset"`
	Chain          string `json:"chain"`
	Address        string `json:"address"`
	Tag            string `json:"tag,omitempty"`
	AddedByAdminID string `json:"addedByAdminId"`
	UpdatedAt      string `json:"updatedAt"`
}

func toDepositAddressDTO(d *domain.DepositAddress) depositAddressDTO {
	return depositAddressDTO{
		ID: d.ID.String(), Asset: d.Asset, Chain: d.Chain, Address: d.Address, Tag: d.Tag,
		AddedByAdminID: d.AddedByAdminID.String(), UpdatedAt: d.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

type setDepositAddressRequest struct {
	Asset   string `json:"asset" binding:"required"`
	Chain   string `json:"chain"`
	Address string `json:"address" binding:"required"`
	Tag     string `json:"tag"`
}

// SetDepositAddress lets admin configure where takers should send crypto
// for a given asset's buy ads — copied once from Bybit's own deposit page,
// since there's no private Bybit endpoint we can safely call for this
// without real API keys wired up.
func (h *OrderHandler) SetDepositAddress(c *gin.Context) {
	var req setDepositAddressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	entry, err := h.svc.SetDepositAddress(c.Request.Context(), req.Asset, req.Chain, req.Address, req.Tag, middleware.CurrentUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toDepositAddressDTO(entry))
}

func (h *OrderHandler) ListDepositAddresses(c *gin.Context) {
	list, err := h.svc.ListDepositAddresses(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	out := make([]depositAddressDTO, 0, len(list))
	for _, d := range list {
		out = append(out, toDepositAddressDTO(d))
	}
	c.JSON(http.StatusOK, out)
}
