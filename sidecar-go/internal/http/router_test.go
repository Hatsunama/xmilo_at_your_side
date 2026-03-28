package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

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
