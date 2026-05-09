package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	DBPath       string `json:"db_path"`
	BearerToken  string `json:"bearer_token"`
	RelayBaseURL string `json:"relay_base_url"`
	MindRoot     string `json:"mind_root"`
	RuntimeID    string `json:"runtime_id"`
	LLMMode      string `json:"llm_mode"`
	BYOKProvider string `json:"byok_provider"`
	BYOKKeyEnv   string `json:"byok_key_env"`
	BYOKKeyFile  string `json:"byok_key_file"`
	BYOKBaseURL  string `json:"byok_base_url"`
	BYOKModel    string `json:"byok_model"`
}

func Load() (Config, error) {
	cfg := Config{
		Host:         getenvAny([]string{"XMILO_SIDECAR_HOST", "PICOCLAW_HOST"}, "127.0.0.1"),
		Port:         getenvIntAny([]string{"XMILO_SIDECAR_PORT", "PICOCLAW_PORT"}, 42817),
		DBPath:       getenvAny([]string{"XMILO_SIDECAR_DB_PATH", "PICOCLAW_DB_PATH"}, filepath.Join(".xmilo", "xmilo.db")),
		BearerToken:  getenvAny([]string{"XMILO_BEARER_TOKEN", "XMILO_SIDECAR_BEARER_TOKEN", "PICOCLAW_BEARER_TOKEN"}, ""),
		RelayBaseURL: getenvAny([]string{"XMILO_RELAY_BASE_URL", "XMILO_RELAY_URL", "XMILO_SIDECAR_RELAY_BASE_URL", "PICOCLAW_RELAY_BASE_URL"}, "http://127.0.0.1:8080"),
		MindRoot:     getenvAny([]string{"XMILO_MIND_ROOT", "XMILO_SIDECAR_MIND_ROOT", "PICOCLAW_MIND_ROOT"}, filepath.Join("..", "docs", "authority", "xMilo_v1")),
		RuntimeID:    getenvAny([]string{"XMILO_RUNTIME_ID", "XMILO_SIDECAR_RUNTIME_ID", "PICOCLAW_RUNTIME_ID"}, "dev-local"),
		LLMMode:      getenvAny([]string{"XMILO_LLM_MODE", "XMILO_SIDECAR_LLM_MODE"}, "relay"),
		BYOKProvider: getenvAny([]string{"XMILO_BYOK_PROVIDER", "XMILO_SIDECAR_BYOK_PROVIDER"}, "xai"),
		BYOKKeyEnv:   getenvAny([]string{"XMILO_BYOK_KEY_ENV", "XMILO_SIDECAR_BYOK_KEY_ENV"}, "XMILO_BYOK_API_KEY"),
		BYOKKeyFile:  getenvAny([]string{"XMILO_BYOK_KEY_FILE", "XMILO_SIDECAR_BYOK_KEY_FILE"}, ""),
		BYOKBaseURL:  getenvAny([]string{"XMILO_BYOK_BASE_URL", "XMILO_SIDECAR_BYOK_BASE_URL"}, ""),
		BYOKModel:    getenvAny([]string{"XMILO_BYOK_MODEL", "XMILO_SIDECAR_BYOK_MODEL"}, ""),
	}

	if path := getenvAny([]string{"XMILO_SIDECAR_CONFIG", "PICOCLAW_CONFIG"}, ""); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return Config{}, err
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			return Config{}, err
		}
	}

	if cfg.BearerToken == "" {
		return Config{}, errors.New("missing bearer token: set XMILO_BEARER_TOKEN or provide config file bearer_token")
	}
	cfg.ApplyBYOKProviderDefaults()

	return cfg, nil
}

func (c Config) LocalBYOKActive() bool {
	return strings.TrimSpace(c.LLMMode) == "local_byok"
}

func (c *Config) ApplyBYOKProviderDefaults() {
	c.BYOKProvider = strings.ToLower(strings.TrimSpace(c.BYOKProvider))
	if c.BYOKProvider == "" || c.BYOKProvider == "grok" {
		c.BYOKProvider = "xai"
	}
	if strings.TrimSpace(c.BYOKKeyEnv) == "" {
		c.BYOKKeyEnv = "XMILO_BYOK_API_KEY"
	}
	if strings.TrimSpace(c.BYOKBaseURL) == "" {
		switch c.BYOKProvider {
		case "xai":
			c.BYOKBaseURL = "https://api.x.ai/v1"
		case "openai":
			c.BYOKBaseURL = "https://api.openai.com/v1"
		case "anthropic":
			c.BYOKBaseURL = "https://api.anthropic.com/v1"
		}
	}
	if strings.TrimSpace(c.BYOKModel) == "" {
		switch c.BYOKProvider {
		case "xai":
			c.BYOKModel = "grok-4"
		case "openai":
			c.BYOKModel = "gpt-5.4"
		case "anthropic":
			c.BYOKModel = "claude-sonnet-4-5"
		case "ollama":
			c.BYOKModel = "llama3.2"
		}
	}
}

func getenvAny(keys []string, fallback string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
	}
	return fallback
}

func getenvIntAny(keys []string, fallback int) int {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			if parsed, err := strconv.Atoi(v); err == nil {
				return parsed
			}
		}
	}
	return fallback
}
