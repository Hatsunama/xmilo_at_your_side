package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"xmilo/sidecar-go/internal/config"
	"xmilo/sidecar-go/internal/providerpolicy"
	"xmilo/sidecar-go/shared/contracts"
)

func TestXAITurnUsesResponsesPathAndParsesResponse(t *testing.T) {
	server := providerServer(t, "/responses", "Bearer local-test-key", responsesPayload("xai done"), nil)
	defer server.Close()
	client := mustLocalClient(t, localConfig(t, "xai", server.URL, "grok-test", true))
	client.HTTP = server.Client()
	resp, err := client.Turn(context.Background(), contracts.RelayTurnRequest{TaskID: "task_1", Phase: "intake"})
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if resp.Summary != "xai done" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestOpenAITurnUsesDistinctResponsesPathAndParsesResponse(t *testing.T) {
	server := providerServer(t, "/responses", "Bearer local-test-key", responsesPayload("openai done"), nil)
	defer server.Close()
	client := mustLocalClient(t, localConfig(t, "openai", server.URL, "gpt-test", true))
	client.HTTP = server.Client()
	resp, err := client.Turn(context.Background(), contracts.RelayTurnRequest{TaskID: "task_1", Phase: "intake"})
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if resp.Summary != "openai done" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestAnthropicTurnUsesMessagesPathAndParsesResponse(t *testing.T) {
	server := providerServer(t, "/messages", "", anthropicPayload("anthropic done"), func(t *testing.T, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "local-test-key" {
			t.Fatalf("unexpected anthropic key header")
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Fatalf("unexpected anthropic version %q", got)
		}
	})
	defer server.Close()
	client := mustLocalClient(t, localConfig(t, "anthropic", server.URL, "claude-test", true))
	client.HTTP = server.Client()
	resp, err := client.Turn(context.Background(), contracts.RelayTurnRequest{TaskID: "task_1", Phase: "intake"})
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if resp.Summary != "anthropic done" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestOllamaTurnAllowsNoKeyAndParsesResponse(t *testing.T) {
	server := providerServer(t, "/api/generate", "", ollamaPayload("ollama done"), nil)
	defer server.Close()
	client := mustLocalClient(t, config.Config{
		LLMMode:      "local_byok",
		BYOKProvider: "ollama",
		BYOKBaseURL:  server.URL,
		BYOKModel:    "llama-test",
	})
	client.HTTP = server.Client()
	resp, err := client.Turn(context.Background(), contracts.RelayTurnRequest{TaskID: "task_1", Phase: "intake"})
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if resp.Summary != "ollama done" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestOllamaCloudTurnRequiresBearerAndUsesGeneratePath(t *testing.T) {
	server := providerServer(t, "/api/generate", "Bearer local-test-key", ollamaPayload("ollama cloud done"), nil)
	defer server.Close()
	client := mustLocalClient(t, localConfig(t, "ollama_cloud", server.URL, "gpt-oss:120b", true))
	client.HTTP = server.Client()
	resp, err := client.Turn(context.Background(), contracts.RelayTurnRequest{TaskID: "task_cloud_ollama", Phase: "intake"})
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if resp.Summary != "ollama cloud done" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}

func TestXAIAndOpenAIHaveDistinctDefaults(t *testing.T) {
	xai := config.Config{BYOKProvider: "xai"}
	xai.ApplyBYOKProviderDefaults()
	openai := config.Config{BYOKProvider: "openai"}
	openai.ApplyBYOKProviderDefaults()
	if xai.BYOKBaseURL == openai.BYOKBaseURL {
		t.Fatalf("xAI and OpenAI base URLs must remain distinct")
	}
	if xai.BYOKModel == openai.BYOKModel {
		t.Fatalf("xAI and OpenAI default models must remain distinct")
	}
	if xai.BYOKBaseURL != "https://api.x.ai/v1" || openai.BYOKBaseURL != "https://api.openai.com/v1" {
		t.Fatalf("unexpected defaults: xai=%#v openai=%#v", xai, openai)
	}
}

func TestOllamaLocalAndCloudHaveDistinctDefaults(t *testing.T) {
	local := config.Config{BYOKProvider: "ollama"}
	local.ApplyBYOKProviderDefaults()
	cloud := config.Config{BYOKProvider: "ollama_cloud"}
	cloud.ApplyBYOKProviderDefaults()
	if local.BYOKBaseURL != "" || local.BYOKModel != "llama3.2" {
		t.Fatalf("unexpected ollama local defaults: %#v", local)
	}
	if cloud.BYOKBaseURL != "https://ollama.com" || cloud.BYOKModel != "gpt-oss:120b" {
		t.Fatalf("unexpected ollama cloud defaults: %#v", cloud)
	}
}

func TestMissingRequiredKeyReturnsPreciseError(t *testing.T) {
	client := mustLocalClient(t, config.Config{
		LLMMode:      "local_byok",
		BYOKProvider: "openai",
		BYOKKeyFile:  filepath.Join(t.TempDir(), "missing.key"),
		BYOKBaseURL:  "https://example.invalid/v1",
		BYOKModel:    "gpt-test",
	})
	if _, err := client.Turn(context.Background(), contracts.RelayTurnRequest{}); err == nil || err.Error() != "missing_local_provider_key" {
		t.Fatalf("expected missing_local_provider_key, got %v", err)
	}
}

func TestOllamaCloudMissingKeyReturnsPreciseError(t *testing.T) {
	client := mustLocalClient(t, config.Config{
		LLMMode:      "local_byok",
		BYOKProvider: "ollama_cloud",
		BYOKBaseURL:  "https://ollama.com",
		BYOKModel:    "gpt-oss:120b",
	})
	if _, err := client.Turn(context.Background(), contracts.RelayTurnRequest{}); err == nil || err.Error() != "missing_local_provider_key" {
		t.Fatalf("expected missing_local_provider_key, got %v", err)
	}
}

func TestUnsupportedProviderReturnsPreciseError(t *testing.T) {
	if _, err := NewLocalProvider(config.Config{BYOKProvider: "mystery"}); err == nil || err.Error() != "local_provider_unavailable" {
		t.Fatalf("expected local_provider_unavailable, got %v", err)
	}
}

func TestProviderAuthFailureReturnsPreciseError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()
	t.Setenv(providerpolicy.DevAllowCloudProviderCustomBaseURLEnv, "1")
	client := mustLocalClient(t, localConfig(t, "xai", server.URL, "grok-test", true))
	client.HTTP = server.Client()
	if _, err := client.Turn(context.Background(), contracts.RelayTurnRequest{}); err == nil || err.Error() != "local_provider_auth_failed" {
		t.Fatalf("expected local_provider_auth_failed, got %v", err)
	}
}

func TestLocalProviderRereadsKeyFileAfterAuthFailure(t *testing.T) {
	var seen []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		seen = append(seen, auth)
		switch auth {
		case "Bearer bad":
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		case "Bearer good":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(responsesPayload("recovered")))
			return
		default:
			t.Fatalf("unexpected auth header")
		}
	}))
	defer server.Close()

	keyPath := filepath.Join(t.TempDir(), "provider.key")
	if err := os.WriteFile(keyPath, []byte("bad\n"), 0o600); err != nil {
		t.Fatalf("write bad key: %v", err)
	}

	t.Setenv(providerpolicy.DevAllowCloudProviderCustomBaseURLEnv, "1")
	client := mustLocalClient(t, config.Config{
		LLMMode:      "local_byok",
		BYOKProvider: "openai",
		BYOKKeyFile:  keyPath,
		BYOKBaseURL:  server.URL,
		BYOKModel:    "gpt-test",
	})
	client.HTTP = server.Client()

	if _, err := client.Turn(context.Background(), contracts.RelayTurnRequest{TaskID: "task_bad", Phase: "intake"}); err == nil || err.Error() != "local_provider_auth_failed" {
		t.Fatalf("expected auth failure with bad key, got %v", err)
	}
	if err := os.WriteFile(keyPath, []byte("good\n"), 0o600); err != nil {
		t.Fatalf("write good key: %v", err)
	}
	resp, err := client.Turn(context.Background(), contracts.RelayTurnRequest{TaskID: "task_good", Phase: "intake"})
	if err != nil {
		t.Fatalf("expected recovery with rewritten key: %v", err)
	}
	if resp.Summary != "recovered" {
		t.Fatalf("unexpected response after key rewrite: %#v", resp)
	}
	if len(seen) != 2 || seen[0] != "Bearer bad" || seen[1] != "Bearer good" {
		t.Fatalf("expected provider to see bad then good auth headers, got %#v", seen)
	}
}

