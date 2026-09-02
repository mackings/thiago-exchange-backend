package http

import (
	"github.com/gin-gonic/gin"

	"thiagoexchange/backend/internal/delivery/http/handlers"
	"thiagoexchange/backend/internal/delivery/http/middleware"
)

type Handlers struct {
	Auth    *handlers.AuthHandler
	Ads     *handlers.AdHandler
	Orders  *handlers.OrderHandler
	Wallet  *handlers.WalletHandler
	KYC     *handlers.KYCHandler
	Dispute *handlers.DisputeHandler
	Admin   *handlers.AdminHandler
	Chat    *handlers.ChatHandler
	Upload  *handlers.UploadHandler
}

func NewRouter(h Handlers, jwtSecret, allowedOrigin string) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger(), middleware.CORS(allowedOrigin))

	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })

	api := r.Group("/api/v1")

	auth := api.Group("/auth")
	auth.POST("/register", h.Auth.Register)
	auth.POST("/login", h.Auth.Login)
	auth.POST("/refresh", h.Auth.Refresh)
	auth.POST("/logout", h.Auth.Logout)
	auth.POST("/forgot-password", h.Auth.ForgotPassword)
	auth.POST("/reset-password", h.Auth.ResetPassword)
	auth.POST("/verify-email", h.Auth.VerifyEmail)
	auth.POST("/resend-verification", h.Auth.ResendVerification)

	// Public ad browsing — no auth required, matches how P2P marketplaces let
	// anyone see rates/offers before signing up.
	api.GET("/ads", h.Ads.List)
	api.GET("/ads/:id", h.Ads.Get)

	authed := api.Group("")
	authed.Use(middleware.RequireAuth(jwtSecret))
	authed.GET("/auth/me", h.Auth.Me)
	authed.PATCH("/auth/me", h.Auth.UpdateMe)
	{
		authed.POST("/orders", h.Orders.Create)
		authed.GET("/orders/mine", h.Orders.ListMine)
		authed.GET("/orders/:id", h.Orders.Get)
		authed.POST("/orders/:id/mark-paid", h.Orders.MarkPaid)
		authed.GET("/orders/:id/deposit-instructions", h.Orders.DepositInstructions)
		authed.GET("/orders/:id/payment-instructions", h.Orders.PaymentInstructions)
		authed.POST("/orders/:id/submit-deposit", h.Orders.SubmitDeposit)
		authed.POST("/orders/:id/confirm-payment", h.Orders.ConfirmPayment)
		authed.POST("/orders/:id/cancel", h.Orders.Cancel)

		authed.GET("/orders/:id/messages", h.Chat.History)
		authed.GET("/orders/:id/ws", h.Chat.Stream)

		authed.GET("/wallet/balances", h.Wallet.Balances)
		authed.GET("/wallet/history", h.Wallet.History)

		authed.POST("/kyc", h.KYC.Submit)
		authed.GET("/kyc/me", h.KYC.MyStatus)

		authed.POST("/disputes", h.Dispute.Raise)

		authed.POST("/uploads", h.Upload.Upload)
	}

	admin := api.Group("/admin")
	admin.Use(middleware.RequireAuth(jwtSecret), middleware.RequireAdmin())
	{
		admin.GET("/users", h.Admin.ListUsers)
		admin.PATCH("/users/:id/disabled", h.Admin.SetUserDisabled)
		admin.GET("/bybit/balance", h.Admin.BybitBalance)

		admin.POST("/ads", h.Ads.Create)
		admin.GET("/ads/mine", h.Ads.ListMine)
		admin.PATCH("/ads/:id", h.Ads.Update)

		admin.GET("/kyc/pending", h.KYC.ListPending)
		admin.POST("/kyc/:id/review", h.KYC.Review)

		admin.GET("/disputes", h.Dispute.ListOpen)
		admin.POST("/disputes/:id/resolve", h.Dispute.Resolve)

		admin.GET("/orders", h.Orders.AdminList)
		admin.GET("/orders/all", h.Orders.AdminListAll)
		admin.POST("/orders/:id/release", h.Orders.Release)
		admin.POST("/wallet/credit", h.Wallet.AdminCredit)

		admin.GET("/wallet-whitelist", h.Orders.ListWhitelist)
		admin.POST("/wallet-whitelist", h.Orders.MarkWhitelisted)

		admin.GET("/deposit-addresses", h.Orders.ListDepositAddresses)
		admin.POST("/deposit-addresses", h.Orders.SetDepositAddress)

		admin.GET("/notifications/ws", h.Chat.StreamNotifications)
	}

	return r
}
