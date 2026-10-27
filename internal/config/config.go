package config

import (
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration loaded from environment variables.
type Config struct {
	Port             string
	DatabaseURL      string
	JWTSecret        string
	JWTExpiry        time.Duration
	GroqAPIKey       string
	GroqModel        string
	GeminiAPIKey     string
	GeminiModel      string
	CronInterval     time.Duration
	FeedWorkerCount  int
	AppEnv           string
	AdminDefaultUser string
	AdminDefaultPass string
}

// Load reads a .env file (if present) and then environment variables,
// applying sensible defaults where a value is not set.
func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("[config] no .env file found, relying on real environment variables")
	}

	cfg := &Config{
		Port:             getEnv("PORT", "8080"),
		DatabaseURL:      getEnv("DATABASE_URL", ""),
		JWTSecret:        getEnv("JWT_SECRET", "change-this-secret-in-production"),
		JWTExpiry:        getEnvDuration("JWT_EXPIRY", 24*time.Hour),
		GroqAPIKey:       getEnv("GROQ_API_KEY", ""),
		GroqModel:        getEnv("GROQ_MODEL", "openai/gpt-oss-20b"),
		GeminiAPIKey:     getEnv("GEMINI_API_KEY", ""),
		GeminiModel:      getEnv("GEMINI_MODEL", "gemini-1.5-flash"),
		CronInterval:     getEnvDuration("CRON_INTERVAL", 1*time.Hour),
		FeedWorkerCount:  getEnvInt("FEED_WORKER_COUNT", 5),
		AppEnv:           getEnv("APP_ENV", "development"),
		AdminDefaultUser: getEnv("ADMIN_DEFAULT_USER", "admin"),
		AdminDefaultPass: getEnv("ADMIN_DEFAULT_PASS", "admin123"),
	}

	if cfg.DatabaseURL == "" {
		log.Fatal("[config] DATABASE_URL is required (Neon/Supabase Postgres connection string)")
	}
	if cfg.CronInterval <= 0 {
		log.Fatal("[config] CRON_INTERVAL must be a positive Go duration, e.g. 1h")
	}
	if cfg.FeedWorkerCount < 1 {
		cfg.FeedWorkerCount = 1
	}
	if cfg.JWTExpiry <= 0 {
		log.Fatal("[config] JWT_EXPIRY must be a positive Go duration, e.g. 24h")
	}
	if cfg.AppEnv == "production" {
		if len(cfg.JWTSecret) < 32 || cfg.JWTSecret == "change-this-secret-in-production" {
			log.Fatal("[config] production requires a unique JWT_SECRET of at least 32 characters")
		}
		if cfg.AdminDefaultPass == "admin123" || len(cfg.AdminDefaultPass) < 12 {
			log.Fatal("[config] production requires ADMIN_DEFAULT_PASS to be changed to at least 12 characters before first startup")
		}
	}

	return cfg
}

func getEnv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