func TestXAIProviderRereadsKeyFileAfterAuthFailure(t *testing.T) {
	testProviderRereadsKeyFileAfterAuthFailure(t, "xai", "/responses", "grok-test", func(summary string) string {
		return responsesPayload(summary)
	})
}

func TestAnthropicProviderRereadsKeyFileAfterAuthFailure(t *testing.T) {
	testProviderRereadsKeyFileAfterAuthFailure(t, "anthropic", "/messages", "claude-test", func(summary string) string {
		return anthropicPayload(summary)
	})
}

func TestOllamaUnreachableClearsAfterConfigRestart(t *testing.T) {
	client := mustLocalClient(t, config.Config{
		LLMMode:      "local_byok",
		BYOKProvider: "ollama",
		BYOKBaseURL:  "http://127.0.0.1:1",
		BYOKModel:    "llama-test",
	})
	if _, err := client.Turn(context.Background(), contracts.RelayTurnRequest{TaskID: "task_bad_ollama", Phase: "intake"}); err == nil || err.Error() != "local_provider_unreachable" {
		t.Fatalf("expected unreachable ollama before config fix, got %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Fatalf("unexpected ollama path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(ollamaPayload("ollama recovered")))
	}))
	defer server.Close()

	restarted := mustLocalClient(t, config.Config{
		LLMMode:      "local_byok",
		BYOKProvider: "ollama",
		BYOKBaseURL:  server.URL,
		BYOKModel:    "llama-test",
	})
	restarted.HTTP = server.Client()
	resp, err := restarted.Turn(context.Background(), contracts.RelayTurnRequest{TaskID: "task_good_ollama", Phase: "intake"})
	if err != nil {
		t.Fatalf("expected ollama recovery after config restart: %v", err)
	}
	if resp.Summary != "ollama recovered" {
		t.Fatalf("unexpected ollama recovery response: %#v", resp)
	}
}

