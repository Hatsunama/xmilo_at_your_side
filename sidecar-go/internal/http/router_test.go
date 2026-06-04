package httpx

import (
	"encoding/base64"
	"encoding/json"
	"errors"
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
	"xmilo/sidecar-go/internal/relay"
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

func TestHandleReadyHostedRouteIgnoresSavedBYOKConfigWhenLLMModeRelay(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "provider.key")
	if err := os.WriteFile(keyPath, []byte("local-key"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	app := &App{
		cfg: config.Config{
			LLMMode:      "relay",
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
	if out["llm_mode"] != "relay" || out["relay_llm_turn_allowed"] != true || out["local_llm_turn_allowed"] != false {
		t.Fatalf("hosted route must stay active despite saved BYOK config: %#v", out)
	}
}

func TestHandleAuthCheckLocalBYOKIgnoresHostedSessionState(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "provider.key")
	if err := os.WriteFile(keyPath, []byte("local-key"), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	_ = store.SetRuntimeConfig("relay_session_jwt", "stale-hosted-session")
	app := &App{
		cfg: config.Config{
			LLMMode:      "local_byok",
			BYOKProvider: "openai",
			BYOKKeyFile:  keyPath,
		},
		store: store,
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/check", http.NoBody)
	rec := httptest.NewRecorder()
	app.handleAuthCheck(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, rec.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out["access_mode"] != "local_byok" || out["local_llm_turn_allowed"] != true || out["relay_llm_turn_allowed"] != false {
		t.Fatalf("hosted session state must not block BYOK route: %#v", out)
	}
}

func TestHandleAuthCheckBootstrapsFreshRelaySessionWithoutJWT(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	sessionJWT := testUnsignedJWT(t, map[string]any{
		"sub":                    "device-fresh",
		"entitled":               false,
		"access_mode":            "code_only",
		"access_code_only":       true,
		"trial_allowed":          false,
		"subscription_allowed":   false,
		"access_code_grant_days": 30,
		"verified_email":         "",
		"email_verified":         false,
		"two_factor_enabled":     false,
		"two_factor_ok":          false,
		"website_handoff_ready":  false,
	})
	var sessionStartCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/session/start" {
			t.Fatalf("unexpected relay path %s", r.URL.Path)
		}
		sessionStartCalls++
		writeJSON(w, http.StatusOK, map[string]any{
			"device_user_id": "device-fresh",
			"session_jwt":    sessionJWT,
			"expires_at":     time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		})
	}))
	defer server.Close()

	app := &App{
		cfg:   config.Config{RelayBaseURL: server.URL},
		store: store,
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/check", http.NoBody)
	rec := httptest.NewRecorder()

	app.handleAuthCheck(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if sessionStartCalls != 1 {
		t.Fatalf("expected one session bootstrap, got %d", sessionStartCalls)
	}
	storedJWT, _ := store.GetRuntimeConfig("relay_session_jwt")
	if storedJWT != sessionJWT {
		t.Fatalf("expected stored bootstrap jwt")
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out["ok"] != true || out["device_user_id"] != "device-fresh" || out["entitled"] != false {
		t.Fatalf("unexpected auth check response: %#v", out)
	}
}

func TestHandleAuthCheckBootstrapHTTPErrorReturnsSafeClass(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/session/start" {
			t.Fatalf("unexpected relay path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"raw relay body jwt token api_key user_note bundle_json should not return"}`))
	}))
	defer server.Close()

	app := &App{
		cfg:   config.Config{RelayBaseURL: server.URL},
		store: store,
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/check", http.NoBody)
	rec := httptest.NewRecorder()

	app.handleAuthCheck(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected %d, got %d body=%s", http.StatusBadGateway, rec.Code, rec.Body.String())
	}
	assertAuthCheckSafeFailure(t, rec.Body.Bytes(), authSessionRelayHTTPError)
}

func TestHandleAuthCheckBootstrapUnreachableReturnsSafeClass(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("relay should be closed before auth check")
	}))
	relayURL := server.URL
	server.Close()

	app := &App{
		cfg:   config.Config{RelayBaseURL: relayURL},
		store: store,
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/check", http.NoBody)
	rec := httptest.NewRecorder()

	app.handleAuthCheck(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected %d, got %d body=%s", http.StatusBadGateway, rec.Code, rec.Body.String())
	}
	assertAuthCheckSafeFailure(t, rec.Body.Bytes(), authSessionRelayUnreachable)
}

func TestHandleAuthCheckBootstrapBadResponseReturnsSafeClass(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{not-json jwt token api_key}`))
	}))
	defer server.Close()

	app := &App{
		cfg:   config.Config{RelayBaseURL: server.URL},
		store: store,
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/check", http.NoBody)
	rec := httptest.NewRecorder()

	app.handleAuthCheck(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected %d, got %d body=%s", http.StatusBadGateway, rec.Code, rec.Body.String())
	}
	assertAuthCheckSafeFailure(t, rec.Body.Bytes(), authSessionBadResponse)
}

func TestHandleAuthCheckBootstrapEmptyJWTReturnsSafeClass(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"device_user_id": "device-empty",
			"session_jwt":    "",
			"expires_at":     time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		})
	}))
	defer server.Close()

	app := &App{
		cfg:   config.Config{RelayBaseURL: server.URL},
		store: store,
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/check", http.NoBody)
	rec := httptest.NewRecorder()

	app.handleAuthCheck(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected %d, got %d body=%s", http.StatusBadGateway, rec.Code, rec.Body.String())
	}
	assertAuthCheckSafeFailure(t, rec.Body.Bytes(), authSessionEmptyJWT)
}

func TestNewAppDoesNotBootstrapRelayBeforeHTTPStart(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "unexpected relay call", http.StatusInternalServerError)
	}))
	defer server.Close()

	app, err := NewApp(config.Config{
		DBPath:       filepath.Join(t.TempDir(), "sidecar.db"),
		MindRoot:     testMindRoot(t),
		RelayBaseURL: server.URL,
		RuntimeID:    "test-runtime",
	})
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	defer app.store.Close()
	if called {
		t.Fatal("NewApp called relay before HTTP start")
	}
}

func TestSidecarHTTPStartupProofLinesAreSafeAndStable(t *testing.T) {
	line := sidecarHTTPStartupProofLine(
		"XMILO_SIDECAR_HTTP_LISTENER_BOUND",
		"host",
		"127.0.0.1",
		"port",
		"42817",
		"address",
		"127.0.0.1:42817",
		"bad key",
		"value with spaces",
	)
	if line != "XMILO_SIDECAR_HTTP_LISTENER_BOUND host=127.0.0.1 port=42817 address=127.0.0.1:42817 badkey=valuewithspaces" {
		t.Fatalf("unexpected proof line: %q", line)
	}
}

func TestSidecarHTTPStartupErrorClassesAreSafe(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{nil, "none"},
		{http.ErrServerClosed, "server_closed"},
		{errors.New("listen tcp 127.0.0.1:42817: bind: address already in use"), "address_in_use"},
		{errors.New("listen tcp 127.0.0.1:42817: bind: permission denied"), "permission_denied"},
		{errors.New("some private startup failure"), "other"},
	}
	for _, tc := range cases {
		if got := classifySidecarHTTPStartupError(tc.err); got != tc.want {
			t.Fatalf("classifySidecarHTTPStartupError(%v)=%q want %q", tc.err, got, tc.want)
		}
	}
}

func TestHandleHealthDoesNotRequireRelaySessionOrProvider(t *testing.T) {
	app := &App{
		cfg: config.Config{
			RelayBaseURL: "http://127.0.0.1:1",
			LLMMode:      "local_byok",
			BYOKKeyFile:  filepath.Join(t.TempDir(), "missing.key"),
		},
		startedAt: time.Now().UTC(),
	}
	req := httptest.NewRequest(http.MethodGet, "/health", http.NoBody)
	rec := httptest.NewRecorder()

	app.handleHealth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out["ok"] != true || out["service"] != "xmilo-sidecar" {
		t.Fatalf("unexpected health response: %#v", out)
	}
}

func TestHandleSettingsReportFailsWithoutRelaySessionProof(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "relay unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	app := &App{
		cfg:         config.Config{RelayBaseURL: server.URL, RuntimeID: "test-runtime"},
		store:       store,
		relayClient: relay.New(server.URL, func() (string, error) { return store.GetRuntimeConfig("relay_session_jwt") }),
	}
	req := httptest.NewRequest(http.MethodPost, "/report/settings", strings.NewReader(`{"client_report_id":"client-1","bundle_schema_version":1,"bundle":{"summary":{}}}`))
	rec := httptest.NewRecorder()

	app.handleSettingsReport(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected %d, got %d body=%s", http.StatusBadGateway, rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out["accepted"] == true || out["report_id"] != nil {
		t.Fatalf("settings report failure must not invent proof: %#v", out)
	}
}

func TestHandleSettingsReportBootstrapsSessionAndRequiresRelayProof(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	sessionJWT := testUnsignedJWT(t, map[string]any{"sub": "device-report", "entitled": false})
	var sawSessionStart bool
	var sawSettingsReport bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/session/start":
			sawSessionStart = true
			writeJSON(w, http.StatusOK, map[string]any{
				"device_user_id": "device-report",
				"session_jwt":    sessionJWT,
				"expires_at":     time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
			})
		case "/report/settings":
			sawSettingsReport = true
			if r.Header.Get("Authorization") != "Bearer "+sessionJWT {
				t.Fatalf("settings report missing bootstrapped relay auth")
			}
			writeJSON(w, http.StatusAccepted, map[string]any{
				"accepted":         true,
				"report_id":        "settings-report-1",
				"status":           "new",
				"received_at":      time.Now().UTC().Format(time.RFC3339),
				"client_report_id": "client-1",
				"bundle_hash":      "hash-1",
				"duplicate":        false,
			})
		default:
			t.Fatalf("unexpected relay path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	app := &App{
		cfg:         config.Config{RelayBaseURL: server.URL, RuntimeID: "test-runtime"},
		store:       store,
		relayClient: relay.New(server.URL, func() (string, error) { return store.GetRuntimeConfig("relay_session_jwt") }),
	}
	req := httptest.NewRequest(http.MethodPost, "/report/settings", strings.NewReader(`{"client_report_id":"client-1","bundle_schema_version":1,"bundle":{"summary":{}}}`))
	rec := httptest.NewRecorder()

	app.handleSettingsReport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if !sawSessionStart || !sawSettingsReport {
		t.Fatalf("expected session bootstrap and settings report forwarding, got start=%v report=%v", sawSessionStart, sawSettingsReport)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out["accepted"] != true || out["report_id"] != "settings-report-1" {
		t.Fatalf("settings report must preserve relay proof, got %#v", out)
	}
}

func TestHandleAIReportStillSubmitsFlatReport(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if gotPath != "/report/ai" {
			t.Fatalf("unexpected path %s", gotPath)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}))
	defer server.Close()

	app := &App{
		cfg:         config.Config{RuntimeID: "test-runtime"},
		relayClient: relay.New(server.URL, func() (string, error) { return "relay-jwt", nil }),
	}
	req := httptest.NewRequest(http.MethodPost, "/report/ai", strings.NewReader(`{"task_id":"task-1","event_type":"ai_output","report_reason":"bad","output_text":"flat output"}`))
	rec := httptest.NewRecorder()

	app.handleAIReport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	if gotPath != "/report/ai" {
		t.Fatalf("expected /report/ai, got %s", gotPath)
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

func testMindRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"IDENTITY.md", "SOUL.md", "SECURITY.md", "TOOLS.md", "USER.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("test"), 0o600); err != nil {
			t.Fatalf("write mind file: %v", err)
		}
	}
	return root
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

func TestHandleContextSetRedactsSecretBearingExternalContext(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := &App{store: store}

	req := httptest.NewRequest(http.MethodPost, "/context/set", strings.NewReader(`{"content":"external context has Authorization: Bearer user-provided-token and api_key=sk-user-provided-value","source":"document_picker"}`))
	rec := httptest.NewRecorder()

	app.handleContextSet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d body=%s", http.StatusOK, rec.Code, rec.Body.String())
	}
	content, _ := store.GetRuntimeConfig("active_context")
	for _, forbidden := range []string{"Authorization: Bearer user-provided-token", "api_key=sk-user-provided-value", "user-provided-token", "sk-user-provided-value"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("active context leaked %q: %s", forbidden, content)
		}
	}
	if !strings.Contains(content, "[REDACTED_SECRET]") || !strings.Contains(content, "external context has") {
		t.Fatalf("expected useful redacted active context, got %q", content)
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

func TestPublicTaskSnapshotRedactsImmediateResponsePrompt(t *testing.T) {
	task := &runtime.TaskSnapshot{
		TaskID:    "task-public-redaction",
		AttemptID: "attempt-public-redaction",
		Status:    "running",
		Prompt:    "Format config with api_key=sk-user-provided-value",
	}
	public := publicTaskSnapshot(task)
	if public == nil {
		t.Fatal("expected public task")
	}
	if task.Prompt == public.Prompt {
		t.Fatalf("expected public prompt projection to redact")
	}
	if strings.Contains(public.Prompt, "sk-user-provided-value") || strings.Contains(public.Prompt, "api_key=sk-user-provided-value") {
		t.Fatalf("public prompt leaked secret: %q", public.Prompt)
	}
	if task.Prompt != "Format config with api_key=sk-user-provided-value" {
		t.Fatalf("projection mutated active in-memory task prompt: %q", task.Prompt)
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
		Prompt:    "Check status with api_key=sk-user-provided-value",
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
	if strings.Contains(current["task"].Prompt, "sk-user-provided-value") || strings.Contains(current["task"].Prompt, "api_key=sk-user-provided-value") {
		t.Fatalf("/task/current leaked raw prompt secret: %#v", current["task"].Prompt)
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
	if strings.Contains(state.ActiveTask.Prompt, "sk-user-provided-value") || strings.Contains(state.ActiveTask.Prompt, "api_key=sk-user-provided-value") {
		t.Fatalf("/state leaked raw prompt secret: %#v", state.ActiveTask.Prompt)
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

func assertAuthCheckSafeFailure(t *testing.T, body []byte, wantClass string) {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if out["error"] != "auth_check_failed" {
		t.Fatalf("expected safe error, got %#v", out)
	}
	if out["error_class"] != wantClass {
		t.Fatalf("expected class %s, got %#v", wantClass, out)
	}
	if out["entitled"] != false {
		t.Fatalf("expected entitled false, got %#v", out)
	}
	text := string(body)
	for _, forbidden := range []string{"raw relay body", "api_key", "user_note", "bundle_json", "Bearer ", "Authorization"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("response leaked forbidden text %q in %s", forbidden, text)
		}
	}
}

func TestMemoryRoutesRequireBearer(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	app := &App{cfg: config.Config{BearerToken: "test-token"}, store: store}
	mux := http.NewServeMux()
	mux.Handle("/memory", RequireBearer(app.cfg.BearerToken, http.HandlerFunc(app.handleMemoryRoot)))

	req := httptest.NewRequest(http.MethodGet, "/memory", http.NoBody)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected bearer auth failure, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/memory", http.NoBody)
	req.Header.Set("Authorization", "Bearer test-token")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected authorized memory route, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleMemoryRoutesExposeSafeProjectionAndActions(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	entry := testHTTPMemoryEntry("memory.http", "durable_user_preference")
	if err := store.UpsertMemoryEntry(entry); err != nil {
		t.Fatalf("upsert memory: %v", err)
	}
	if err := store.AppendMemoryEvidenceRef(db.MemoryEvidenceRef{
		EvidenceID:       "evidence.http",
		MemoryID:         entry.MemoryID,
		SourceType:       "direct_user",
		SourceID:         "user",
		SourceRef:        "user safe note",
		EvidenceKind:     "user_statement",
		TrustTier:        2,
		AuthorityRank:    "rank_300_direct_user",
		DisplayAllowed:   true,
		PromotionAllowed: true,
	}); err != nil {
		t.Fatalf("append evidence: %v", err)
	}
	app := &App{store: store}

	req := httptest.NewRequest(http.MethodGet, "/memory", http.NoBody)
	rec := httptest.NewRecorder()
	app.handleMemoryRoot(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected memory list ok, got %d body=%s", rec.Code, rec.Body.String())
	}
	text := rec.Body.String()
	for _, want := range []string{"memory.http", "correct_supersede", "view_provenance"} {
		if !strings.Contains(text, want) {
			t.Fatalf("memory list missing %q in %s", want, text)
		}
	}
	if strings.Contains(text, "api_key") || strings.Contains(text, "Bearer ") {
		t.Fatalf("memory list leaked forbidden text: %s", text)
	}

	req = httptest.NewRequest(http.MethodGet, "/memory/memory.http/provenance", http.NoBody)
	rec = httptest.NewRecorder()
	app.handleMemoryPath(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected provenance ok, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "evidence.http") || !strings.Contains(rec.Body.String(), "audit_id") {
		t.Fatalf("provenance did not include evidence and audit proof: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/memory/memory.http/suppress", strings.NewReader(`{"reason":"hide this"}`))
	rec = httptest.NewRecorder()
	app.handleMemoryPath(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected suppress ok, got %d body=%s", rec.Code, rec.Body.String())
	}
	loaded, err := store.GetMemoryEntry("memory.http")
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}
	if loaded.Status != "suppressed" || loaded.RetrievalEligible {
		t.Fatalf("suppress did not disable retrieval: %#v", loaded)
	}
}

func TestHandleMemoryRoutesBlockProtectedTruthAndDeferredActions(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	canon := testHTTPMemoryEntry("memory.canon.http", "canon_memory")
	canon.SourceType = "canon"
	canon.AuthorityRank = "rank_000_canon"
	canon.Provenance = map[string]any{"source_type": "canon"}
	if err := store.UpsertMemoryEntry(canon); err != nil {
		t.Fatalf("upsert canon memory: %v", err)
	}
	summary := testHTTPMemoryEntry("memory.summary.http", "approved_summary")
	if err := store.UpsertMemoryEntry(summary); err != nil {
		t.Fatalf("upsert summary memory: %v", err)
	}
	candidate := testHTTPMemoryCandidate("candidate.http")
	if err := store.UpsertMemoryCandidate(candidate); err != nil {
		t.Fatalf("upsert candidate: %v", err)
	}
	app := &App{store: store}

	req := httptest.NewRequest(http.MethodPost, "/memory/memory.canon.http/suppress", strings.NewReader(`{"reason":"hide"}`))
	rec := httptest.NewRecorder()
	app.handleMemoryPath(rec, req)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "memory_canon_memory_cannot_modify") {
		t.Fatalf("canon mutation should be forbidden, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/memory/memory.summary.http/correct", strings.NewReader(`{"summary":"new"}`))
	rec = httptest.NewRecorder()
	app.handleMemoryPath(rec, req)
	if rec.Code != http.StatusNotImplemented || !strings.Contains(rec.Body.String(), "memory_approved_summary_correction_deferred") {
		t.Fatalf("approved summary correction should be deferred, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/memory/memory.summary.http/rollback", strings.NewReader(`{}`))
	rec = httptest.NewRecorder()
	app.handleMemoryPath(rec, req)
	if rec.Code != http.StatusNotImplemented || !strings.Contains(rec.Body.String(), "memory_rollback_deferred") {
		t.Fatalf("rollback should be deferred, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/memory/candidates", http.NoBody)
	rec = httptest.NewRecorder()
	app.handleMemoryPath(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected candidates ok, got %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "approve_candidate") {
		t.Fatalf("candidate approval leaked as supported action: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/memory/candidates/candidate.http/reject", strings.NewReader(`{"reason":"not useful"}`))
	rec = httptest.NewRecorder()
	app.handleMemoryPath(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected reject ok, got %d body=%s", rec.Code, rec.Body.String())
	}
	updated, err := store.GetMemoryCandidate("candidate.http")
	if err != nil {
		t.Fatalf("get candidate: %v", err)
	}
	if updated.Status != "rejected" {
		t.Fatalf("candidate not rejected: %#v", updated)
	}
}

func TestHandleMemoryRoutesRejectWrongMethodsAndUnknownFields(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "sidecar.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	entry := testHTTPMemoryEntry("memory.method", "durable_user_preference")
	if err := store.UpsertMemoryEntry(entry); err != nil {
		t.Fatalf("upsert memory: %v", err)
	}
	app := &App{store: store}

	req := httptest.NewRequest(http.MethodPost, "/memory", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	app.handleMemoryRoot(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected method not allowed, got %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/memory/memory.method/suppress", strings.NewReader(`{"reason":"hide","unknown":true}`))
	rec = httptest.NewRecorder()
	app.handleMemoryPath(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "memory_invalid_request") {
		t.Fatalf("expected strict JSON rejection, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func testHTTPMemoryEntry(memoryID, memoryClass string) db.MemoryEntry {
	return db.MemoryEntry{
		MemoryID:                        memoryID,
		MemoryClass:                     memoryClass,
		Status:                          "active",
		Title:                           "safe title",
		Summary:                         "safe summary",
		Content:                         "safe content",
		ContentExcerpt:                  "safe content",
		SourceType:                      "direct_user",
		SourceID:                        "user",
		TrustTier:                       2,
		AuthorityRank:                   "rank_300_direct_user",
		Provenance:                      map[string]any{"source_type": "direct_user", "source_id": "user"},
		EvidenceRefs:                    []string{"evidence.user"},
		FreshnessState:                  "fresh",
		Confidence:                      0.8,
		ContradictionState:              "none",
		QuarantineStatus:                "clean",
		SuppressionStatus:               "active",
		AllowedActions:                  []string{"view"},
		RollbackAvailable:               true,
		ExternalContentIsNotInstruction: true,
		RetrievalEligible:               true,
		RetrievalReason:                 "safe preference",
		EmbeddingStatus:                 "not_needed",
		UserVisible:                     true,
	}
}

func testHTTPMemoryCandidate(candidateID string) db.MemoryCandidate {
	return db.MemoryCandidate{
		CandidateID:        candidateID,
		CandidateType:      "memory_candidate",
		Status:             "generated",
		Title:              "candidate",
		Summary:            "candidate summary",
		Content:            "candidate content",
		SourceType:         "model_output",
		SourceID:           "turn_1",
		TrustTier:          6,
		AuthorityRank:      "rank_700_model_output",
		Provenance:         map[string]any{"source_type": "model_output", "turn_id": "turn_1"},
		EvidenceRefs:       []string{"evidence.turn_1"},
		FreshnessState:     "fresh",
		Confidence:         0.5,
		ContradictionState: "none",
		QuarantineStatus:   "clean",
		SuppressionStatus:  "active",
	}
}

func testUnsignedJWT(t *testing.T, claims map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	return "e30." + base64.RawURLEncoding.EncodeToString(raw) + ".sig"
}
