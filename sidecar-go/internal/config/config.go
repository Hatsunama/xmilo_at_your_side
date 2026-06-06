package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"xmilo/sidecar-go/internal/providerpolicy"
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
		Host:         getenvAny([]string{"XMILOCLAW_HOST", "XMILO_SIDECAR_HOST"}, "127.0.0.1"),
		Port:         getenvIntAny([]string{"XMILOCLAW_PORT", "XMILO_SIDECAR_PORT"}, 42817),
		DBPath:       getenvAny([]string{"XMILOCLAW_DB_PATH", "XMILO_SIDECAR_DB_PATH"}, filepath.Join(".xmilo", "xmilo.db")),
		BearerToken:  getenvAny([]string{"XMILOCLAW_BEARER_TOKEN", "XMILO_BEARER_TOKEN", "XMILO_SIDECAR_BEARER_TOKEN"}, ""),
		RelayBaseURL: getenvAny([]string{"XMILOCLAW_RELAY_BASE_URL", "XMILO_RELAY_BASE_URL", "XMILO_RELAY_URL", "XMILO_SIDECAR_RELAY_BASE_URL"}, "http://127.0.0.1:8080"),
		MindRoot:     getenvAny([]string{"XMILOCLAW_MIND_ROOT", "XMILO_MIND_ROOT", "XMILO_SIDECAR_MIND_ROOT"}, filepath.Join("..", "docs", "authority", "xMilo_v1")),
		RuntimeID:    getenvAny([]string{"XMILOCLAW_RUNTIME_ID", "XMILO_RUNTIME_ID", "XMILO_SIDECAR_RUNTIME_ID"}, "dev-local"),
		LLMMode:      getenvAny([]string{"XMILO_LLM_MODE", "XMILO_SIDECAR_LLM_MODE"}, "relay"),
		BYOKProvider: getenvAny([]string{"XMILO_BYOK_PROVIDER", "XMILO_SIDECAR_BYOK_PROVIDER"}, "xai"),
		BYOKKeyEnv:   getenvAny([]string{"XMILO_BYOK_KEY_ENV", "XMILO_SIDECAR_BYOK_KEY_ENV"}, "XMILO_BYOK_API_KEY"),
		BYOKKeyFile:  getenvAny([]string{"XMILO_BYOK_KEY_FILE", "XMILO_SIDECAR_BYOK_KEY_FILE"}, ""),
		BYOKBaseURL:  getenvAny([]string{"XMILO_BYOK_BASE_URL", "XMILO_SIDECAR_BYOK_BASE_URL"}, ""),
		BYOKModel:    getenvAny([]string{"XMILO_BYOK_MODEL", "XMILO_SIDECAR_BYOK_MODEL"}, ""),
	}

	if path := getenvAny([]string{"XMILOCLAW_CONFIG", "XMILO_SIDECAR_CONFIG"}, ""); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return Config{}, err
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			return Config{}, err
		}
	}

	if cfg.BearerToken == "" {
		return Config{}, errors.New("missing bearer token: set XMILOCLAW_BEARER_TOKEN, XMILO_BEARER_TOKEN, or provide config file bearer_token")
	}
	cfg.ApplyBYOKProviderDefaults()

	return cfg, nil
}

func (c Config) LocalBYOKActive() bool {
	return strings.TrimSpace(c.LLMMode) == "local_byok"
}

func (c *Config) ApplyBYOKProviderDefaults() {
	provider, err := providerpolicy.NormalizeProvider(c.BYOKProvider)
	if err != nil {
		c.BYOKProvider = strings.ToLower(strings.TrimSpace(c.BYOKProvider))
		return
	}
	c.BYOKProvider = provider
	if strings.TrimSpace(c.BYOKKeyEnv) == "" {
		c.BYOKKeyEnv = "XMILO_BYOK_API_KEY"
	}
	spec, err := providerpolicy.Spec(provider)
	if err != nil {
		return
	}
	if strings.TrimSpace(c.BYOKBaseURL) == "" {
		c.BYOKBaseURL = spec.DefaultBaseURL
	}
	if strings.TrimSpace(c.BYOKModel) == "" {
		c.BYOKModel = spec.DefaultModel
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