func TestProviderSwitchingUsesCurrentConstructedProvider(t *testing.T) {
	steps := []struct {
		provider string
		path     string
		model    string
		payload  string
	}{
		{provider: "openai", path: "/responses", model: "gpt-test", payload: responsesPayload("openai first")},
		{provider: "xai", path: "/responses", model: "grok-test", payload: responsesPayload("xai next")},
		{provider: "openai", path: "/responses", model: "gpt-test", payload: responsesPayload("openai return")},
		{provider: "anthropic", path: "/messages", model: "claude-test", payload: anthropicPayload("anthropic next")},
		{provider: "xai", path: "/responses", model: "grok-test", payload: responsesPayload("xai return")},
	}
	for _, step := range steps {
		t.Run(step.provider+"_"+step.model, func(t *testing.T) {
			wantAuth := "Bearer local-test-key"
			if step.provider == "anthropic" {
				wantAuth = ""
			}
			server := providerServer(t, step.path, wantAuth, step.payload, func(t *testing.T, r *http.Request) {
				if step.provider == "anthropic" && r.Header.Get("x-api-key") != "local-test-key" {
					t.Fatalf("anthropic switch did not use the current provider key header")
				}
			})
			defer server.Close()
			client := mustLocalClient(t, localConfig(t, step.provider, server.URL, step.model, true))
			client.HTTP = server.Client()
			resp, err := client.Turn(context.Background(), contracts.RelayTurnRequest{TaskID: "task_switch", Phase: "intake"})
			if err != nil {
				t.Fatalf("turn after provider switch: %v", err)
			}
			if resp.Summary == "" {
				t.Fatalf("expected provider response after switch")
			}
		})
	}
}

