package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"thiagoexchange/backend/internal/delivery/http/middleware"
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
	ID                string `json:"id"`
	Email             string `json:"email"`
	Phone             string `json:"phone"`
	FullName          string `json:"fullName"`
	Role              string `json:"role"`
	KYCStatus         string `json:"kycStatus"`
	Disabled          bool   `json:"disabled"`
	EmailVerified     bool   `json:"emailVerified"`
	CreatedAt         string `json:"createdAt"`
	BankName          string `json:"bankName,omitempty"`
	BankAccountNumber string `json:"bankAccountNumber,omitempty"`
	BankAccountName   string `json:"bankAccountName,omitempty"`
}

func toUserDTO(u *domain.User) userDTO {
	return userDTO{
		ID: u.ID.String(), Email: u.Email, Phone: u.Phone,
		FullName: u.FullName, Role: string(u.Role), KYCStatus: string(u.KYCStatus),
		Disabled: u.Disabled, EmailVerified: u.EmailVerified, CreatedAt: u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		BankName: u.BankName, BankAccountNumber: u.BankAccountNumber, BankAccountName: u.BankAccountName,
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

func (h *AuthHandler) Me(c *gin.Context) {
	user, err := h.svc.Me(c.Request.Context(), middleware.CurrentUserID(c))
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toUserDTO(user))
}

type updateProfileRequest struct {
	FullName          string `json:"fullName"`
	Phone             string `json:"phone"`
	BankName          string `json:"bankName"`
	BankAccountNumber string `json:"bankAccountNumber"`
	BankAccountName   string `json:"bankAccountName"`
}

func (h *AuthHandler) UpdateMe(c *gin.Context) {
	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := h.svc.UpdateProfile(c.Request.Context(), middleware.CurrentUserID(c), auth.UpdateProfileInput{
		FullName: req.FullName, Phone: req.Phone,
		BankName: req.BankName, BankAccountNumber: req.BankAccountNumber, BankAccountName: req.BankAccountName,
	})
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, toUserDTO(user))
}

type forgotPasswordRequest struct {
	Email string `json:"email" binding:"required,email"`
}

func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req forgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Always 204, regardless of whether the email exists — see
	// RequestPasswordReset's comment on why.
	_ = h.svc.RequestPasswordReset(c.Request.Context(), req.Email)
	c.Status(http.StatusNoContent)
}

type resetPasswordRequest struct {
	Token    string `json:"token" binding:"required"`
	Password string `json:"password" binding:"required,min=8"`
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req resetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.ResetPassword(c.Request.Context(), req.Token, req.Password); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type verifyEmailRequest struct {
	Token string `json:"token" binding:"required"`
}

func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	var req verifyEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.VerifyEmail(c.Request.Context(), req.Token); err != nil {
		respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

type resendVerificationRequest struct {
	Email string `json:"email" binding:"required,email"`
}

func (h *AuthHandler) ResendVerification(c *gin.Context) {
	var req resendVerificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Always 204 — see ResendVerification's comment on why.
	_ = h.svc.ResendVerification(c.Request.Context(), req.Email)
	c.Status(http.StatusNoContent)
}
