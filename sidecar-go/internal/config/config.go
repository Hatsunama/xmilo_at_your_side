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
		Host:         getenv("PICOCLAW_HOST", "127.0.0.1"),
		Port:         getenvInt("PICOCLAW_PORT", 42817),
		DBPath:       getenv("PICOCLAW_DB_PATH", filepath.Join(".miloclaw", "picoclaw.sqlite")),
		BearerToken:  getenv("PICOCLAW_BEARER_TOKEN", ""),
		RelayBaseURL: getenv("PICOCLAW_RELAY_BASE_URL", "http://127.0.0.1:8080"),
		MindRoot:     getenv("PICOCLAW_MIND_ROOT", filepath.Join("..", "docs", "authority", "xMilo_v1")),
		RuntimeID:    getenv("PICOCLAW_RUNTIME_ID", "dev-local"),
	}

	if path := os.Getenv("PICOCLAW_CONFIG"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return Config{}, err
		}
		if err := json.Unmarshal(data, &cfg); err != nil {
			return Config{}, err
		}
	}

	if cfg.BearerToken == "" {
		return Config{}, errors.New("missing bearer token: set PICOCLAW_BEARER_TOKEN or config file bearer_token")
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
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	return fallback
}
