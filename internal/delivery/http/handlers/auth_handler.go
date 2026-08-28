package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"thiagoexchange/backend/internal/domain"
	"thiagoexchange/backend/internal/usecase/auth"
)

type AuthHandler struct {
	svc             *auth.Service
	refreshTokenTTL time.Duration
	secureCookies   bool
}

func NewAuthHandler(svc *auth.Service, refreshTokenTTL time.Duration, secureCookies bool) *AuthHandler {
	return &AuthHandler{svc: svc, refreshTokenTTL: refreshTokenTTL, secureCookies: secureCookies}
}

type registerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Phone    string `json:"phone"`
	Password string `json:"password" binding:"required,min=8"`
	FullName string `json:"fullName" binding:"required"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type authResponse struct {
	User        userDTO `json:"user"`
	AccessToken string  `json:"accessToken"`
}

type userDTO struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	FullName  string `json:"fullName"`
	Role      string `json:"role"`
	KYCStatus string `json:"kycStatus"`
}

func toUserDTO(u *domain.User) userDTO {
	return userDTO{
		ID: u.ID.String(), Email: u.Email, Phone: u.Phone,
		FullName: u.FullName, Role: string(u.Role), KYCStatus: string(u.KYCStatus),
	}
}

func (h *AuthHandler) setRefreshCookie(c *gin.Context, token string) {
	c.SetCookie("refresh_token", token, int(h.refreshTokenTTL.Seconds()), "/", "", h.secureCookies, true)
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, tokens, err := h.svc.Register(c.Request.Context(), req.Email, req.Phone, req.Password, req.FullName)
	if err != nil {
		respondError(c, err)
		return
	}
	h.setRefreshCookie(c, tokens.RefreshToken)
	c.JSON(http.StatusCreated, authResponse{User: toUserDTO(user), AccessToken: tokens.AccessToken})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, tokens, err := h.svc.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		respondError(c, err)
		return
	}
	h.setRefreshCookie(c, tokens.RefreshToken)
	c.JSON(http.StatusOK, authResponse{User: toUserDTO(user), AccessToken: tokens.AccessToken})
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	refreshToken, err := c.Cookie("refresh_token")
	if err != nil || refreshToken == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing refresh token"})
		return
	}
	tokens, err := h.svc.Refresh(c.Request.Context(), refreshToken)
	if err != nil {
		respondError(c, err)
		return
	}
	h.setRefreshCookie(c, tokens.RefreshToken)
	c.JSON(http.StatusOK, gin.H{"accessToken": tokens.AccessToken})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	c.SetCookie("refresh_token", "", -1, "/", "", h.secureCookies, true)
	c.Status(http.StatusNoContent)
}
