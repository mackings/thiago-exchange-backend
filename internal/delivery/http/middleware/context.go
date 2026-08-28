package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"thiagoexchange/backend/internal/domain"
)

func CurrentUserID(c *gin.Context) uuid.UUID {
	v, _ := c.Get(ctxUserID)
	id, _ := v.(uuid.UUID)
	return id
}

func CurrentRole(c *gin.Context) domain.Role {
	v, _ := c.Get(ctxRole)
	role, _ := v.(domain.Role)
	return role
}
