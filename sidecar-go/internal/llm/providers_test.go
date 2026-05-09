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
	"xmilo/sidecar-go/shared/contracts"
)

func TestXAITurnUsesResponsesPathAndParsesResponse(t *testing.T) {
	server := providerServer(t, "/responses", "Bearer local-test-key", responsesPayload("xai done"), nil)
	defer server.Close()
	client := mustLocalClient(t, localConfig(t, "xai", server.URL, "grok-test", true))
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
	resp, err := client.Turn(context.Background(), contracts.RelayTurnRequest{TaskID: "task_1", Phase: "intake"})
	if err != nil {
		t.Fatalf("turn: %v", err)
	}
	if resp.Summary != "ollama done" {
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

func TestUnsupportedProviderReturnsPreciseError(t *testing.T) {
	if _, err := NewLocalProvider(config.Config{BYOKProvider: "mystery"}); err == nil || err.Error() != "local_provider_unavailable" {
		t.Fatalf("expected local_provider_unavailable, got %v", err)
	}
}

func TestProviderAuthFailureReturnsPreciseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()
	client := mustLocalClient(t, localConfig(t, "xai", server.URL, "grok-test", true))
	if _, err := client.Turn(context.Background(), contracts.RelayTurnRequest{}); err == nil || err.Error() != "local_provider_auth_failed" {
		t.Fatalf("expected local_provider_auth_failed, got %v", err)
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
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/responses" {
					t.Fatalf("unexpected path %s", r.URL.Path)
				}
				http.Error(w, "provider error", test.statusCode)
			}))
			defer server.Close()

			client := mustLocalClient(t, localConfig(t, "openai", server.URL, "gpt-test", true))
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
	if _, err := client.Turn(context.Background(), contracts.RelayTurnRequest{}); err == nil || err.Error() != "local_provider_unreachable" {
		t.Fatalf("expected local_provider_unreachable, got %v", err)
	}
}

func TestOpenAIUnreachableIncludesSafeDiagnostic(t *testing.T) {
	client := mustLocalClient(t, localConfig(t, "openai", "http://127.0.0.1:1", "gpt-test", true))
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

func TestLocalProviderReadyRequiresExplicitOllamaBaseURL(t *testing.T) {
	if LocalProviderReady(config.Config{LLMMode: "local_byok", BYOKProvider: "ollama", BYOKModel: "llama3.2"}) {
		t.Fatalf("ollama must require explicit base URL")
	}
	if !LocalProviderReady(config.Config{LLMMode: "local_byok", BYOKProvider: "ollama", BYOKBaseURL: "http://192.168.1.10:11434", BYOKModel: "llama3.2"}) {
		t.Fatalf("ollama with explicit base URL and model should be locally eligible without a key")
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
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	raw, _ := json.Marshal(map[string]any{
		"intent":               "general",
		"target_room":          "main_hall",
		"thought_text":         "ok",
		"summary":              summary,
		"report_text":          summary,
		"completion_status":    "completed",
		"continuation_status":  "completed",
		"requires_user_choice": false,
		"choices":              []string{},
	})
	escaped, _ := json.Marshal(string(raw))
	return string(escaped[1 : len(escaped)-1])
}
