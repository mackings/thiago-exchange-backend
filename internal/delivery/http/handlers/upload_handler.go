package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"thiagoexchange/backend/internal/infra/storage"
)

// UploadHandler is a generic authenticated file upload used for payment
// proof screenshots in the trade room (KYC documents go through their own
// endpoint since that flow also updates KYC status).
type UploadHandler struct{ storage storage.Storage }

func NewUploadHandler(storage storage.Storage) *UploadHandler {
	return &UploadHandler{storage: storage}
}

func (h *UploadHandler) Upload(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	defer file.Close()
	url, err := h.storage.Save(file, header)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"url": url})
}
