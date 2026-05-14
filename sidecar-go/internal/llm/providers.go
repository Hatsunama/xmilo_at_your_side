package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"xmilo/sidecar-go/internal/config"
	"xmilo/sidecar-go/internal/netutil"
	"xmilo/sidecar-go/internal/providerpolicy"
	"xmilo/sidecar-go/shared/contracts"
	"xmilo/sidecar-go/shared/plannerpolicy"
)

type ProviderAdapter interface {
	Provider() string
	KeyRequired() bool
	Turn(context.Context, *http.Client, config.Config, string, contracts.RelayTurnRequest) (contracts.RelayTurnResponse, error)
}

type LocalClient struct {
	cfg     config.Config
	adapter ProviderAdapter
	HTTP    *http.Client
}

type ProviderError struct {
	Code          string
	Provider      string
	BaseURLHost   string
	EndpointPath  string
	HTTPStatus    int
	NetworkClass  string
	ProviderClass string
}

func (e *ProviderError) Error() string {
	return e.Code
}

func (e *ProviderError) SafeFields() map[string]any {
	fields := map[string]any{
		"error_code": e.Code,
	}
	if e.Provider != "" {
		fields["provider"] = e.Provider
	}
	if e.BaseURLHost != "" {
		fields["base_url_host"] = e.BaseURLHost
	}
	if e.EndpointPath != "" {
		fields["endpoint_path"] = e.EndpointPath
	}
	if e.HTTPStatus != 0 {
		fields["http_status"] = e.HTTPStatus
	}
	if e.NetworkClass != "" {
		fields["network_class"] = e.NetworkClass
	}
	if e.ProviderClass != "" {
		fields["provider_error_class"] = e.ProviderClass
	}
	return fields
}

func SafeDiagnostic(err error) map[string]any {
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		return providerErr.SafeFields()
	}
	return nil
}

func NewLocalProvider(cfg config.Config) (*LocalClient, error) {
	cfg.ApplyBYOKProviderDefaults()
	adapter, err := adapterFor(cfg.BYOKProvider)
	if err != nil {
		return nil, err
	}
	return &LocalClient{
		cfg:     cfg,
		adapter: adapter,
		HTTP:    netutil.NewResilientHTTPClient(10 * time.Minute),
	}, nil
}

func (c *LocalClient) Turn(ctx context.Context, req contracts.RelayTurnRequest) (contracts.RelayTurnResponse, error) {
	var out contracts.RelayTurnResponse
	apiKey, err := apiKey(c.cfg, c.adapter.KeyRequired())
	if err != nil {
		return out, err
	}
	return c.adapter.Turn(ctx, c.HTTP, c.cfg, apiKey, req)
}

func LocalProviderReady(cfg config.Config) bool {
	if !cfg.LocalBYOKActive() {
		return false
	}
	cfg.ApplyBYOKProviderDefaults()
	adapter, err := adapterFor(cfg.BYOKProvider)
	if err != nil {
		return false
	}
	_, keyErr := apiKey(cfg, adapter.KeyRequired())
	resolved, err := providerpolicy.Resolve(providerpolicy.ResolveInput{
		Provider:     cfg.BYOKProvider,
		BaseURL:      cfg.BYOKBaseURL,
		Model:        cfg.BYOKModel,
		KeyFileReady: keyErr == nil,
		HasAPIKey:    keyErr == nil,
	})
	return err == nil && resolved.LocalTurnAllowed
}

func adapterFor(provider string) (ProviderAdapter, error) {
	normalized, err := providerpolicy.NormalizeProvider(provider)
	if err != nil {
		return nil, err
	}
	switch normalized {
	case providerpolicy.ProviderXAI:
		return xaiAdapter{}, nil
	case providerpolicy.ProviderOpenAI:
		return openAIAdapter{}, nil
	case providerpolicy.ProviderAnthropic:
		return anthropicAdapter{}, nil
	case providerpolicy.ProviderOllama:
		return ollamaAdapter{}, nil
	default:
		return nil, errors.New("local_provider_unavailable")
	}
}

