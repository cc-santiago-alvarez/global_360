// Package config loads the application configuration from environment variables
// (optionally from a .env file) and validates it at startup.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                   string
	MongoURI               string
	MongoDatabase          string
	JWTSecret              string
	JWTIssuer              string
	AccessTokenTTL         time.Duration
	RefreshTokenTTL        time.Duration
	BcryptCost             int
	MaxFailedLoginAttempts int
	SeedSuperadminEmail    string
	SeedSuperadminPassword string
	LogLevel               string
	GinMode                string
}

// Load reads .env (if present) and the process environment.
func Load() (Config, error) {
	_ = godotenv.Load() // .env is optional; real environment variables take precedence

	var errs []error
	cfg := Config{
		Port:                   getEnv("PORT", "8080"),
		MongoURI:               os.Getenv("MONGO_URI"),
		MongoDatabase:          getEnv("MONGO_DATABASE", "global360"),
		JWTSecret:              os.Getenv("JWT_SECRET"),
		JWTIssuer:              getEnv("JWT_ISSUER", "global360"),
		AccessTokenTTL:         getDuration("JWT_ACCESS_TTL", 15*time.Minute, &errs),
		RefreshTokenTTL:        getDuration("REFRESH_TOKEN_TTL", 7*24*time.Hour, &errs),
		BcryptCost:             getInt("BCRYPT_COST", 12, &errs),
		MaxFailedLoginAttempts: getInt("MAX_FAILED_LOGIN_ATTEMPTS", 5, &errs),
		SeedSuperadminEmail:    strings.TrimSpace(os.Getenv("SEED_SUPERADMIN_EMAIL")),
		SeedSuperadminPassword: os.Getenv("SEED_SUPERADMIN_PASSWORD"),
		LogLevel:               getEnv("LOG_LEVEL", "info"),
		GinMode:                getEnv("GIN_MODE", "release"),
	}

	if cfg.MongoURI == "" {
		errs = append(errs, errors.New("MONGO_URI is required"))
	}
	if len(cfg.JWTSecret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET is required and must have at least 32 characters"))
	}
	if cfg.BcryptCost < 10 || cfg.BcryptCost > 15 {
		errs = append(errs, errors.New("BCRYPT_COST must be between 10 and 15"))
	}
	if cfg.MaxFailedLoginAttempts < 1 {
		errs = append(errs, errors.New("MAX_FAILED_LOGIN_ATTEMPTS must be positive"))
	}
	if cfg.AccessTokenTTL <= 0 || cfg.RefreshTokenTTL <= cfg.AccessTokenTTL {
		errs = append(errs, errors.New("REFRESH_TOKEN_TTL must be greater than JWT_ACCESS_TTL and both positive"))
	}
	return cfg, errors.Join(errs...)
}

func getEnv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func getInt(key string, def int, errs *[]error) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s must be an integer", key))
		return def
	}
	return n
}

func getDuration(key string, def time.Duration, errs *[]error) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		*errs = append(*errs, fmt.Errorf("%s must be a duration such as 15m or 168h", key))
		return def
	}
	return d
}
