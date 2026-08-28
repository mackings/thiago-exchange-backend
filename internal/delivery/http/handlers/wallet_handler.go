package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"thiagoexchange/backend/internal/delivery/http/middleware"
	"thiagoexchange/backend/internal/domain"
	"thiagoexchange/backend/internal/usecase/wallet"
)

type WalletHandler struct{ svc *wallet.Service }

func NewWalletHandler(svc *wallet.Service) *WalletHandler { return &WalletHandler{svc: svc} }

type balanceDTO struct {
	Asset     string  `json:"asset"`
	Available float64 `json:"available"`
	Locked    float64 `json:"locked"`
}

func toBalanceDTO(b domain.Balance) balanceDTO {
	return balanceDTO{Asset: b.Asset, Available: b.Available, Locked: b.Locked}
}

func (h *WalletHandler) Balances(c *gin.Context) {
	balances, err := h.svc.Balances(c.Request.Context(), middleware.CurrentUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	out := make([]balanceDTO, 0, len(balances))
	for _, b := range balances {
		out = append(out, toBalanceDTO(b))
	}
	c.JSON(http.StatusOK, out)
}

type ledgerEntryDTO struct {
	ID        string  `json:"id"`
	Asset     string  `json:"asset"`
	Bucket    string  `json:"bucket"`
	Direction string  `json:"direction"`
	Amount    float64 `json:"amount"`
	Reason    string  `json:"reason"`
	OrderID   string  `json:"orderId,omitempty"`
	Note      string  `json:"note,omitempty"`
	CreatedAt string  `json:"createdAt"`
}

func (h *WalletHandler) History(c *gin.Context) {
	entries, err := h.svc.History(c.Request.Context(), middleware.CurrentUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	out := make([]ledgerEntryDTO, 0, len(entries))
	for _, e := range entries {
		dto := ledgerEntryDTO{
			ID: e.ID.String(), Asset: e.Asset, Bucket: string(e.Bucket), Direction: string(e.Direction),
			Amount: e.Amount, Reason: string(e.Reason), Note: e.Note,
			CreatedAt: e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		if e.OrderID != nil {
			dto.OrderID = e.OrderID.String()
		}
		out = append(out, dto)
	}
	c.JSON(http.StatusOK, out)
}

type adminCreditRequest struct {
	UserID string  `json:"userId" binding:"required"`
	Asset  string  `json:"asset" binding:"required"`
	Amount float64 `json:"amount" binding:"required,gt=0"`
	Note   string  `json:"note"`
}

func (h *WalletHandler) AdminCredit(c *gin.Context) {
	var req adminCreditRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	userID, err := uuid.Parse(req.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid userId"})
		return
	}
	if err := h.svc.AdminCredit(c.Request.Context(), userID, middleware.CurrentUserID(c), req.Asset, req.Amount, req.Note); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