func TestAnthropicOllamaAnthropicSwitchingUsesCurrentConstructedProvider(t *testing.T) {
	anthropicOne := providerServer(t, "/messages", "", anthropicPayload("anthropic first"), func(t *testing.T, r *http.Request) {
		if r.Header.Get("x-api-key") != "local-test-key" {
			t.Fatalf("unexpected anthropic key header")
		}
	})
	defer anthropicOne.Close()
	client := mustLocalClient(t, localConfig(t, "anthropic", anthropicOne.URL, "claude-test", true))
	client.HTTP = anthropicOne.Client()
	if resp, err := client.Turn(context.Background(), contracts.RelayTurnRequest{TaskID: "task_anthropic_1", Phase: "intake"}); err != nil || resp.Summary != "anthropic first" {
		t.Fatalf("anthropic first failed resp=%#v err=%v", resp, err)
	}

	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Fatalf("unexpected ollama path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(ollamaPayload("ollama middle")))
	}))
	defer ollama.Close()
	ollamaClient := mustLocalClient(t, config.Config{LLMMode: "local_byok", BYOKProvider: "ollama", BYOKBaseURL: ollama.URL, BYOKModel: "llama-test"})
	ollamaClient.HTTP = ollama.Client()
	if resp, err := ollamaClient.Turn(context.Background(), contracts.RelayTurnRequest{TaskID: "task_ollama", Phase: "intake"}); err != nil || resp.Summary != "ollama middle" {
		t.Fatalf("ollama middle failed resp=%#v err=%v", resp, err)
	}

	anthropicTwo := providerServer(t, "/messages", "", anthropicPayload("anthropic return"), func(t *testing.T, r *http.Request) {
		if r.Header.Get("x-api-key") != "local-test-key" {
			t.Fatalf("unexpected anthropic return key header")
		}
	})
	defer anthropicTwo.Close()
	returnClient := mustLocalClient(t, localConfig(t, "anthropic", anthropicTwo.URL, "claude-test", true))
	returnClient.HTTP = anthropicTwo.Client()
	if resp, err := returnClient.Turn(context.Background(), contracts.RelayTurnRequest{TaskID: "task_anthropic_2", Phase: "intake"}); err != nil || resp.Summary != "anthropic return" {
		t.Fatalf("anthropic return failed resp=%#v err=%v", resp, err)
	}
}

func TestOpenAIHTTPErrorMappingIsNotCollapsedToUnreachable(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantCode   string
	}{
		{name: "unauthorized", statusCode: http.StatusUnauthorized, wantCode: "local_provider_auth_failed"},
		{name: "forbidden", statusCode: http.StatusForbidden, wantCode: "local_provider_auth_failed"},
		{name: "rate_limited", statusCode: http.StatusTooManyRequests, wantCode: "local_provider_rate_limited"},
		{name: "bad_request", statusCode: http.StatusBadRequest, wantCode: "local_provider_request_failed"},
		{name: "not_found", statusCode: http.StatusNotFound, wantCode: "local_provider_request_failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/responses" {
					t.Fatalf("unexpected path %s", r.URL.Path)
				}
				http.Error(w, "provider error", test.statusCode)
			}))
			defer server.Close()

			t.Setenv(providerpolicy.DevAllowCloudProviderCustomBaseURLEnv, "1")
			client := mustLocalClient(t, localConfig(t, "openai", server.URL, "gpt-test", true))
			client.HTTP = server.Client()
			_, err := client.Turn(context.Background(), contracts.RelayTurnRequest{})
			if err == nil || err.Error() != test.wantCode {
				t.Fatalf("expected %s, got %v", test.wantCode, err)
			}
			if err.Error() == "local_provider_unreachable" {
				t.Fatalf("HTTP %d must not be reported as unreachable", test.statusCode)
			}
			diag := SafeDiagnostic(err)
			if diag["provider"] != "openai" || diag["endpoint_path"] != "/responses" || diag["http_status"] != test.statusCode {
				t.Fatalf("unexpected safe diagnostic: %#v", diag)
			}
		})
	}
}

func TestProviderUnreachableReturnsPreciseError(t *testing.T) {
	client := mustLocalClient(t, localConfig(t, "xai", "http://127.0.0.1:1", "grok-test", true))
	if _, err := client.Turn(context.Background(), contracts.RelayTurnRequest{}); err == nil || err.Error() != "local_provider_disallowed_url_scheme" {
		t.Fatalf("expected local_provider_disallowed_url_scheme, got %v", err)
	}
}

func TestOpenAIUnreachableIncludesSafeDiagnostic(t *testing.T) {
	t.Setenv(providerpolicy.DevAllowCloudProviderCustomBaseURLEnv, "1")
	client := mustLocalClient(t, localConfig(t, "openai", "https://127.0.0.1:1", "gpt-test", true))
	_, err := client.Turn(context.Background(), contracts.RelayTurnRequest{})
	if err == nil || err.Error() != "local_provider_unreachable" {
		t.Fatalf("expected local_provider_unreachable, got %v", err)
	}
	diag := SafeDiagnostic(err)
	if diag["provider"] != "openai" || diag["endpoint_path"] != "/responses" || diag["base_url_host"] != "127.0.0.1:1" {
		t.Fatalf("unexpected safe diagnostic: %#v", diag)
	}
	if _, ok := diag["network_class"]; !ok {
		t.Fatalf("expected safe network class, got %#v", diag)
	}
}

