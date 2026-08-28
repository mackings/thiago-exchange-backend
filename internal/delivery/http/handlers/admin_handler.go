package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"thiagoexchange/backend/internal/infra/bybit"
	"thiagoexchange/backend/internal/usecase/admin"
)

type AdminHandler struct{ svc *admin.Service }

func NewAdminHandler(svc *admin.Service) *AdminHandler { return &AdminHandler{svc: svc} }

func (h *AdminHandler) ListUsers(c *gin.Context) {
	users, err := h.svc.ListUsers(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	out := make([]userDTO, 0, len(users))
	for _, u := range users {
		out = append(out, toUserDTO(u))
	}
	c.JSON(http.StatusOK, out)
}

type setDisabledRequest struct {
	Disabled bool `json:"disabled"`
}

func (h *AdminHandler) SetUserDisabled(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req setDisabledRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.SetDisabled(c.Request.Context(), id, req.Disabled); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// BybitBalance surfaces the platform's real Bybit account balance so ops can
// verify funds before releasing an order. Returns 503 with a clear message
// if BYBIT_API_KEY/SECRET haven't been configured yet.
func (h *AdminHandler) BybitBalance(c *gin.Context) {
	balances, err := h.svc.BybitBalance(c.Request.Context())
	if err != nil {
		if errors.Is(err, bybit.ErrNotConfigured) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Bybit API credentials are not configured on this server yet"})
			return
		}
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, balances)
}
