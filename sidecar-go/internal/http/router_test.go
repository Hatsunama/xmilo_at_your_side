package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"xmilo/sidecar-go/internal/config"
	"xmilo/sidecar-go/internal/db"
)

func TestHandleTaskStartFailsFastWithoutRelayJWT(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	app := &App{store: store}

	req := httptest.NewRequest(http.MethodPost, "/task/start", strings.NewReader(`{"prompt":"Hello"}`))
	rec := httptest.NewRecorder()

	app.handleTaskStart(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d", http.StatusForbidden, rec.Code)
	}

	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out["error"] != "entitlement_lost" {
		t.Fatalf("expected entitlement_lost, got %#v", out["error"])
	}
	if out["error_code"] != "entitlement_lost" {
		t.Fatalf("expected entitlement_lost error_code, got %#v", out["error_code"])
	}
}

func TestHandleTaskStartLocalBYOKMissingKeyDoesNotReportSubscriptionEntitlement(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	app := &App{
		cfg:   config.Config{LLMMode: "local_byok", BYOKKeyFile: filepath.Join(t.TempDir(), "missing.key")},
		store: store,
	}

	req := httptest.NewRequest(http.MethodPost, "/task/start", strings.NewReader(`{"prompt":"Hello"}`))
	rec := httptest.NewRecorder()

	app.handleTaskStart(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected %d, got %d", http.StatusForbidden, rec.Code)
	}

	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out["error"] != "missing_local_provider_key" {
		t.Fatalf("expected missing_local_provider_key, got %#v", out["error"])
	}
}

func TestHandleAuthCheckLocalBYOKReturnsLocalAccessWithoutRelayEntitlement(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "provider.key")
	if err := os.WriteFile(keyPath, []byte("local-key"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	app := &App{
		cfg: config.Config{LLMMode: "local_byok", BYOKKeyFile: keyPath},
	}

	req := httptest.NewRequest(http.MethodGet, "/auth/check", http.NoBody)
	rec := httptest.NewRecorder()

	app.handleAuthCheck(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, rec.Code)
	}

	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out["subscription_entitled"] != false || out["entitled"] != false {
		t.Fatalf("BYOK must not masquerade as subscription entitlement: %#v", out)
	}
	if out["first_task_eligible"] != true || out["local_llm_turn_allowed"] != true || out["relay_llm_turn_allowed"] != false {
		t.Fatalf("unexpected local access fields: %#v", out)
	}
}
