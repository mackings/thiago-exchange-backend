package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"thiagoexchange/backend/internal/delivery/http/middleware"
	"thiagoexchange/backend/internal/domain"
	"thiagoexchange/backend/internal/infra/storage"
	"thiagoexchange/backend/internal/usecase/kyc"
)

type KYCHandler struct {
	svc     *kyc.Service
	storage storage.Storage
}

func NewKYCHandler(svc *kyc.Service, storage storage.Storage) *KYCHandler {
	return &KYCHandler{svc: svc, storage: storage}
}

type kycDTO struct {
	ID          string `json:"id"`
	FullName    string `json:"fullName"`
	IDType      string `json:"idType"`
	IDNumber    string `json:"idNumber"`
	DocumentURL string `json:"documentUrl"`
	Status      string `json:"status"`
	ReviewNote  string `json:"reviewNote,omitempty"`
}

func toKYCDTO(k *domain.KYCSubmission) kycDTO {
	return kycDTO{
		ID: k.ID.String(), FullName: k.FullName, IDType: k.IDType, IDNumber: k.IDNumber,
		DocumentURL: k.DocumentURL, Status: string(k.Status), ReviewNote: k.ReviewNote,
	}
}

// Submit accepts a multipart form: fullName, idType, idNumber, document (file).
func (h *KYCHandler) Submit(c *gin.Context) {
	file, header, err := c.Request.FormFile("document")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "document file is required"})
		return
	}
	defer file.Close()
	docURL, err := h.storage.Save(file, header)
	if err != nil {
		respondError(c, err)
		return
	}

	sub, err := h.svc.Submit(c.Request.Context(), kyc.SubmitInput{
		UserID: middleware.CurrentUserID(c), FullName: c.PostForm("fullName"),
		IDType: c.PostForm("idType"), IDNumber: c.PostForm("idNumber"), DocumentURL: docURL,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toKYCDTO(sub))
}

func (h *KYCHandler) MyStatus(c *gin.Context) {
	sub, err := h.svc.MyStatus(c.Request.Context(), middleware.CurrentUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toKYCDTO(sub))
}

func (h *KYCHandler) ListPending(c *gin.Context) {
	list, err := h.svc.ListPending(c.Request.Context())
	if err != nil {
		respondError(c, err)
		return
	}
	out := make([]kycDTO, 0, len(list))
	for _, k := range list {
		out = append(out, toKYCDTO(k))
	}
	c.JSON(http.StatusOK, out)
}

type reviewKYCRequest struct {
	Approve bool   `json:"approve"`
	Note    string `json:"note"`
}

func (h *KYCHandler) Review(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req reviewKYCRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	sub, err := h.svc.Review(c.Request.Context(), id, middleware.CurrentUserID(c), req.Approve, req.Note)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toKYCDTO(sub))
}