func TestOpenAIMalformedBaseURLIsUnavailableNotUnreachable(t *testing.T) {
	client := mustLocalClient(t, localConfig(t, "openai", "api.openai.com/v1", "gpt-test", true))
	_, err := client.Turn(context.Background(), contracts.RelayTurnRequest{})
	if err == nil || err.Error() != "local_provider_unavailable" {
		t.Fatalf("expected local_provider_unavailable, got %v", err)
	}
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Provider != "openai" || providerErr.EndpointPath != "/responses" {
		t.Fatalf("expected safe provider diagnostic, got %#v", err)
	}
}

func TestLocalProviderUsesResilientHTTPClient(t *testing.T) {
	client := mustLocalClient(t, localConfig(t, "openai", "https://api.openai.com/v1", "gpt-test", true))
	if client.HTTP == nil {
		t.Fatal("expected HTTP client")
	}
	if client.HTTP.Transport == nil {
		t.Fatal("local provider must use configured transport for Android DNS resilience")
	}
}

func TestLocalProviderReadyRequiresGuidedOllamaConnectionTarget(t *testing.T) {
	if LocalProviderReady(config.Config{LLMMode: "local_byok", BYOKProvider: "ollama", BYOKModel: "llama3.2"}) {
		t.Fatalf("blank ollama must require guided connection target before readiness")
	}
	if !LocalProviderReady(config.Config{LLMMode: "local_byok", BYOKProvider: "ollama", BYOKBaseURL: "http://192.168.1.10:11434", BYOKModel: "llama3.2"}) {
		t.Fatalf("ollama with explicit base URL and model should be locally eligible without a key")
	}
}

func TestOllamaManualURLInvalidUsesTypedReason(t *testing.T) {
	client := mustLocalClient(t, config.Config{LLMMode: "local_byok", BYOKProvider: "ollama", BYOKBaseURL: "localhost:11434", BYOKModel: "llama3.2"})
	if _, err := client.Turn(context.Background(), contracts.RelayTurnRequest{}); err == nil || err.Error() != providerpolicy.ReasonManualURLInvalid {
		t.Fatalf("expected %s, got %v", providerpolicy.ReasonManualURLInvalid, err)
	}
}

func TestParsePlannerResponseTextAcceptsStrictJSONAndFencedJSON(t *testing.T) {
	raw := turnJSON("strict json")
	for _, text := range []string{
		raw,
		"```json\n" + raw + "\n```",
		"```\n" + raw + "\n```",
	} {
		resp, err := parsePlannerResponseText(text)
		if err != nil {
			t.Fatalf("expected valid planner JSON to parse: %v", err)
		}
		if resp.Summary != "strict json" {
			t.Fatalf("unexpected parsed response: %#v", resp)
		}
	}
}

func TestParsePlannerResponseTextRejectsFreeformMalformedAndUnsafeShape(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "freeform", text: "Sure, I can help with that."},
		{name: "embedded json with prose", text: "Here is the JSON:\n" + turnJSON("embedded")},
		{name: "malformed json", text: `{"summary":`},
		{name: "invalid completion shape", text: strings.Replace(turnJSON("bad completion"), `"completion_status":"completed"`, `"completion_status":"done"`, 1)},
		{name: "missing required key", text: strings.Replace(turnJSON("missing key"), `"next_blocker":"",`, "", 1)},
		{name: "extra key", text: strings.Replace(turnJSON("extra key"), `"choices":[]`, `"choices":[],"unexpected":true`, 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parsePlannerResponseText(tt.text); err == nil || err.Error() != "local_provider_invalid_planner_response" {
				t.Fatalf("expected safe invalid planner response error, got %v", err)
			}
		})
	}
}

func TestLocalBYOKPromptContainsStrictJSONContract(t *testing.T) {
	prompt := buildProviderPrompt(contracts.RelayTurnRequest{Phase: "intake", Prompt: "hey"}, "openai")
	for _, needle := range []string{
		"Output must be a single JSON object, with no markdown fences, no prose, and no text before or after it.",
		"Required JSON shape for a simple informational answer:",
		`"completion_status":"completed"`,
		`"action_payload":{}`,
		`"expected_check":null`,
	} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("expected local BYOK prompt to contain %q", needle)
		}
	}
}

func mustLocalClient(t *testing.T, cfg config.Config) *LocalClient {
	t.Helper()
	client, err := NewLocalProvider(cfg)
	if err != nil {
		t.Fatalf("new local provider: %v", err)
	}
	return client
}

