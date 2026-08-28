package config

import (
	"os"
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
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
