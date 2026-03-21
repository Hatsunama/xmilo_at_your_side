package openai

import (
	"testing"

	"xmilo/relay-go/shared/contracts"
)

func TestBuildResponsesBodyStripsUnsupportedXAIFields(t *testing.T) {
	body := buildResponsesBody(contracts.RelayTurnRequest{
		Phase:        "research",
		Prompt:       "test",
		SystemPrompt: "system",
	}, "grok-4")

	for _, key := range []string{
		"presence_penalty",
		"frequency_penalty",
		"stop",
		"reasoning_effort",
		"logprobs",
		"top_logprobs",
		"temperature",
	} {
		if _, exists := body[key]; exists {
			t.Fatalf("expected %s to be stripped", key)
		}
	}

	if body["model"] != "grok-4" {
		t.Fatalf("expected model to be preserved")
	}
}

func TestNewDefaultsBaseURLToXAI(t *testing.T) {
	client := New("key", "grok-4", "")
	if client.BaseURL != defaultXAIBaseURL {
		t.Fatalf("expected default base URL %s, got %s", defaultXAIBaseURL, client.BaseURL)
	}
}
