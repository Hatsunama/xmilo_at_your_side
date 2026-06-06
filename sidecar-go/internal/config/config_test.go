package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var configEnvNames = []string{
	"XMILOCLAW_HOST",
	"XMILOCLAW_PORT",
	"XMILOCLAW_DB_PATH",
	"XMILOCLAW_BEARER_TOKEN",
	"XMILOCLAW_RELAY_BASE_URL",
	"XMILOCLAW_MIND_ROOT",
	"XMILOCLAW_RUNTIME_ID",
	"XMILOCLAW_CONFIG",
	"XMILO_SIDECAR_HOST",
	"XMILO_SIDECAR_PORT",
	"XMILO_SIDECAR_DB_PATH",
	"XMILO_BEARER_TOKEN",
	"XMILO_SIDECAR_BEARER_TOKEN",
	"XMILO_RELAY_BASE_URL",
	"XMILO_RELAY_URL",
	"XMILO_SIDECAR_RELAY_BASE_URL",
	"XMILO_MIND_ROOT",
	"XMILO_SIDECAR_MIND_ROOT",
	"XMILO_RUNTIME_ID",
	"XMILO_SIDECAR_RUNTIME_ID",
	"XMILO_SIDECAR_CONFIG",
	"XMILO_LLM_MODE",
	"XMILO_SIDECAR_LLM_MODE",
	"XMILO_BYOK_PROVIDER",
	"XMILO_SIDECAR_BYOK_PROVIDER",
	"XMILO_BYOK_KEY_ENV",
	"XMILO_SIDECAR_BYOK_KEY_ENV",
	"XMILO_BYOK_KEY_FILE",
	"XMILO_SIDECAR_BYOK_KEY_FILE",
	"XMILO_BYOK_BASE_URL",
	"XMILO_SIDECAR_BYOK_BASE_URL",
	"XMILO_BYOK_MODEL",
	"XMILO_SIDECAR_BYOK_MODEL",
}

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, name := range configEnvNames {
		t.Setenv(name, "")
	}
}

func TestXMILOCLAWEnvNamesLoadAffectedFields(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("XMILOCLAW_HOST", "10.0.0.2")
	t.Setenv("XMILOCLAW_PORT", "43123")
	t.Setenv("XMILOCLAW_DB_PATH", filepath.Join(t.TempDir(), "xmilo.db"))
	t.Setenv("XMILOCLAW_BEARER_TOKEN", "test-token")
	t.Setenv("XMILOCLAW_RELAY_BASE_URL", "https://relay.example.test")
	t.Setenv("XMILOCLAW_MIND_ROOT", filepath.Join(t.TempDir(), "mind"))
	t.Setenv("XMILOCLAW_RUNTIME_ID", "runtime-xmiloclaw")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Host != "10.0.0.2" ||
		cfg.Port != 43123 ||
		!strings.HasSuffix(cfg.DBPath, "xmilo.db") ||
		cfg.BearerToken != "test-token" ||
		cfg.RelayBaseURL != "https://relay.example.test" ||
		!strings.HasSuffix(cfg.MindRoot, "mind") ||
		cfg.RuntimeID != "runtime-xmiloclaw" {
		t.Fatalf("Load() did not use XMILOCLAW env fields: %+v", cfg)
	}
}

func TestCurrentXMILOAliasesStillWork(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("XMILO_SIDECAR_HOST", "127.0.0.9")
	t.Setenv("XMILO_SIDECAR_PORT", "43000")
	t.Setenv("XMILO_SIDECAR_DB_PATH", "xmilo-alias.db")
	t.Setenv("XMILO_BEARER_TOKEN", "alias-token")
	t.Setenv("XMILO_RELAY_BASE_URL", "https://relay.alias.test")
	t.Setenv("XMILO_MIND_ROOT", "alias-mind")
	t.Setenv("XMILO_RUNTIME_ID", "alias-runtime")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Host != "127.0.0.9" ||
		cfg.Port != 43000 ||
		cfg.DBPath != "xmilo-alias.db" ||
		cfg.BearerToken != "alias-token" ||
		cfg.RelayBaseURL != "https://relay.alias.test" ||
		cfg.MindRoot != "alias-mind" ||
		cfg.RuntimeID != "alias-runtime" {
		t.Fatalf("Load() did not preserve current xMilo aliases: %+v", cfg)
	}
}

