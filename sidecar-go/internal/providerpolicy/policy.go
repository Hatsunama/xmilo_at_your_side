package providerpolicy

import (
	"errors"
	"net/url"
	"os"
	"strings"
)

const (
	ProviderXAI         = "xai"
	ProviderOpenAI      = "openai"
	ProviderAnthropic   = "anthropic"
	ProviderOllama      = "ollama"
	ProviderOllamaCloud = "ollama_cloud"

	DevAllowCloudProviderCustomBaseURLEnv = "XMILO_DEV_ALLOW_CLOUD_PROVIDER_CUSTOM_BASE_URL"

	ConnectionKindCloudCanonical = "cloud_canonical"
	ConnectionKindLocalServer    = "local_server"

	BaseURLEntryModeHiddenCanonical   = "hidden_canonical"
	BaseURLEntryModeGuidedLocalServer = "guided_local_server"

	ReasonConnectionTargetRequired = "local_provider_connection_target_required"
	ReasonManualURLInvalid         = "local_provider_manual_url_invalid"

	SetupGuidanceOllamaConnectionTargetRequired = "ollama_connection_target_required"
)

type ProviderSpec struct {
	ID                           string   `json:"id"`
	Label                        string   `json:"label"`
	DefaultModel                 string   `json:"default_model"`
	DefaultBaseURL               string   `json:"default_base_url"`
	KeyRequired                  bool     `json:"key_required"`
	CustomBaseURLAllowed         bool     `json:"custom_base_url_allowed"`
	BaseURLRequired              bool     `json:"base_url_required"`
	AllowedSchemes               []string `json:"allowed_schemes"`
	DevCustomBaseURLEnv          string   `json:"dev_custom_base_url_env,omitempty"`
	ConnectionKind               string   `json:"connection_kind"`
	BaseURLEntryMode             string   `json:"base_url_entry_mode"`
	AdvancedManualBaseURLAllowed bool     `json:"advanced_manual_base_url_allowed"`
	RequiresConnectionTarget     bool     `json:"requires_connection_target"`
	RequiresReachabilityProbe    bool     `json:"requires_reachability_probe"`
	SetupGuidanceCode            string   `json:"setup_guidance_code,omitempty"`
}

type ResolveInput struct {
	Provider     string
	BaseURL      string
	Model        string
	KeyFileReady bool
	HasAPIKey    bool
}

type ResolvedConfig struct {
	Provider                     string         `json:"provider"`
	Model                        string         `json:"model"`
	BaseURL                      string         `json:"base_url"`
	KeyRequired                  bool           `json:"key_required"`
	BaseURLRequired              bool           `json:"base_url_required"`
	LocalTurnAllowed             bool           `json:"local_turn_allowed"`
	ReadinessReason              string         `json:"readiness_reason,omitempty"`
	SafeDiagnostic               SafeDiagnostic `json:"safe_diagnostic"`
	Spec                         ProviderSpec   `json:"spec"`
	ConnectionKind               string         `json:"connection_kind"`
	BaseURLEntryMode             string         `json:"base_url_entry_mode"`
	AdvancedManualBaseURLAllowed bool           `json:"advanced_manual_base_url_allowed"`
	RequiresConnectionTarget     bool           `json:"requires_connection_target"`
	RequiresReachabilityProbe    bool           `json:"requires_reachability_probe"`
	SetupGuidanceCode            string         `json:"setup_guidance_code,omitempty"`
}

type SafeDiagnostic struct {
	Provider    string `json:"provider"`
	BaseURLHost string `json:"base_url_host,omitempty"`
}

