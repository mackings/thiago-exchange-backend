package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"

	"github.com/google/uuid"

	"thiagoexchange/backend/internal/config"
	deliveryhttp "thiagoexchange/backend/internal/delivery/http"
	"thiagoexchange/backend/internal/delivery/http/handlers"
	"thiagoexchange/backend/internal/domain"
	"thiagoexchange/backend/internal/infra/bybit"
	"thiagoexchange/backend/internal/infra/storage"
	"thiagoexchange/backend/internal/infra/ws"
	"thiagoexchange/backend/internal/platform/db"
	"thiagoexchange/backend/internal/repository/mongodb"
	"thiagoexchange/backend/internal/usecase/admin"
	"thiagoexchange/backend/internal/usecase/ads"
	"thiagoexchange/backend/internal/usecase/auth"
	"thiagoexchange/backend/internal/usecase/chat"
	"thiagoexchange/backend/internal/usecase/dispute"
	"thiagoexchange/backend/internal/usecase/kyc"
	"thiagoexchange/backend/internal/usecase/orders"
	"thiagoexchange/backend/internal/usecase/wallet"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	mongoClient, mongoDB, err := db.Connect(cfg.MongoURI, cfg.MongoDBName)
	if err != nil {
		log.Fatalf("connect db: %v", err)
	}
	ctx := context.Background()
	if err := mongodb.EnsureUserIndexes(ctx, mongoDB); err != nil {
		log.Fatalf("ensure indexes: %v", err)
	}

	// Repositories
	userRepo := mongodb.NewUserRepo(mongoDB)
	kycRepo := mongodb.NewKYCRepo(mongoDB)
	adRepo := mongodb.NewAdRepo(mongoDB)
	orderRepo := mongodb.NewOrderRepo(mongoDB)
	messageRepo := mongodb.NewOrderMessageRepo(mongoDB)
	disputeRepo := mongodb.NewDisputeRepo(mongoDB)
	ledgerRepo := mongodb.NewLedgerRepo(mongoDB, mongoClient)

	// Infra
	bybitClient := bybit.NewClient(cfg.BybitBaseURL, cfg.BybitAPIKey, cfg.BybitAPISecret, map[string]float64{"NGN": ngnRate()})
	localStorage, err := storage.NewLocal(cfg.StorageDir, "/uploads")
	if err != nil {
		log.Fatalf("init storage: %v", err)
	}
	hub := ws.NewHub()

	// Usecases
	authSvc := auth.NewService(userRepo, cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	adsSvc := ads.NewService(adRepo, userRepo)
	ordersSvc := orders.NewService(orderRepo, adRepo, ledgerRepo, bybitClient, bybitClient)
	walletSvc := wallet.NewService(ledgerRepo)
	kycSvc := kyc.NewService(kycRepo, userRepo)
	disputeSvc := dispute.NewService(disputeRepo, ordersSvc)
	chatSvc := chat.NewService(orderRepo, messageRepo)
	adminSvc := admin.NewService(userRepo, bybitClient)

	seedAdmin(userRepo)

	// Handlers
	h := deliveryhttp.Handlers{
		Auth:    handlers.NewAuthHandler(authSvc, cfg.RefreshTokenTTL, isProd()),
		Ads:     handlers.NewAdHandler(adsSvc),
		Orders:  handlers.NewOrderHandler(ordersSvc),
		Wallet:  handlers.NewWalletHandler(walletSvc),
		KYC:     handlers.NewKYCHandler(kycSvc, localStorage),
		Dispute: handlers.NewDisputeHandler(disputeSvc),
		Admin:   handlers.NewAdminHandler(adminSvc),
		Chat:    handlers.NewChatHandler(chatSvc, hub, cfg.AllowedOrigin),
		Upload:  handlers.NewUploadHandler(localStorage),
	}

	router := deliveryhttp.NewRouter(h, cfg.JWTSecret, cfg.AllowedOrigin)
	router.Static("/uploads", cfg.StorageDir)

	log.Printf("thiago-exchange api listening on :%s", cfg.Port)
	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func isProd() bool {
	return os.Getenv("ENV") == "production"
}

func ngnRate() float64 {
	if v := os.Getenv("USD_NGN_RATE"); v != "" {
		if rate, err := strconv.ParseFloat(v, 64); err == nil && rate > 0 {
			return rate
		}
	}
	return 1550 // fallback reference rate, override via USD_NGN_RATE
}

// seedAdmin creates the first admin account from ADMIN_EMAIL/ADMIN_PASSWORD
// env vars if one doesn't already exist, so there's a way into the admin
// console without hand-editing the database.
func seedAdmin(users domain.UserRepository) {
	email := os.Getenv("ADMIN_EMAIL")
	password := os.Getenv("ADMIN_PASSWORD")
	if email == "" || password == "" {
		return
	}
	ctx := context.Background()
	if _, err := users.GetByEmail(ctx, email); err == nil {
		return // already exists
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("seed admin: %v", err)
		return
	}
	admin := &domain.User{
		ID: uuid.New(), Email: email, PasswordHash: string(hash), FullName: "Admin",
		Role: domain.RoleAdmin, KYCStatus: domain.KYCStatusVerified,
	}
	if err := users.Create(ctx, admin); err != nil {
		log.Printf("seed admin: %v", err)
		return
	}
	log.Printf("seeded admin account: %s", email)
}
