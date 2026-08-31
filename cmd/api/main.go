package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"

	"thiagoexchange/backend/internal/config"
	deliveryhttp "thiagoexchange/backend/internal/delivery/http"
	"thiagoexchange/backend/internal/delivery/http/handlers"
	"thiagoexchange/backend/internal/infra/bybit"
	"thiagoexchange/backend/internal/infra/mailer"
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
	resetRepo := mongodb.NewPasswordResetRepo(mongoDB)
	whitelistRepo := mongodb.NewWhitelistRepo(mongoDB)
	depositAddressRepo := mongodb.NewDepositAddressRepo(mongoDB)

	// Infra
	bybitClient := bybit.NewClient(cfg.BybitBaseURL, cfg.BybitAPIKey, cfg.BybitAPISecret, map[string]float64{"NGN": ngnRate()})
	localStorage, err := storage.NewLocal(cfg.StorageDir, "/uploads")
	if err != nil {
		log.Fatalf("init storage: %v", err)
	}
	hub := ws.NewHub()
	mailSvc := mailer.New(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.SMTPFromName)
	if !mailSvc.Configured() {
		log.Println("SMTP not configured — transactional emails are disabled")
	}

	// Usecases
	authSvc := auth.NewService(userRepo, resetRepo, cfg.JWTSecret, cfg.AccessTokenTTL, cfg.RefreshTokenTTL, cfg.AdminEmails, mailSvc, cfg.AllowedOrigin)
	adsSvc := ads.NewService(adRepo, userRepo)
	ordersSvc := orders.NewService(orderRepo, adRepo, ledgerRepo, bybitClient, bybitClient, userRepo, mailSvc, whitelistRepo, depositAddressRepo)
	walletSvc := wallet.NewService(ledgerRepo)
	kycSvc := kyc.NewService(kycRepo, userRepo)
	disputeSvc := dispute.NewService(disputeRepo, ordersSvc)
	chatSvc := chat.NewService(orderRepo, messageRepo)
	adminSvc := admin.NewService(userRepo, bybitClient)

	// Handlers
	h := deliveryhttp.Handlers{
		Auth:    handlers.NewAuthHandler(authSvc, cfg.RefreshTokenTTL, isProd()),
		Ads:     handlers.NewAdHandler(adsSvc),
		Orders:  handlers.NewOrderHandler(ordersSvc, userRepo),
		Wallet:  handlers.NewWalletHandler(walletSvc),
		KYC:     handlers.NewKYCHandler(kycSvc, localStorage),
		Dispute: handlers.NewDisputeHandler(disputeSvc),
		Admin:   handlers.NewAdminHandler(adminSvc),
		Chat:    handlers.NewChatHandler(chatSvc, hub, cfg.AllowedOrigin),
		Upload:  handlers.NewUploadHandler(localStorage),
	}

	router := deliveryhttp.NewRouter(h, cfg.JWTSecret, cfg.AllowedOrigin)
	router.Static("/uploads", cfg.StorageDir)

	startKeepAlive(os.Getenv("BACKEND_URL"))
	startExpirySweeper(ordersSvc)

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

// startExpirySweeper polls every minute for orders whose payment deadline
// has passed while still awaiting payment, auto-cancelling them so they stop
// counting as active trades (and free up the merchant's locked crypto / the
// ad's capacity) without needing anyone to click Cancel.
func startExpirySweeper(ordersSvc *orders.Service) {
	go func() {
		for {
			time.Sleep(1 * time.Minute)
			n, err := ordersSvc.ExpireStale(context.Background())
			if err != nil {
				log.Printf("expiry sweep failed: %v", err)
				continue
			}
			if n > 0 {
				log.Printf("expiry sweep: auto-cancelled %d order(s)", n)
			}
		}
	}()
}

// startKeepAlive pings our own /healthz every 14 minutes so Render's free
// tier doesn't spin the service down after 15 minutes of no traffic. A
// no-op if BACKEND_URL isn't set (e.g. local dev).
func startKeepAlive(backendURL string) {
	if backendURL == "" {
		return
	}
	client := &http.Client{Timeout: 10 * time.Second}
	go func() {
		for {
			time.Sleep(14 * time.Minute)
			resp, err := client.Get(backendURL + "/healthz")
			if err != nil {
				log.Printf("keep-alive ping failed: %v", err)
				continue
			}
			resp.Body.Close()
		}
	}()
}