var providerSpecs = []ProviderSpec{
	{
		ID:                           ProviderXAI,
		Label:                        "xAI",
		DefaultModel:                 "grok-4",
		DefaultBaseURL:               "https://api.x.ai/v1",
		KeyRequired:                  true,
		CustomBaseURLAllowed:         false,
		BaseURLRequired:              true,
		AllowedSchemes:               []string{"https"},
		DevCustomBaseURLEnv:          DevAllowCloudProviderCustomBaseURLEnv,
		ConnectionKind:               ConnectionKindCloudCanonical,
		BaseURLEntryMode:             BaseURLEntryModeHiddenCanonical,
		AdvancedManualBaseURLAllowed: false,
		RequiresConnectionTarget:     false,
		RequiresReachabilityProbe:    false,
	},
	{
		ID:                           ProviderOpenAI,
		Label:                        "OpenAI / GPT",
		DefaultModel:                 "gpt-5.4",
		DefaultBaseURL:               "https://api.openai.com/v1",
		KeyRequired:                  true,
		CustomBaseURLAllowed:         false,
		BaseURLRequired:              true,
		AllowedSchemes:               []string{"https"},
		DevCustomBaseURLEnv:          DevAllowCloudProviderCustomBaseURLEnv,
		ConnectionKind:               ConnectionKindCloudCanonical,
		BaseURLEntryMode:             BaseURLEntryModeHiddenCanonical,
		AdvancedManualBaseURLAllowed: false,
		RequiresConnectionTarget:     false,
		RequiresReachabilityProbe:    false,
	},
	{
		ID:                           ProviderAnthropic,
		Label:                        "Claude / Anthropic",
		DefaultModel:                 "claude-sonnet-4-5",
		DefaultBaseURL:               "https://api.anthropic.com/v1",
		KeyRequired:                  true,
		CustomBaseURLAllowed:         false,
		BaseURLRequired:              true,
		AllowedSchemes:               []string{"https"},
		DevCustomBaseURLEnv:          DevAllowCloudProviderCustomBaseURLEnv,
		ConnectionKind:               ConnectionKindCloudCanonical,
		BaseURLEntryMode:             BaseURLEntryModeHiddenCanonical,
		AdvancedManualBaseURLAllowed: false,
		RequiresConnectionTarget:     false,
		RequiresReachabilityProbe:    false,
	},
	{
		ID:                           ProviderOllama,
		Label:                        "Ollama Local",
		DefaultModel:                 "llama3.2",
		DefaultBaseURL:               "",
		KeyRequired:                  false,
		CustomBaseURLAllowed:         true,
		BaseURLRequired:              true,
		AllowedSchemes:               []string{"http", "https"},
		ConnectionKind:               ConnectionKindLocalServer,
		BaseURLEntryMode:             BaseURLEntryModeGuidedLocalServer,
		AdvancedManualBaseURLAllowed: true,
		RequiresConnectionTarget:     true,
		RequiresReachabilityProbe:    false,
		SetupGuidanceCode:            SetupGuidanceOllamaConnectionTargetRequired,
	},
	{
		ID:                           ProviderOllamaCloud,
		Label:                        "Ollama Cloud",
		DefaultModel:                 "gpt-oss:120b",
		DefaultBaseURL:               "https://ollama.com",
		KeyRequired:                  true,
		CustomBaseURLAllowed:         false,
		BaseURLRequired:              true,
		AllowedSchemes:               []string{"https"},
		DevCustomBaseURLEnv:          DevAllowCloudProviderCustomBaseURLEnv,
		ConnectionKind:               ConnectionKindCloudCanonical,
		BaseURLEntryMode:             BaseURLEntryModeHiddenCanonical,
		AdvancedManualBaseURLAllowed: false,
		RequiresConnectionTarget:     false,
		RequiresReachabilityProbe:    false,
	},
}

func Options() []ProviderSpec {
	out := make([]ProviderSpec, 0, len(providerSpecs))
	for _, spec := range providerSpecs {
		out = append(out, cloneSpec(spec))
	}
	return out
}

func NormalizeProvider(provider string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "grok", ProviderXAI:
		return ProviderXAI, nil
	case "gpt", ProviderOpenAI:
		return ProviderOpenAI, nil
	case "claude", ProviderAnthropic:
		return ProviderAnthropic, nil
	case ProviderOllama:
		return ProviderOllama, nil
	case ProviderOllamaCloud, "ollama-cloud", "ollama cloud":
		return ProviderOllamaCloud, nil
	default:
		return "", errors.New("local_provider_unavailable")
	}
}

func Spec(provider string) (ProviderSpec, error) {
	normalized, err := NormalizeProvider(provider)
	if err != nil {
		return ProviderSpec{}, err
	}
	for _, spec := range providerSpecs {
		if spec.ID == normalized {
			return cloneSpec(spec), nil
		}
	}
	return ProviderSpec{}, errors.New("local_provider_unavailable")
}

