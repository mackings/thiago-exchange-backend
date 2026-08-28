package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"thiagoexchange/backend/internal/domain"
)

// respondError maps a domain error to the appropriate HTTP status. Unknown
// errors (infra failures, etc.) fall back to 500 without leaking details.
func respondError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, domain.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, domain.ErrAlreadyExists):
		status = http.StatusConflict
	case errors.Is(err, domain.ErrInvalidCredentials):
		status = http.StatusUnauthorized
	case errors.Is(err, domain.ErrUnauthorized):
		status = http.StatusUnauthorized
	case errors.Is(err, domain.ErrForbidden):
		status = http.StatusForbidden
	case errors.Is(err, domain.ErrInvalidInput):
		status = http.StatusBadRequest
	case errors.Is(err, domain.ErrInsufficientBalance):
		status = http.StatusBadRequest
	case errors.Is(err, domain.ErrInvalidOrderState):
		status = http.StatusConflict
	case errors.Is(err, domain.ErrAdUnavailable):
		status = http.StatusConflict
	case errors.Is(err, domain.ErrDepositNotFound):
		status = http.StatusNotFound
	}
	c.JSON(status, gin.H{"error": err.Error()})
}