func apiKey(cfg config.Config, required bool) (string, error) {
	if path := strings.TrimSpace(cfg.BYOKKeyFile); path != "" {
		raw, err := os.ReadFile(path)
		if err != nil {
			if required {
				return "", errors.New("missing_local_provider_key")
			}
			return "", nil
		}
		if key := strings.TrimSpace(string(raw)); key != "" {
			return key, nil
		}
	}
	envName := strings.TrimSpace(cfg.BYOKKeyEnv)
	if envName == "" {
		envName = "XMILO_BYOK_API_KEY"
	}
	if key := strings.TrimSpace(os.Getenv(envName)); key != "" {
		return key, nil
	}
	if required {
		return "", errors.New("missing_local_provider_key")
	}
	return "", nil
}

type xaiAdapter struct{}

func (xaiAdapter) Provider() string { return providerpolicy.ProviderXAI }
func (xaiAdapter) KeyRequired() bool {
	return providerpolicy.MustSpec(providerpolicy.ProviderXAI).KeyRequired
}
func (xaiAdapter) Turn(ctx context.Context, httpClient *http.Client, cfg config.Config, apiKey string, req contracts.RelayTurnRequest) (contracts.RelayTurnResponse, error) {
	return responsesAPITurn(ctx, httpClient, cfg, apiKey, req, providerpolicy.ProviderXAI)
}

type openAIAdapter struct{}

func (openAIAdapter) Provider() string { return providerpolicy.ProviderOpenAI }
func (openAIAdapter) KeyRequired() bool {
	return providerpolicy.MustSpec(providerpolicy.ProviderOpenAI).KeyRequired
}
func (openAIAdapter) Turn(ctx context.Context, httpClient *http.Client, cfg config.Config, apiKey string, req contracts.RelayTurnRequest) (contracts.RelayTurnResponse, error) {
	return responsesAPITurn(ctx, httpClient, cfg, apiKey, req, providerpolicy.ProviderOpenAI)
}

type anthropicAdapter struct{}