func localConfig(t *testing.T, provider, baseURL, model string, writeKey bool) config.Config {
	t.Helper()
	cfg := config.Config{
		LLMMode:      "local_byok",
		BYOKProvider: provider,
		BYOKBaseURL:  baseURL,
		BYOKModel:    model,
	}
	if writeKey {
		keyPath := filepath.Join(t.TempDir(), "provider.key")
		if err := os.WriteFile(keyPath, []byte("local-test-key\n"), 0o600); err != nil {
			t.Fatalf("write key: %v", err)
		}
		cfg.BYOKKeyFile = keyPath
	}
	return cfg
}

func providerServer(t *testing.T, wantPath, wantAuth, payload string, extra func(*testing.T, *http.Request)) *httptest.Server {
	t.Helper()
	t.Setenv(providerpolicy.DevAllowCloudProviderCustomBaseURLEnv, "1")
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if wantAuth != "" && r.Header.Get("Authorization") != wantAuth {
			t.Fatalf("unexpected auth header")
		}
		if extra != nil {
			extra(t, r)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode provider request: %v", err)
		}
		if strings.TrimSpace(body["model"].(string)) == "" {
			t.Fatalf("expected model in request")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
}

func testProviderRereadsKeyFileAfterAuthFailure(t *testing.T, provider, wantPath, model string, payload func(string) string) {
	t.Helper()
	var seen []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wantPath {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		auth := r.Header.Get("Authorization")
		if provider == "anthropic" {
			auth = "Bearer " + r.Header.Get("x-api-key")
		}
		seen = append(seen, auth)
		switch auth {
		case "Bearer bad":
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		case "Bearer good":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(payload(provider + " recovered")))
			return
		default:
			t.Fatalf("unexpected auth header for %s", provider)
		}
	}))
	defer server.Close()

	keyPath := filepath.Join(t.TempDir(), "provider.key")
	if err := os.WriteFile(keyPath, []byte("bad\n"), 0o600); err != nil {
		t.Fatalf("write bad key: %v", err)
	}

	t.Setenv(providerpolicy.DevAllowCloudProviderCustomBaseURLEnv, "1")
	client := mustLocalClient(t, config.Config{
		LLMMode:      "local_byok",
		BYOKProvider: provider,
		BYOKKeyFile:  keyPath,
		BYOKBaseURL:  server.URL,
		BYOKModel:    model,
	})
	client.HTTP = server.Client()

	if _, err := client.Turn(context.Background(), contracts.RelayTurnRequest{TaskID: "task_bad_" + provider, Phase: "intake"}); err == nil || err.Error() != "local_provider_auth_failed" {
		t.Fatalf("expected auth failure with bad %s key, got %v", provider, err)
	}
	if err := os.WriteFile(keyPath, []byte("good\n"), 0o600); err != nil {
		t.Fatalf("write good key: %v", err)
	}
	resp, err := client.Turn(context.Background(), contracts.RelayTurnRequest{TaskID: "task_good_" + provider, Phase: "intake"})
	if err != nil {
		t.Fatalf("expected %s recovery with rewritten key: %v", provider, err)
	}
	if resp.Summary != provider+" recovered" {
		t.Fatalf("unexpected %s response after key rewrite: %#v", provider, resp)
	}
	if len(seen) != 2 || seen[0] != "Bearer bad" || seen[1] != "Bearer good" {
		t.Fatalf("expected %s provider to see bad then good auth headers, got %#v", provider, seen)
	}
}

func responsesPayload(summary string) string {
	return `{"output":[{"content":[{"type":"output_text","text":"` + escapedTurnJSON(summary) + `"}]}]}`
}

func anthropicPayload(summary string) string {
	return `{"content":[{"type":"text","text":"` + escapedTurnJSON(summary) + `"}]}`
}

func ollamaPayload(summary string) string {
	return `{"response":"` + escapedTurnJSON(summary) + `"}`
}

func escapedTurnJSON(summary string) string {
	raw := turnJSON(summary)
	escaped, _ := json.Marshal(raw)
	return string(escaped[1 : len(escaped)-1])
}

func turnJSON(summary string) string {
	raw, _ := json.Marshal(map[string]any{
		"intent":               "general",
		"target_room":          "main_hall",
		"thought_text":         "ok",
		"summary":              summary,
		"report_text":          summary,
		"completion_status":    "completed",
		"continuation_status":  "completed",
		"next_blocker":         "",
		"action_type":          "none",
		"action_payload":       map[string]any{},
		"expected_check":       nil,
		"requires_user_choice": false,
		"choices":              []string{},
	})
	return string(raw)
}