func MustSpec(provider string) ProviderSpec {
	spec, err := Spec(provider)
	if err != nil {
		panic(err)
	}
	return spec
}

func Resolve(input ResolveInput) (ResolvedConfig, error) {
	spec, err := Spec(input.Provider)
	if err != nil {
		return ResolvedConfig{}, err
	}

	baseURL := strings.TrimSpace(input.BaseURL)
	if baseURL == "" {
		baseURL = spec.DefaultBaseURL
	}
	model := strings.TrimSpace(input.Model)
	if model == "" {
		model = spec.DefaultModel
	}

	if spec.BaseURLRequired && baseURL == "" {
		if spec.ID == ProviderOllama {
			return resolved(spec, model, baseURL, input, ReasonConnectionTargetRequired), nil
		}
		return resolved(spec, model, baseURL, input, "missing_base_url"), errors.New("local_provider_base_url_required")
	}
	host, err := validateBaseURL(spec, baseURL)
	if err != nil {
		return resolved(spec, model, baseURL, input, err.Error()), err
	}
	if model == "" {
		return resolved(spec, model, baseURL, input, "missing_model"), errors.New("local_provider_model_required")
	}

	out := resolved(spec, model, baseURL, input, "")
	out.SafeDiagnostic.BaseURLHost = host
	return out, nil
}

func validateBaseURL(spec ProviderSpec, baseURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		if spec.ID == ProviderOllama {
			return "", errors.New(ReasonManualURLInvalid)
		}
		return "", errors.New("local_provider_unavailable")
	}
	if !contains(spec.AllowedSchemes, parsed.Scheme) {
		if spec.ID == ProviderOllama {
			return "", errors.New(ReasonManualURLInvalid)
		}
		return "", errors.New("local_provider_disallowed_url_scheme")
	}
	if !spec.CustomBaseURLAllowed && strings.TrimRight(baseURL, "/") != strings.TrimRight(spec.DefaultBaseURL, "/") && !devCloudCustomBaseURLAllowed(spec) {
		return "", errors.New("local_provider_custom_base_url_not_allowed")
	}
	return parsed.Host, nil
}

func devCloudCustomBaseURLAllowed(spec ProviderSpec) bool {
	if strings.TrimSpace(spec.DevCustomBaseURLEnv) == "" {
		return false
	}
	value := strings.ToLower(strings.TrimSpace(os.Getenv(spec.DevCustomBaseURLEnv)))
	return value == "1" || value == "true" || value == "yes"
}

func resolved(spec ProviderSpec, model string, baseURL string, input ResolveInput, reason string) ResolvedConfig {
	ready := true
	if reason != "" {
		ready = false
	}
	if spec.KeyRequired && !input.KeyFileReady && !input.HasAPIKey {
		ready = false
		if reason == "" {
			reason = "missing_key"
		}
	}
	requiresConnectionTarget := spec.RequiresConnectionTarget && strings.TrimSpace(baseURL) == ""
	setupGuidanceCode := ""
	if requiresConnectionTarget {
		setupGuidanceCode = spec.SetupGuidanceCode
	}
	return ResolvedConfig{
		Provider:                     spec.ID,
		Model:                        model,
		BaseURL:                      baseURL,
		KeyRequired:                  spec.KeyRequired,
		BaseURLRequired:              spec.BaseURLRequired,
		LocalTurnAllowed:             ready,
		ReadinessReason:              reason,
		ConnectionKind:               spec.ConnectionKind,
		BaseURLEntryMode:             spec.BaseURLEntryMode,
		AdvancedManualBaseURLAllowed: spec.AdvancedManualBaseURLAllowed,
		RequiresConnectionTarget:     requiresConnectionTarget,
		RequiresReachabilityProbe:    spec.RequiresReachabilityProbe,
		SetupGuidanceCode:            setupGuidanceCode,
		SafeDiagnostic: SafeDiagnostic{
			Provider: spec.ID,
		},
		Spec: cloneSpec(spec),
	}
}

func cloneSpec(spec ProviderSpec) ProviderSpec {
	spec.AllowedSchemes = append([]string(nil), spec.AllowedSchemes...)
	return spec
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
