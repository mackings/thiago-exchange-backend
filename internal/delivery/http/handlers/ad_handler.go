package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"thiagoexchange/backend/internal/delivery/http/middleware"
	"thiagoexchange/backend/internal/domain"
	"thiagoexchange/backend/internal/usecase/ads"
)

type AdHandler struct{ svc *ads.Service }

func NewAdHandler(svc *ads.Service) *AdHandler { return &AdHandler{svc: svc} }

type adDTO struct {
	ID                string  `json:"id"`
	OwnerID           string  `json:"ownerId"`
	Side              string  `json:"side"`
	Asset             string  `json:"asset"`
	Fiat              string  `json:"fiat"`
	RateType          string  `json:"rateType"`
	FixedRate         float64 `json:"fixedRate"`
	FloatingMarginPct float64 `json:"floatingMarginPct"`
	MinLimit          float64 `json:"minLimit"`
	MaxLimit          float64 `json:"maxLimit"`
	AvailableAmount   float64 `json:"availableAmount"`
	PaymentMethods    string  `json:"paymentMethods"`
	Terms             string  `json:"terms"`
	Status            string  `json:"status"`
}

func toAdDTO(a *domain.Ad) adDTO {
	return adDTO{
		ID: a.ID.String(), OwnerID: a.OwnerID.String(), Side: string(a.Side), Asset: a.Asset, Fiat: a.Fiat,
		RateType: string(a.RateType), FixedRate: a.FixedRate, FloatingMarginPct: a.FloatingMarginPct,
		MinLimit: a.MinLimit, MaxLimit: a.MaxLimit, AvailableAmount: a.AvailableAmount,
		PaymentMethods: a.PaymentMethods, Terms: a.Terms, Status: string(a.Status),
	}
}

type createAdRequest struct {
	Side              string  `json:"side" binding:"required,oneof=buy sell"`
	Asset             string  `json:"asset" binding:"required"`
	Fiat              string  `json:"fiat"`
	RateType          string  `json:"rateType" binding:"required,oneof=fixed floating_margin"`
	FixedRate         float64 `json:"fixedRate"`
	FloatingMarginPct float64 `json:"floatingMarginPct"`
	MinLimit          float64 `json:"minLimit" binding:"required,gt=0"`
	MaxLimit          float64 `json:"maxLimit" binding:"required,gt=0"`
	AvailableAmount   float64 `json:"availableAmount" binding:"required,gt=0"`
	PaymentMethods    string  `json:"paymentMethods"`
	Terms             string  `json:"terms"`
}

func (h *AdHandler) Create(c *gin.Context) {
	var req createAdRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ad, err := h.svc.Create(c.Request.Context(), ads.CreateAdInput{
		OwnerID: middleware.CurrentUserID(c), Side: domain.AdSide(req.Side), Asset: req.Asset, Fiat: req.Fiat,
		RateType: domain.RateType(req.RateType), FixedRate: req.FixedRate, FloatingMarginPct: req.FloatingMarginPct,
		MinLimit: req.MinLimit, MaxLimit: req.MaxLimit, AvailableAmount: req.AvailableAmount,
		PaymentMethods: req.PaymentMethods, Terms: req.Terms,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toAdDTO(ad))
}

func (h *AdHandler) List(c *gin.Context) {
	var f domain.AdFilter
	if side := c.Query("side"); side != "" {
		s := domain.AdSide(side)
		f.Side = &s
	}
	f.Asset = c.Query("asset")
	f.Fiat = c.Query("fiat")

	list, err := h.svc.List(c.Request.Context(), f)
	if err != nil {
		respondError(c, err)
		return
	}
	out := make([]adDTO, 0, len(list))
	for _, a := range list {
		out = append(out, toAdDTO(a))
	}
	c.JSON(http.StatusOK, out)
}

func (h *AdHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	ad, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toAdDTO(ad))
}

func (h *AdHandler) ListMine(c *gin.Context) {
	list, err := h.svc.ListMine(c.Request.Context(), middleware.CurrentUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	out := make([]adDTO, 0, len(list))
	for _, a := range list {
		out = append(out, toAdDTO(a))
	}
	c.JSON(http.StatusOK, out)
}

type updateAdRequest struct {
	Status          *string  `json:"status"`
	AvailableAmount *float64 `json:"availableAmount"`
	FixedRate       *float64 `json:"fixedRate"`
	Terms           *string  `json:"terms"`
}

func (h *AdHandler) Update(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req updateAdRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	in := ads.UpdateAdInput{AvailableAmount: req.AvailableAmount, FixedRate: req.FixedRate, Terms: req.Terms}
	if req.Status != nil {
		s := domain.AdStatus(*req.Status)
		in.Status = &s
	}
	ad, err := h.svc.Update(c.Request.Context(), id, middleware.CurrentUserID(c), in)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toAdDTO(ad))
}
