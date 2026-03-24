package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	Host         string `json:"host"`
	Port         int    `json:"port"`
	DBPath       string `json:"db_path"`
	BearerToken  string `json:"bearer_token"`
	RelayBaseURL string `json:"relay_base_url"`
	MindRoot     string `json:"mind_root"`
	RuntimeID    string `json:"runtime_id"`
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

	return cfg, nil
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
