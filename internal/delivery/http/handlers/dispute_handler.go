package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"thiagoexchange/backend/internal/delivery/http/middleware"
	"thiagoexchange/backend/internal/domain"
	"thiagoexchange/backend/internal/usecase/dispute"
)

type DisputeHandler struct{ svc *dispute.Service }

func NewDisputeHandler(svc *dispute.Service) *DisputeHandler { return &DisputeHandler{svc: svc} }

type disputeDTO struct {
	ID         string `json:"id"`
	OrderID    string `json:"orderId"`
	RaisedBy   string `json:"raisedBy"`
	Reason     string `json:"reason"`
	Status     string `json:"status"`
	Resolution string `json:"resolution,omitempty"`
}

func toDisputeDTO(d *domain.Dispute) disputeDTO {
	return disputeDTO{
		ID: d.ID.String(), OrderID: d.OrderID.String(), RaisedBy: d.RaisedBy.String(),
		Reason: d.Reason, Status: string(d.Status), Resolution: string(d.Resolution),
	}
}

type raiseDisputeRequest struct {
	OrderID string `json:"orderId" binding:"required"`
	Reason  string `json:"reason" binding:"required"`
}

func (h *DisputeHandler) Raise(c *gin.Context) {
	var req raiseDisputeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	orderID, err := uuid.Parse(req.OrderID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid orderId"})
		return
	}
	d, err := h.svc.Raise(c.Request.Context(), orderID, middleware.CurrentUserID(c), req.Reason)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toDisputeDTO(d))
}

func (h *DisputeHandler) ListOpen(c *gin.Context) {
	list, err := h.svc.ListOpen(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	out := make([]disputeDTO, 0, len(list))
	for _, d := range list {
		out = append(out, toDisputeDTO(d))
	}
	c.JSON(http.StatusOK, out)
}

type resolveDisputeRequest struct {
	Resolution string `json:"resolution" binding:"required,oneof=release_to_buyer refund_to_seller"`
}

func (h *DisputeHandler) Resolve(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req resolveDisputeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	d, err := h.svc.Resolve(c.Request.Context(), id, middleware.CurrentUserID(c), domain.DisputeResolution(req.Resolution))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toDisputeDTO(d))
}
