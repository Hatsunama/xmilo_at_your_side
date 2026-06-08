package providerpolicy

import "testing"

func TestOptionsExposeSidecarOwnedProviderPolicy(t *testing.T) {
	options := Options()
	if len(options) != 5 {
		t.Fatalf("expected five providers, got %d", len(options))
	}
	seen := map[string]ProviderSpec{}
	for _, spec := range options {
		seen[spec.ID] = spec
		if spec.ID == ProviderOllama {
			if spec.Label != "Ollama Local" {
				t.Fatalf("local ollama label must be explicit, got %#v", spec)
			}
			if !spec.BaseURLRequired || spec.KeyRequired || !spec.CustomBaseURLAllowed {
				t.Fatalf("unexpected ollama policy: %#v", spec)
			}
			if spec.ConnectionKind != ConnectionKindLocalServer ||
				spec.BaseURLEntryMode != BaseURLEntryModeGuidedLocalServer ||
				!spec.AdvancedManualBaseURLAllowed ||
				!spec.RequiresConnectionTarget ||
				spec.RequiresReachabilityProbe ||
				spec.SetupGuidanceCode != SetupGuidanceOllamaConnectionTargetRequired {
				t.Fatalf("ollama must expose guided local-server setup contract: %#v", spec)
			}
			continue
		}
		if !spec.KeyRequired || !spec.BaseURLRequired || spec.CustomBaseURLAllowed {
			t.Fatalf("cloud provider must require key/base URL and disallow custom base URL by default: %#v", spec)
		}
		if spec.ConnectionKind != ConnectionKindCloudCanonical ||
			spec.BaseURLEntryMode != BaseURLEntryModeHiddenCanonical ||
			spec.AdvancedManualBaseURLAllowed ||
			spec.RequiresConnectionTarget ||
			spec.RequiresReachabilityProbe ||
			spec.SetupGuidanceCode != "" {
			t.Fatalf("cloud provider must expose hidden canonical setup contract: %#v", spec)
		}
		if len(spec.AllowedSchemes) != 1 || spec.AllowedSchemes[0] != "https" {
			t.Fatalf("cloud provider must be https-only: %#v", spec)
		}
		if spec.DevCustomBaseURLEnv != DevAllowCloudProviderCustomBaseURLEnv {
			t.Fatalf("cloud provider dev exception must be explicit and visible: %#v", spec)
		}
	}
	if seen[ProviderOllamaCloud].ID == "" {
		t.Fatalf("provider options must expose ollama cloud")
	}
	cloud := seen[ProviderOllamaCloud]
	if cloud.Label != "Ollama Cloud" ||
		cloud.DefaultBaseURL != "https://ollama.com" ||
		cloud.DefaultModel != "gpt-oss:120b" ||
		!cloud.KeyRequired ||
		cloud.CustomBaseURLAllowed ||
		cloud.AdvancedManualBaseURLAllowed ||
		cloud.ConnectionKind != ConnectionKindCloudCanonical ||
		cloud.BaseURLEntryMode != BaseURLEntryModeHiddenCanonical ||
		cloud.RequiresConnectionTarget ||
		cloud.RequiresReachabilityProbe {
		t.Fatalf("ollama cloud must expose canonical cloud setup contract: %#v", cloud)
	}
}

func TestResolveNormalizesAliasesAndDefaults(t *testing.T) {
	resolved, err := Resolve(ResolveInput{Provider: "grok", KeyFileReady: true})
	if err != nil {
		t.Fatalf("resolve alias: %v", err)
	}
	if resolved.Provider != ProviderXAI || resolved.Model == "" || resolved.BaseURL == "" {
		t.Fatalf("unexpected resolved config: %#v", resolved)
	}
	if resolved.ConnectionKind != ConnectionKindCloudCanonical ||
		resolved.BaseURLEntryMode != BaseURLEntryModeHiddenCanonical ||
		resolved.RequiresConnectionTarget ||
		resolved.RequiresReachabilityProbe {
		t.Fatalf("cloud default resolve must stay canonical and not require user base URL: %#v", resolved)
	}
}

func TestCloudProvidersUseCanonicalDefaultsWithoutManualBaseURL(t *testing.T) {
	tests := []struct {
		provider string
		wantURL  string
	}{
		{ProviderXAI, "https://api.x.ai/v1"},
		{ProviderOpenAI, "https://api.openai.com/v1"},
		{ProviderAnthropic, "https://api.anthropic.com/v1"},
		{ProviderOllamaCloud, "https://ollama.com"},
	}
	for _, test := range tests {
		t.Run(test.provider, func(t *testing.T) {
			resolved, err := Resolve(ResolveInput{Provider: test.provider, HasAPIKey: true})
			if err != nil {
				t.Fatalf("resolve cloud default: %v", err)
			}
			if resolved.BaseURL != test.wantURL || !resolved.LocalTurnAllowed {
				t.Fatalf("cloud provider must use canonical default without manual base URL: %#v", resolved)
			}
			if resolved.BaseURLEntryMode != BaseURLEntryModeHiddenCanonical || resolved.AdvancedManualBaseURLAllowed {
				t.Fatalf("cloud normal path must hide manual base URL: %#v", resolved)
			}
		})
	}
}

