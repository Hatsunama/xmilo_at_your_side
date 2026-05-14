package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xmilo/sidecar-go/internal/config"
	"xmilo/sidecar-go/internal/contextpolicy"
	"xmilo/sidecar-go/internal/db"
	"xmilo/sidecar-go/internal/runtime"
	"xmilo/sidecar-go/internal/tasks"
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

func TestHandleReadyLocalBYOKExposesResolvedProviderIdentity(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "provider.key")
	if err := os.WriteFile(keyPath, []byte("local-key"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	app := &App{
		cfg: config.Config{
			LLMMode:      "local_byok",
			BYOKProvider: "openai",
			BYOKKeyFile:  keyPath,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/ready", http.NoBody)
	rec := httptest.NewRecorder()

	app.handleReady(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, rec.Code)
	}

	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out["llm_mode"] != "local_byok" {
		t.Fatalf("expected local_byok llm mode, got %#v", out["llm_mode"])
	}
	if out["byok_provider"] != "openai" {
		t.Fatalf("ready response must expose sidecar-resolved BYOK provider identity, got %#v", out["byok_provider"])
	}
	if out["byok_provider"] == "unknown" || out["byok_provider"] == "" {
		t.Fatalf("ready response must not use unknown provider identity: %#v", out)
	}
	if out["local_llm_turn_allowed"] != true || out["first_task_eligible"] != true {
		t.Fatalf("expected local BYOK readiness fields to be true, got %#v", out)
	}
}

func TestHandleLocalProviderOptionsReturnsSidecarPolicy(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodGet, "/local-provider/options", http.NoBody)
	rec := httptest.NewRecorder()

	app.handleLocalProviderOptions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, rec.Code)
	}
	var out struct {
		Providers []struct {
			ID             string   `json:"id"`
			AllowedSchemes []string `json:"allowed_schemes"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(out.Providers) != 4 {
		t.Fatalf("expected four providers, got %#v", out)
	}
	for _, provider := range out.Providers {
		if provider.ID == "openai" && (len(provider.AllowedSchemes) != 1 || provider.AllowedSchemes[0] != "https") {
			t.Fatalf("openai must remain https-only by default: %#v", provider)
		}
	}
}

func TestHandleLocalProviderResolveRejectsCloudHTTP(t *testing.T) {
	app := &App{}
	req := httptest.NewRequest(http.MethodPost, "/local-provider/resolve", strings.NewReader(`{"provider":"openai","base_url":"http://api.openai.com/v1","model":"gpt-test","key_file_ready":true}`))
	rec := httptest.NewRecorder()

	app.handleLocalProviderResolve(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d body=%s", http.StatusBadRequest, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "local_provider_disallowed_url_scheme") {
		t.Fatalf("expected disallowed scheme response, got %s", rec.Body.String())
	}
}

func TestHandleContextSetStoresBoundedMetadata(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := &App{store: store}

	req := httptest.NewRequest(http.MethodPost, "/context/set", strings.NewReader(`{"content":"hello\r\nworld","source":"document_picker","provenance":"document_picker","label":"notes.txt","mime_type":"text/plain"}`))
	rec := httptest.NewRecorder()

	app.handleContextSet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	content, _ := store.GetRuntimeConfig("active_context")
	if content != "hello\nworld" {
		t.Fatalf("expected normalized context, got %q", content)
	}
	metaRaw, _ := store.GetRuntimeConfig("active_context_meta")
	stored, ok := contextpolicy.ParseStored(content, metaRaw, timeNow())
	if !ok {
		t.Fatalf("stored context did not parse with metadata: content=%q meta=%s", content, metaRaw)
	}
	if stored.Meta.TrustTier != contextpolicy.TrustTierUntrusted || stored.Meta.Source != "document_picker" || stored.Meta.SHA256 == "" {
		t.Fatalf("unexpected metadata: %#v", stored.Meta)
	}
}

func TestHandleContextSetRejectsEmptyAndOversized(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := &App{store: store}

	for _, body := range []string{
		`{"content":"   "}`,
		`{"content":"` + strings.Repeat("x", contextpolicy.MaxStagedContextBytes+1) + `"}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/context/set", strings.NewReader(body))
		rec := httptest.NewRecorder()
		app.handleContextSet(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected bad request for body length %d, got %d", len(body), rec.Code)
		}
	}
}