func (anthropicAdapter) Provider() string { return providerpolicy.ProviderAnthropic }
func (anthropicAdapter) KeyRequired() bool {
	return providerpolicy.MustSpec(providerpolicy.ProviderAnthropic).KeyRequired
}
func (anthropicAdapter) Turn(ctx context.Context, httpClient *http.Client, cfg config.Config, apiKey string, req contracts.RelayTurnRequest) (contracts.RelayTurnResponse, error) {
	var out contracts.RelayTurnResponse
	rawBody, err := json.Marshal(map[string]any{
		"model":      cfg.BYOKModel,
		"max_tokens": 2048,
		"system":     req.SystemPrompt,
		"messages": []map[string]string{
			{"role": "user", "content": buildProviderPrompt(req, "anthropic")},
		},
	})
	if err != nil {
		return out, errors.New("local_provider_request_failed")
	}
	endpoint, diag, err := providerEndpoint(cfg.BYOKBaseURL, "/messages", providerpolicy.ProviderAnthropic)
	if err != nil {
		return out, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(rawBody))
	if err != nil {
		return out, providerError("local_provider_request_failed", diag)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	raw, err := doProviderRequest(httpClient, httpReq, diag)
	if err != nil {
		return out, err
	}
	text, err := extractAnthropicText(raw)
	if err != nil {
		return out, errors.New("local_provider_request_failed")
	}
	out, err = parsePlannerResponseText(text)
	if err != nil {
		return out, err
	}
	return out, nil
}

type ollamaAdapter struct{}

func (ollamaAdapter) Provider() string { return providerpolicy.ProviderOllama }
func (ollamaAdapter) KeyRequired() bool {
	return providerpolicy.MustSpec(providerpolicy.ProviderOllama).KeyRequired
}
func (ollamaAdapter) Turn(ctx context.Context, httpClient *http.Client, cfg config.Config, apiKey string, req contracts.RelayTurnRequest) (contracts.RelayTurnResponse, error) {
	var out contracts.RelayTurnResponse
	if strings.TrimSpace(cfg.BYOKBaseURL) == "" {
		return out, errors.New("local_provider_unavailable")
	}
	rawBody, err := json.Marshal(map[string]any{
		"model":  cfg.BYOKModel,
		"prompt": buildProviderPrompt(req, "ollama"),
		"stream": false,
		"format": "json",
	})
	if err != nil {
		return out, errors.New("local_provider_request_failed")
	}
	endpoint, diag, err := providerEndpoint(cfg.BYOKBaseURL, "/api/generate", providerpolicy.ProviderOllama)
	if err != nil {
		return out, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(rawBody))
	if err != nil {
		return out, providerError("local_provider_request_failed", diag)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}
	raw, err := doProviderRequest(httpClient, httpReq, diag)
	if err != nil {
		return out, err
	}
	text, err := extractOllamaResponse(raw)
	if err != nil {
		return out, errors.New("local_provider_request_failed")
	}
	out, err = parsePlannerResponseText(text)
	if err != nil {
		return out, err
	}
	return out, nil
}

func responsesAPITurn(ctx context.Context, httpClient *http.Client, cfg config.Config, apiKey string, req contracts.RelayTurnRequest, provider string) (contracts.RelayTurnResponse, error) {
	var out contracts.RelayTurnResponse
	rawBody, err := json.Marshal(buildResponsesBody(req, cfg.BYOKModel, provider))
	if err != nil {
		return out, errors.New("local_provider_request_failed")
	}
	endpoint, diag, err := providerEndpoint(cfg.BYOKBaseURL, "/responses", provider)
	if err != nil {
		return out, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(rawBody))
	if err != nil {
		return out, providerError("local_provider_request_failed", diag)
	}
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	raw, err := doProviderRequest(httpClient, httpReq, diag)
	if err != nil {
		return out, err
	}
	text, err := extractResponsesOutputText(raw)
	if err != nil {
		return out, errors.New("local_provider_request_failed")
	}
	out, err = parsePlannerResponseText(text)
	if err != nil {
		return out, err
	}
	return out, nil
}

func parsePlannerResponseText(text string) (contracts.RelayTurnResponse, error) {
	var out contracts.RelayTurnResponse
	candidate, ok := plannerJSONCandidate(text)
	if !ok {
		return out, errors.New("local_provider_invalid_planner_response")
	}
	if err := validatePlannerJSONKeySet(candidate); err != nil {
		return out, errors.New("local_provider_invalid_planner_response")
	}
	if err := json.Unmarshal([]byte(candidate), &out); err != nil {
		return out, errors.New("local_provider_invalid_planner_response")
	}
	if err := plannerpolicy.ValidateResponse(out); err != nil {
		return out, errors.New("local_provider_invalid_planner_response")
	}
	return out, nil
}

var requiredPlannerResponseKeys = map[string]struct{}{
	"intent":               {},
	"target_room":          {},
	"thought_text":         {},
	"summary":              {},
	"report_text":          {},
	"completion_status":    {},
	"continuation_status":  {},
	"next_blocker":         {},
	"action_type":          {},
	"action_payload":       {},
	"expected_check":       {},
	"requires_user_choice": {},
	"choices":              {},
}

func validatePlannerJSONKeySet(candidate string) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(candidate), &raw); err != nil {
		return err
	}
	for key := range raw {
		if _, ok := requiredPlannerResponseKeys[key]; !ok {
			return fmt.Errorf("unexpected_key:%s", key)
		}
	}
	for key := range requiredPlannerResponseKeys {
		if _, ok := raw[key]; !ok {
			return fmt.Errorf("missing_key:%s", key)
		}
	}
	return nil
}

func plannerJSONCandidate(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", false
	}
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		return trimmed, true
	}
	if strings.HasPrefix(trimmed, "```") {
		lines := strings.Split(trimmed, "\n")
		if len(lines) < 3 {
			return "", false
		}
		opening := strings.TrimSpace(lines[0])
		if opening != "```" && !strings.EqualFold(opening, "```json") {
			return "", false
		}
		if strings.TrimSpace(lines[len(lines)-1]) != "```" {
			return "", false
		}
		body := strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
		if strings.HasPrefix(body, "{") && strings.HasSuffix(body, "}") {
			return body, true
		}
	}
	return "", false
}

