package config

import (
	"errors"
	"os"
	"strconv"
)

// Config holds runtime configuration sourced from environment variables.
type Config struct {
	Port            string
	DatabaseDSN     string
	SessionSecret   string
	SessionLifetime int // hours
	SecureCookies   bool
	AvatarDir       string
}

// Load reads configuration from the environment. .env loading should already
// have happened in main before calling Load.
func Load() (Config, error) {
	cfg := Config{
		Port:            getenv("PORT", "8080"),
		DatabaseDSN:     os.Getenv("APP_DATABASE_DSN"),
		SessionSecret:   os.Getenv("SESSION_SECRET"),
		SessionLifetime: getenvInt("SESSION_LIFETIME_HOURS", 24*14),
		SecureCookies:   getenvBool("SECURE_COOKIES", false),
		AvatarDir:       getenv("APP_AVATAR_DIR", "web/static/avatars"),
	}
	if cfg.DatabaseDSN == "" {
		return cfg, errors.New("APP_DATABASE_DSN is required")
	}
	if len(cfg.SessionSecret) < 16 {
		return cfg, errors.New("SESSION_SECRET must be at least 16 characters")
	}
	return cfg, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