func TestResolveRejectsGenericHTTPForCloudProviders(t *testing.T) {
	_, err := Resolve(ResolveInput{
		Provider:     ProviderOpenAI,
		BaseURL:      "http://api.openai.com/v1",
		Model:        "gpt-test",
		KeyFileReady: true,
	})
	if err == nil || err.Error() != "local_provider_disallowed_url_scheme" {
		t.Fatalf("expected disallowed scheme, got %v", err)
	}
}

func TestResolveBlankOllamaReturnsGuidedConnectionTargetState(t *testing.T) {
	resolved, err := Resolve(ResolveInput{Provider: ProviderOllama})
	if err != nil {
		t.Fatalf("blank ollama should resolve to guided setup state, got %v", err)
	}
	if resolved.LocalTurnAllowed {
		t.Fatalf("blank ollama must not be ready: %#v", resolved)
	}
	if resolved.ReadinessReason != ReasonConnectionTargetRequired ||
		!resolved.RequiresConnectionTarget ||
		resolved.RequiresReachabilityProbe ||
		resolved.SetupGuidanceCode != SetupGuidanceOllamaConnectionTargetRequired ||
		resolved.ConnectionKind != ConnectionKindLocalServer ||
		resolved.BaseURLEntryMode != BaseURLEntryModeGuidedLocalServer ||
		!resolved.AdvancedManualBaseURLAllowed {
		t.Fatalf("blank ollama must expose guided connection target contract: %#v", resolved)
	}
}

func TestResolveOllamaCloudRequiresKeyAndUsesCanonicalCloudEndpoint(t *testing.T) {
	missingKey, err := Resolve(ResolveInput{Provider: ProviderOllamaCloud})
	if err != nil {
		t.Fatalf("ollama cloud missing key should resolve to not-ready state, got %v", err)
	}
	if missingKey.LocalTurnAllowed || missingKey.ReadinessReason != "missing_key" {
		t.Fatalf("ollama cloud without key must be key-specific not-ready state: %#v", missingKey)
	}
	if missingKey.RequiresConnectionTarget || missingKey.ConnectionKind != ConnectionKindCloudCanonical || missingKey.BaseURLEntryMode != BaseURLEntryModeHiddenCanonical {
		t.Fatalf("ollama cloud must not use local-server setup state: %#v", missingKey)
	}
	resolved, err := Resolve(ResolveInput{Provider: ProviderOllamaCloud, HasAPIKey: true})
	if err != nil {
		t.Fatalf("resolve ollama cloud: %v", err)
	}
	if !resolved.LocalTurnAllowed || resolved.BaseURL != "https://ollama.com" || resolved.Model != "gpt-oss:120b" {
		t.Fatalf("ollama cloud with key must use canonical cloud defaults: %#v", resolved)
	}
}

func TestResolveOllamaCloudRejectsCustomBaseURL(t *testing.T) {
	_, err := Resolve(ResolveInput{Provider: ProviderOllamaCloud, BaseURL: "https://example.com", HasAPIKey: true})
	if err == nil || err.Error() != "local_provider_custom_base_url_not_allowed" {
		t.Fatalf("expected custom cloud base URL rejection, got %v", err)
	}
	_, err = Resolve(ResolveInput{Provider: ProviderOllamaCloud, BaseURL: "http://ollama.com", HasAPIKey: true})
	if err == nil || err.Error() != "local_provider_disallowed_url_scheme" {
		t.Fatalf("expected cloud scheme rejection, got %v", err)
	}
}

func TestResolveOllamaAdvancedManualURLValidation(t *testing.T) {
	for _, badURL := range []string{"", "localhost:11434", "ftp://localhost:11434", "http://"} {
		if badURL == "" {
			continue
		}
		_, err := Resolve(ResolveInput{Provider: ProviderOllama, BaseURL: badURL})
		if err == nil || err.Error() != ReasonManualURLInvalid {
			t.Fatalf("expected %s for %q, got %v", ReasonManualURLInvalid, badURL, err)
		}
	}
	resolved, err := Resolve(ResolveInput{
		Provider: ProviderOllama,
		BaseURL:  "http://192.168.1.10:11434",
	})
	if err != nil {
		t.Fatalf("resolve ollama: %v", err)
	}
	if !resolved.LocalTurnAllowed {
		t.Fatalf("ollama without key should be locally allowed when base URL is explicit: %#v", resolved)
	}
	if resolved.RequiresConnectionTarget || resolved.SetupGuidanceCode != "" {
		t.Fatalf("manual ollama URL should satisfy connection target without fake probe claims: %#v", resolved)
	}
}