func providerEndpoint(baseURL, endpointPath, provider string) (string, *ProviderError, error) {
	trimmed := strings.TrimSpace(baseURL)
	resolved, resolveErr := providerpolicy.Resolve(providerpolicy.ResolveInput{
		Provider:  provider,
		BaseURL:   trimmed,
		HasAPIKey: true,
	})
	parsed, err := url.Parse(resolved.BaseURL)
	if resolveErr != nil || err != nil || parsed.Scheme == "" || parsed.Host == "" {
		diag := &ProviderError{
			Code:         providerEndpointErrorCode(resolveErr),
			Provider:     provider,
			EndpointPath: endpointPath,
		}
		return "", diag, diag
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + endpointPath
	parsed.RawQuery = ""
	parsed.Fragment = ""
	diag := &ProviderError{
		Provider:     provider,
		BaseURLHost:  parsed.Host,
		EndpointPath: endpointPath,
	}
	return parsed.String(), diag, nil
}

func providerEndpointErrorCode(err error) string {
	if err == nil {
		return "local_provider_unavailable"
	}
	switch err.Error() {
	case "local_provider_disallowed_url_scheme", "local_provider_custom_base_url_not_allowed":
		return err.Error()
	default:
		return "local_provider_unavailable"
	}
}

func providerError(code string, base *ProviderError) error {
	if base == nil {
		return errors.New(code)
	}
	out := *base
	out.Code = code
	return &out
}

func doProviderRequest(httpClient *http.Client, req *http.Request, diag *ProviderError) ([]byte, error) {
	resp, err := httpClient.Do(req)
	if err != nil {
		out := providerError("local_provider_unreachable", diag).(*ProviderError)
		out.NetworkClass = classifyNetworkError(err)
		return nil, out
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, providerError("local_provider_request_failed", diag)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		out := providerError("local_provider_auth_failed", diag).(*ProviderError)
		out.HTTPStatus = resp.StatusCode
		out.ProviderClass = "auth"
		return nil, out
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		out := providerError("local_provider_rate_limited", diag).(*ProviderError)
		out.HTTPStatus = resp.StatusCode
		out.ProviderClass = "rate_limit"
		return nil, out
	}
	if resp.StatusCode >= 400 {
		out := providerError("local_provider_request_failed", diag).(*ProviderError)
		out.HTTPStatus = resp.StatusCode
		out.ProviderClass = "http_error"
		return nil, out
	}
	return raw, nil
}

func classifyNetworkError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "dns"
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if strings.Contains(strings.ToLower(opErr.Err.Error()), "certificate") {
			return "tls_certificate"
		}
		return "network"
	}
	if strings.Contains(strings.ToLower(err.Error()), "certificate") {
		return "tls_certificate"
	}
	return "network"
}

func buildResponsesBody(req contracts.RelayTurnRequest, model, provider string) map[string]any {
	return map[string]any{
		"model": model,
		"store": false,
		"input": []map[string]any{
			{
				"role": "system",
				"content": []map[string]string{
					{"type": "input_text", "text": req.SystemPrompt},
				},
			},
			{
				"role": "user",
				"content": []map[string]string{
					{"type": "input_text", "text": buildProviderPrompt(req, provider)},
				},
			},
		},
		"text": map[string]any{
			"format": map[string]any{
				"type": "json_object",
			},
		},
	}
}

func buildProviderPrompt(req contracts.RelayTurnRequest, provider string) string {
	return plannerpolicy.RenderPrompt(plannerpolicy.LocalBYOKPlannerRole(provider), req)
}

func extractResponsesOutputText(raw []byte) (string, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	output, _ := payload["output"].([]any)
	for _, item := range output {
		msg, _ := item.(map[string]any)
		contents, _ := msg["content"].([]any)
		for _, content := range contents {
			piece, _ := content.(map[string]any)
			if piece["type"] == "output_text" {
				if text, ok := piece["text"].(string); ok {
					return text, nil
				}
			}
		}
	}
	return "", fmt.Errorf("no output_text found")
}

func extractAnthropicText(raw []byte) (string, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	content, _ := payload["content"].([]any)
	for _, item := range content {
		piece, _ := item.(map[string]any)
		if piece["type"] == "text" {
			if text, ok := piece["text"].(string); ok {
				return text, nil
			}
		}
	}
	return "", fmt.Errorf("no text content found")
}

func extractOllamaResponse(raw []byte) (string, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	text, _ := payload["response"].(string)
	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("no response found")
	}
	return text, nil
}
