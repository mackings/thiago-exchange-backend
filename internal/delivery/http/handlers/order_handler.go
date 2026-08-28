package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"thiagoexchange/backend/internal/delivery/http/middleware"
	"thiagoexchange/backend/internal/domain"
	"thiagoexchange/backend/internal/usecase/orders"
)

type OrderHandler struct{ svc *orders.Service }

func NewOrderHandler(svc *orders.Service) *OrderHandler { return &OrderHandler{svc: svc} }

type orderDTO struct {
	ID              string  `json:"id"`
	AdID            string  `json:"adId"`
	Side            string  `json:"side"`
	BuyerID         string  `json:"buyerId"`
	SellerID        string  `json:"sellerId"`
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

func toOrderDTO(o *domain.Order) orderDTO {
	return orderDTO{
		ID: o.ID.String(), AdID: o.AdID.String(), Side: string(o.Side),
		BuyerID: o.BuyerID.String(), SellerID: o.SellerID.String(),
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
	c.JSON(http.StatusCreated, toOrderDTO(order))
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
	c.JSON(http.StatusOK, toOrderDTO(order))
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
	c.JSON(http.StatusOK, toOrderDTO(order))
}

func (h *OrderHandler) ListMine(c *gin.Context) {
	list, err := h.svc.ListMine(c.Request.Context(), middleware.CurrentUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	out := make([]orderDTO, 0, len(list))
	for _, o := range list {
		out = append(out, toOrderDTO(o))
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
	c.JSON(http.StatusOK, toOrderDTO(order))
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
	c.JSON(http.StatusOK, toOrderDTO(order))
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
	c.JSON(http.StatusOK, toOrderDTO(order))
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
	c.JSON(http.StatusOK, toOrderDTO(order))
}