func TestTaskCurrentAndStateExposeAttemptID(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	task := runtime.TaskSnapshot{
		TaskID:    "task-current-attempt",
		AttemptID: "attempt-current-attempt",
		Status:    "running",
		Prompt:    "Check status",
	}
	if err := store.UpsertTask("active", task); err != nil {
		t.Fatalf("upsert task: %v", err)
	}
	app := &App{store: store, engine: tasks.New(store, nil, nil, "")}

	currentReq := httptest.NewRequest(http.MethodGet, "/task/current", http.NoBody)
	currentRec := httptest.NewRecorder()
	app.handleTaskCurrent(currentRec, currentReq)
	if currentRec.Code != http.StatusOK {
		t.Fatalf("expected current status %d, got %d body=%s", http.StatusOK, currentRec.Code, currentRec.Body.String())
	}
	var current map[string]runtime.TaskSnapshot
	if err := json.Unmarshal(currentRec.Body.Bytes(), &current); err != nil {
		t.Fatalf("decode current response: %v", err)
	}
	if current["task"].AttemptID != "attempt-current-attempt" {
		t.Fatalf("expected /task/current attempt id, got %#v", current)
	}

	stateReq := httptest.NewRequest(http.MethodGet, "/state", http.NoBody)
	stateRec := httptest.NewRecorder()
	app.handleState(stateRec, stateReq)
	if stateRec.Code != http.StatusOK {
		t.Fatalf("expected state status %d, got %d body=%s", http.StatusOK, stateRec.Code, stateRec.Body.String())
	}
	var state runtime.RuntimeState
	if err := json.Unmarshal(stateRec.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode state response: %v", err)
	}
	if state.ActiveTask == nil || state.ActiveTask.AttemptID != "attempt-current-attempt" {
		t.Fatalf("expected /state active task attempt id, got %#v", state.ActiveTask)
	}
}

