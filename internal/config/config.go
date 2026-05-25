package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
)

// Config holds runtime configuration sourced from environment variables.
type Config struct {
	Port            string
	DatabaseDSN     string
	SessionSecret   string
	SessionLifetime int // hours
	SecureCookies   bool
	AvatarDir       string
	AppBaseURL      string
	SMTPHost        string
	SMTPPort        int
	SMTPUsername    string
	SMTPPassword    string
	SMTPFrom        string
	SMTPTLSMode     string
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
		AppBaseURL:      strings.TrimRight(getenv("APP_BASE_URL", "http://localhost:8080"), "/"),
		SMTPHost:        os.Getenv("SMTP_HOST"),
		SMTPPort:        getenvInt("SMTP_PORT", 587),
		SMTPUsername:    os.Getenv("SMTP_USERNAME"),
		SMTPPassword:    os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:        os.Getenv("SMTP_FROM"),
		SMTPTLSMode:     strings.ToLower(getenv("SMTP_TLS_MODE", "starttls")),
	}
	if cfg.DatabaseDSN == "" {
		return cfg, errors.New("APP_DATABASE_DSN is required")
	}
	if len(cfg.SessionSecret) < 16 {
		return cfg, errors.New("SESSION_SECRET must be at least 16 characters")
	}
	if cfg.AppBaseURL == "" {
		return cfg, errors.New("APP_BASE_URL is required")
	}
	if cfg.SMTPHost == "" {
		return cfg, errors.New("SMTP_HOST is required")
	}
	if cfg.SMTPPort <= 0 {
		return cfg, errors.New("SMTP_PORT must be positive")
	}
	if cfg.SMTPFrom == "" {
		return cfg, errors.New("SMTP_FROM is required")
	}
	switch cfg.SMTPTLSMode {
	case "starttls", "implicit", "none":
	default:
		return cfg, errors.New("SMTP_TLS_MODE must be starttls, implicit, or none")
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
