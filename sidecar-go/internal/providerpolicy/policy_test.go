package providerpolicy

import "testing"

func TestOptionsExposeSidecarOwnedProviderPolicy(t *testing.T) {
	options := Options()
	if len(options) != 4 {
		t.Fatalf("expected four providers, got %d", len(options))
	}
	for _, spec := range options {
		if spec.ID == ProviderOllama {
			if !spec.BaseURLRequired || spec.KeyRequired || !spec.CustomBaseURLAllowed {
				t.Fatalf("unexpected ollama policy: %#v", spec)
			}
			continue
		}
		if !spec.KeyRequired || !spec.BaseURLRequired || spec.CustomBaseURLAllowed {
			t.Fatalf("cloud provider must require key/base URL and disallow custom base URL by default: %#v", spec)
		}
		if len(spec.AllowedSchemes) != 1 || spec.AllowedSchemes[0] != "https" {
			t.Fatalf("cloud provider must be https-only: %#v", spec)
		}
		if spec.DevCustomBaseURLEnv != DevAllowCloudProviderCustomBaseURLEnv {
			t.Fatalf("cloud provider dev exception must be explicit and visible: %#v", spec)
		}
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

func TestResolveRequiresExplicitOllamaBaseURL(t *testing.T) {
	_, err := Resolve(ResolveInput{Provider: ProviderOllama})
	if err == nil || err.Error() != "local_provider_base_url_required" {
		t.Fatalf("expected ollama base URL requirement, got %v", err)
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
}