func TestHandleAppBridgeEvidenceAcceptsValidActiveTaskEvidence(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	task := runtime.TaskSnapshot{
		TaskID:           "task-bridge-1",
		AttemptID:        "attempt-bridge-1",
		Prompt:           "Check the phone runtime state",
		Status:           "active",
		EvidenceBoundary: &runtime.EvidenceBoundary{},
	}
	if err := store.UpsertTask("active", task); err != nil {
		t.Fatalf("upsert active task: %v", err)
	}
	app := &App{store: store, engine: tasks.New(store, nil, nil, "")}

	body := `{
		"proof_class":"app_bridge_verified",
		"verified":true,
		"source":"android_bridge",
		"operation":"runtime_host_status",
		"checked_at":"` + time.Now().UTC().Format(time.RFC3339) + `",
		"summary":"Runtime host status was observed by the Android bridge.",
		"task_id":"task-bridge-1",
		"attempt_id":"attempt-bridge-1",
		"details":{"host_ready":true,"sidecar_process_alive":true}
	}`
	req := httptest.NewRequest(http.MethodPost, "/runtime/evidence/app-bridge", strings.NewReader(body))
	rec := httptest.NewRecorder()

	app.handleAppBridgeEvidence(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	stored, err := store.GetTask("active")
	if err != nil {
		t.Fatalf("get active task: %v", err)
	}
	if stored == nil || stored.EvidenceBoundary == nil || len(stored.EvidenceBoundary.AppBridgeEvidence) != 1 {
		t.Fatalf("expected stored app bridge evidence, got %#v", stored)
	}
	if stored.EvidenceBoundary.AppBridgeEvidence[0].TaskID != "task-bridge-1" || stored.EvidenceBoundary.AppBridgeEvidence[0].AttemptID != "attempt-bridge-1" {
		t.Fatalf("evidence must be attached to active task: %#v", stored.EvidenceBoundary.AppBridgeEvidence[0])
	}
}

func TestHandleAppBridgeEvidenceRejectsInvalidEvidence(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	if err := store.UpsertTask("active", runtime.TaskSnapshot{TaskID: "task-bridge-2", AttemptID: "attempt-bridge-2", Status: "active"}); err != nil {
		t.Fatalf("upsert active task: %v", err)
	}
	app := &App{store: store, engine: tasks.New(store, nil, nil, "")}

	fresh := time.Now().UTC().Format(time.RFC3339)
	stale := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)
	tests := []struct {
		name string
		body string
	}{
		{
			name: "wrong proof class",
			body: `{"proof_class":"model_text_only","verified":true,"source":"android_bridge","operation":"runtime_host_status","checked_at":"` + fresh + `","summary":"observed","task_id":"task-bridge-2","attempt_id":"attempt-bridge-2"}`,
		},
		{
			name: "unknown operation",
			body: `{"proof_class":"app_bridge_verified","verified":true,"source":"android_bridge","operation":"settings_intent_opened","checked_at":"` + fresh + `","summary":"settings opened","task_id":"task-bridge-2","attempt_id":"attempt-bridge-2"}`,
		},
		{
			name: "stale evidence",
			body: `{"proof_class":"app_bridge_verified","verified":true,"source":"android_bridge","operation":"runtime_host_status","checked_at":"` + stale + `","summary":"observed","task_id":"task-bridge-2","attempt_id":"attempt-bridge-2"}`,
		},
		{
			name: "failed without blocking reason",
			body: `{"proof_class":"app_bridge_verified","verified":false,"source":"android_bridge","operation":"runtime_host_status","checked_at":"` + fresh + `","summary":"observed","task_id":"task-bridge-2","attempt_id":"attempt-bridge-2"}`,
		},
		{
			name: "secret detail",
			body: `{"proof_class":"app_bridge_verified","verified":true,"source":"android_bridge","operation":"byok_key_storage","checked_at":"` + fresh + `","summary":"observed","task_id":"task-bridge-2","attempt_id":"attempt-bridge-2","details":{"api_key":"sk-secret"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/runtime/evidence/app-bridge", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			app.handleAppBridgeEvidence(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("expected %d, got %d body=%s", http.StatusBadRequest, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleAppBridgeEvidenceRequiresActiveTaskAssociation(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := &App{store: store, engine: tasks.New(store, nil, nil, "")}

	body := `{"proof_class":"app_bridge_verified","verified":true,"source":"android_bridge","operation":"runtime_host_status","checked_at":"` + time.Now().UTC().Format(time.RFC3339) + `","summary":"Runtime host status was observed."}`
	req := httptest.NewRequest(http.MethodPost, "/runtime/evidence/app-bridge", strings.NewReader(body))
	rec := httptest.NewRecorder()

	app.handleAppBridgeEvidence(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected %d, got %d body=%s", http.StatusConflict, rec.Code, rec.Body.String())
	}
}

func TestHandleAppBridgeEvidenceRejectsMissingAndMismatchedCorrelation(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	if err := store.UpsertTask("active", runtime.TaskSnapshot{TaskID: "task-bridge-3", AttemptID: "attempt-bridge-3", Status: "active"}); err != nil {
		t.Fatalf("upsert active task: %v", err)
	}
	app := &App{store: store, engine: tasks.New(store, nil, nil, "")}
	fresh := time.Now().UTC().Format(time.RFC3339)

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "missing task id",
			body: `{"proof_class":"app_bridge_verified","verified":true,"source":"android_bridge","operation":"runtime_host_status","checked_at":"` + fresh + `","summary":"observed","attempt_id":"attempt-bridge-3"}`,
			want: "app_bridge_evidence_missing_task_id",
		},
		{
			name: "missing attempt id",
			body: `{"proof_class":"app_bridge_verified","verified":true,"source":"android_bridge","operation":"runtime_host_status","checked_at":"` + fresh + `","summary":"observed","task_id":"task-bridge-3"}`,
			want: "app_bridge_evidence_missing_attempt_id",
		},
		{
			name: "wrong task id",
			body: `{"proof_class":"app_bridge_verified","verified":true,"source":"android_bridge","operation":"runtime_host_status","checked_at":"` + fresh + `","summary":"observed","task_id":"task-other","attempt_id":"attempt-bridge-3"}`,
			want: "app_bridge_evidence_task_mismatch",
		},
		{
			name: "wrong attempt id",
			body: `{"proof_class":"app_bridge_verified","verified":true,"source":"android_bridge","operation":"runtime_host_status","checked_at":"` + fresh + `","summary":"observed","task_id":"task-bridge-3","attempt_id":"attempt-other"}`,
			want: "app_bridge_evidence_attempt_mismatch",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/runtime/evidence/app-bridge", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()

			app.handleAppBridgeEvidence(rec, req)

			if rec.Code != http.StatusConflict {
				t.Fatalf("expected %d, got %d body=%s", http.StatusConflict, rec.Code, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tt.want) {
				t.Fatalf("expected %q in response, got %s", tt.want, rec.Body.String())
			}
		})
	}
}

func timeNow() time.Time {
	return time.Now().UTC()
}
