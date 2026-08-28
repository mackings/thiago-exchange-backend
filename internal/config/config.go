package config

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	Port            string
	MongoURI        string
	MongoDBName     string
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	AllowedOrigin   string
	BybitAPIKey     string
	BybitAPISecret  string
	BybitBaseURL    string
	StorageDir      string
	AdminEmails     []string
	SMTPHost        string
	SMTPPort        string
	SMTPUser        string
	SMTPPass        string
	SMTPFromName    string
}

func Load() Config {
	return Config{
		Port:            getEnv("PORT", "8080"),
		MongoURI:        getEnv("MONGO_URI", "mongodb://localhost:27017/thiago_exchange?replicaSet=rs0"),
		MongoDBName:     getEnv("MONGO_DB_NAME", "thiago_exchange"),
		JWTSecret:       getEnv("JWT_SECRET", "dev-secret-change-me"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 30 * 24 * time.Hour,
		AllowedOrigin:   getEnv("ALLOWED_ORIGIN", "http://localhost:3000"),
		BybitAPIKey:     os.Getenv("BYBIT_API_KEY"),
		BybitAPISecret:  os.Getenv("BYBIT_API_SECRET"),
		BybitBaseURL:    getEnv("BYBIT_BASE_URL", "https://api.bybit.com"),
		StorageDir:      getEnv("STORAGE_DIR", "./data/uploads"),
		AdminEmails:     splitCSV(os.Getenv("ADMIN_EMAILS")),
		SMTPHost:        getEnv("SMTP_HOST", "smtp.gmail.com"),
		SMTPPort:        getEnv("SMTP_PORT", "587"),
		SMTPUser:        os.Getenv("SMTP_USER"),
		SMTPPass:        os.Getenv("SMTP_PASS"),
		SMTPFromName:    getEnv("SMTP_FROM_NAME", "Thiago Exchange"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitCSV(v string) []string {
	if v == "" {
		return nil
	}
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