func TestXMILOCLAWEnvPrecedenceOverCurrentAliases(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("XMILOCLAW_HOST", "10.1.1.1")
	t.Setenv("XMILO_SIDECAR_HOST", "10.2.2.2")
	t.Setenv("XMILOCLAW_PORT", "42999")
	t.Setenv("XMILO_SIDECAR_PORT", "42998")
	t.Setenv("XMILOCLAW_BEARER_TOKEN", "new-token")
	t.Setenv("XMILO_BEARER_TOKEN", "old-current-token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Host != "10.1.1.1" || cfg.Port != 42999 || cfg.BearerToken != "new-token" {
		t.Fatalf("XMILOCLAW envs did not take precedence: %+v", cfg)
	}
}

func TestXMILOCLAWConfigLoadsConfigFile(t *testing.T) {
	clearConfigEnv(t)
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfigFile(t, path, Config{
		Host:         "127.0.0.7",
		Port:         42901,
		DBPath:       "file-config.db",
		BearerToken:  "file-token",
		RelayBaseURL: "https://relay.file.test",
		MindRoot:     "file-mind",
		RuntimeID:    "file-runtime",
		BYOKProvider: "xai",
	})
	t.Setenv("XMILOCLAW_CONFIG", path)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Host != "127.0.0.7" ||
		cfg.Port != 42901 ||
		cfg.DBPath != "file-config.db" ||
		cfg.BearerToken != "file-token" ||
		cfg.RelayBaseURL != "https://relay.file.test" ||
		cfg.MindRoot != "file-mind" ||
		cfg.RuntimeID != "file-runtime" {
		t.Fatalf("Load() did not load XMILOCLAW_CONFIG file: %+v", cfg)
	}
}

func TestXMILOCLAWConfigFileValuesOverrideFieldEnvValues(t *testing.T) {
	clearConfigEnv(t)
	path := filepath.Join(t.TempDir(), "config.json")
	writeConfigFile(t, path, Config{
		Host:         "file-host",
		Port:         42902,
		DBPath:       "file.db",
		BearerToken:  "file-token",
		RelayBaseURL: "https://relay.file-override.test",
		MindRoot:     "file-mind-root",
		RuntimeID:    "file-runtime-id",
		BYOKProvider: "xai",
	})
	t.Setenv("XMILOCLAW_CONFIG", path)
	t.Setenv("XMILOCLAW_HOST", "env-host")
	t.Setenv("XMILOCLAW_PORT", "42903")
	t.Setenv("XMILOCLAW_DB_PATH", "env.db")
	t.Setenv("XMILOCLAW_BEARER_TOKEN", "env-token")
	t.Setenv("XMILOCLAW_RELAY_BASE_URL", "https://relay.env.test")
	t.Setenv("XMILOCLAW_MIND_ROOT", "env-mind-root")
	t.Setenv("XMILOCLAW_RUNTIME_ID", "env-runtime-id")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Host != "file-host" ||
		cfg.Port != 42902 ||
		cfg.DBPath != "file.db" ||
		cfg.BearerToken != "file-token" ||
		cfg.RelayBaseURL != "https://relay.file-override.test" ||
		cfg.MindRoot != "file-mind-root" ||
		cfg.RuntimeID != "file-runtime-id" {
		t.Fatalf("config file did not preserve current file-over-env behavior: %+v", cfg)
	}
}

func TestMissingBearerErrorUsesCurrentNamesOnly(t *testing.T) {
	clearConfigEnv(t)

	_, err := Load()
	if err == nil {
		t.Fatalf("Load() error = nil, want missing bearer token error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "XMILOCLAW_BEARER_TOKEN") || !strings.Contains(msg, "XMILO_BEARER_TOKEN") {
		t.Fatalf("missing bearer error does not mention current names: %v", err)
	}
	if strings.Contains(msg, "legacy") || strings.Contains(msg, "retired") {
		t.Fatalf("missing bearer error uses legacy/retired wording: %v", err)
	}
}

func writeConfigFile(t *testing.T, path string, cfg Config) {
	t.Helper()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
}
