package config

import (
	"errors"
	"os"
	"strconv"
)

type Config struct {
	HTTPAddr      string
	PublicBaseURL string
	PostgresDSN   string
	JWTSecret     string
	XAIAPIKey     string
	XAIBaseURL    string
	XAIModel      string
	DevEntitled   bool

	// SMTP — dev mode logs to stdout when Host is empty
	SMTPFrom     string
	SMTPHost     string
	SMTPPort     string
	SMTPUsername string
	SMTPPassword string
	SMTPUseTLS   bool

	// Admin page password — endpoint is /admin
	AdminPassword string

	// Product rules (locked: trial=12h, invite=5d, price=$19.99)
	TrialHours     int
	InviteBetaDays int

	// RevenueCat webhook shared secret
	RevenueCatWebhookAuth string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:      getenv("RELAY_HTTP_ADDR", ":8080"),
		PublicBaseURL: getenv("RELAY_PUBLIC_BASE_URL", "https://xmiloatyourside.com"),
		PostgresDSN:   getenv("RELAY_POSTGRES_DSN", ""),
		JWTSecret:     getenv("RELAY_JWT_SECRET", ""),
		XAIAPIKey:     getenv("XAI_API_KEY", ""),
		XAIBaseURL:    getenv("XAI_BASE_URL", "https://api.x.ai/v1"),
		XAIModel:      getenv("XAI_MODEL", "grok-4"),
		// DevEntitled defaults false in production; set true only in local dev
		DevEntitled: getenv("RELAY_DEV_ENTITLED", "false") != "false",

		SMTPFrom:     getenv("SMTP_FROM", "noreply@xmiloatyourside.com"),
		SMTPHost:     getenv("SMTP_HOST", ""),
		SMTPPort:     getenv("SMTP_PORT", "587"),
		SMTPUsername: getenv("SMTP_USERNAME", ""),
		SMTPPassword: getenv("SMTP_PASSWORD", ""),
		SMTPUseTLS:   getenv("SMTP_USE_TLS", "true") != "false",

		AdminPassword: getenv("RELAY_ADMIN_PASSWORD", ""),

		TrialHours:     getenvInt("XMILO_TRIAL_HOURS", 12),
		InviteBetaDays: getenvInt("XMILO_INVITE_BETA_DAYS", 5),

		RevenueCatWebhookAuth: getenv("REVENUECAT_WEBHOOK_AUTH", ""),
	}

	if cfg.PostgresDSN == "" {
		return Config{}, errors.New("missing RELAY_POSTGRES_DSN")
	}
	if cfg.JWTSecret == "" {
		return Config{}, errors.New("missing RELAY_JWT_SECRET")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
