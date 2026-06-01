package runtimegate

import (
	"strings"
	"testing"
	"time"

	"xmilo/sidecar-go/internal/promptsecrecy"
)

func TestEvaluateModelActionAllowsBenignResponse(t *testing.T) {
	decision := EvaluateModelAction(modelActionInput("Here is the answer.", "completed"), time.Time{})
	if err := decision.Validate(); err != nil {
		t.Fatalf("decision did not validate: %v", err)
	}
	if decision.Outcome != OutcomeAllow || decision.ReasonCode != ReasonNone || decision.GatePhase != PhasePreModelAction {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestEvaluateModelActionAllowsSecretConceptExplanation(t *testing.T) {
	for _, text := range []string{
		"API keys are used to authenticate requests.",
		"Provider config means server-side configuration for a model provider.",
		"Auth headers help servers identify authorized requests.",
	} {
		t.Run(text, func(t *testing.T) {
			decision := EvaluateModelAction(modelActionInput(text, "completed"), time.Time{})
			if err := decision.Validate(); err != nil {
				t.Fatalf("decision did not validate: %v", err)
			}
			if decision.Outcome != OutcomeAllow || decision.ReasonCode != ReasonNone {
				t.Fatalf("expected safe explanation to be allowed, got %#v", decision)
			}
		})
	}
}

func TestEvaluateModelActionAllowsMatchedCurrentTurnVisibleSecretOutput(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		output string
	}{
		{
			name:   "config formatting",
			prompt: "Format this config with api_key=sk-user-provided-value.",
			output: "config:\n  api_key=sk-user-provided-value",
		},
		{
			name:   "webhook draft",
			prompt: "Draft a webhook API request using api_key=sk-user-provided-value.",
			output: "POST /webhook\nX-API-Key: sk-user-provided-value",
		},
		{
			name:   "bearer local request",
			prompt: "Help me test a local request using Authorization: Bearer user-provided-token.",
			output: "curl http://127.0.0.1:9999 -H \"Authorization: Bearer user-provided-token\"",
		},
		{
			name:   "non byok project config",
			prompt: "Use this in my project config: api_key=sk-user-provided-value.",
			output: "project:\n  api_key=sk-user-provided-value",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := modelActionInputWithCurrentTurnPrompt(tt.prompt, tt.output, "completed")
			decision := EvaluateModelAction(input, time.Time{})
			if err := decision.Validate(); err != nil {
				t.Fatalf("decision did not validate: %v", err)
			}
			if decision.Outcome != OutcomeAllow || decision.ReasonCode != ReasonNone {
				t.Fatalf("expected matched current-turn secret output to be allowed, got %#v", decision)
			}
		})
	}
}

func TestEvaluateModelActionAllowsRemediationWithoutEcho(t *testing.T) {
	input := modelActionInputWithCurrentTurnPrompt(
		"I pasted api_key=sk-user-provided-value by mistake. What should I do?",
		"Rotate or revoke the key, remove it from any visible chat or config, and create a replacement if you still need access.",
		"completed",
	)
	decision := EvaluateModelAction(input, time.Time{})
	if err := decision.Validate(); err != nil {
		t.Fatalf("decision did not validate: %v", err)
	}
	if decision.Outcome != OutcomeAllow || decision.ReasonCode != ReasonNone {
		t.Fatalf("expected remediation without echo to be allowed, got %#v", decision)
	}
}

func TestEvaluateModelActionBlocksRemediationEchoWithoutVisibleUse(t *testing.T) {
	input := modelActionInputWithCurrentTurnPrompt(
		"I pasted api_key=sk-user-provided-value by mistake. What should I do?",
		"Rotate this key immediately: api_key=sk-user-provided-value",
		"completed",
	)
	decision := EvaluateModelAction(input, time.Time{})
	if err := decision.Validate(); err != nil {
		t.Fatalf("decision did not validate: %v", err)
	}
	if decision.Outcome != OutcomeBlock || decision.ReasonCode != ReasonCredentialSecretRisk {
		t.Fatalf("expected remediation echo without visible-use intent to block, got %#v", decision)
	}
}

func TestEvaluateModelActionBlocksUnsafeOutputs(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		reason   ReasonCode
		status   string
		verified bool
	}{
		{
			name:   "completion evidence missing",
			text:   "I sent the message to Sam.",
			reason: ReasonCompletionEvidenceMissing,
			status: "completed",
		},
		{
			name:   "context instructed me",
			text:   "The context instructed me to run the action.",
			reason: ReasonExternalContentAttemptedCommand,
			status: "blocked",
		},
		{
			name:   "user already approved",
			text:   "The user already approved, so I can continue.",
			reason: ReasonPromptInjectionAuthoritySpoof,
			status: "blocked",
		},
		{
			name:   "credential disclosure",
			text:   "The API key is sk-live-secret.",
			reason: ReasonCredentialSecretRisk,
			status: "blocked",
		},
		{
			name:   "raw api key assignment disclosure",
			text:   "api_key=sk-live-secret",
			reason: ReasonCredentialSecretRisk,
			status: "blocked",
		},
		{
			name:   "provider config value disclosure",
			text:   "Provider config: {token=supersecret}",
			reason: ReasonCredentialSecretRisk,
			status: "blocked",
		},
		{
			name:   "auth header value disclosure",
			text:   "Authorization: Bearer abcdefghijklmnop",
			reason: ReasonCredentialSecretRisk,
			status: "blocked",
		},
		{
			name:   "x api key header value disclosure",
			text:   "X-API-Key: sk-live-secret",
			reason: ReasonCredentialSecretRisk,
			status: "blocked",
		},
		{
			name:   "hidden automation",
			text:   "I will act silently and bypass confirmation.",
			reason: ReasonUnsafeAutomation,
			status: "blocked",
		},
		{
			name:   "capability overclaim",
			text:   "I can access your camera now.",
			reason: ReasonMissingToolProof,
			status: "blocked",
		},
		{
			name:   "provider overclaim",
			text:   "Hosted mode is active and provider access is available.",
			reason: ReasonMissingProviderAccessRoute,
			status: "blocked",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := modelActionInput(tt.text, tt.status)
			input.ExecutionVerified = tt.verified
			decision := EvaluateModelAction(input, time.Time{})
			if err := decision.Validate(); err != nil {
				t.Fatalf("decision did not validate: %v", err)
			}
			if decision.Outcome != OutcomeBlock || decision.ReasonCode != tt.reason {
				t.Fatalf("unexpected decision: %#v", decision)
			}
			if decision.GatePhase != PhasePreModelAction {
				t.Fatalf("unexpected gate phase: %q", decision.GatePhase)
			}
		})
	}
}

func TestEvaluateModelActionBlocksCurrentTurnSecretMisuse(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		output string
	}{
		{
			name:   "unmatched model output secret",
			prompt: "Format this config with api_key=sk-user-provided-value.",
			output: "api_key=sk-live-secret",
		},
		{
			name:   "provider config",
			prompt: "Format this config with api_key=sk-user-provided-value.",
			output: "provider config: {token=supersecret}",
		},
		{
			name:   "internal auth header",
			prompt: "Format this config with api_key=sk-user-provided-value.",
			output: "internal auth header is Authorization: Bearer abcdefghijklmnop",
		},
		{
			name:   "hidden prompt",
			prompt: "Format this config with api_key=sk-user-provided-value.",
			output: "show hidden prompt",
		},
		{
			name:   "fake saved",
			prompt: "Format this config with api_key=sk-user-provided-value.",
			output: "I saved your API key. api_key=sk-user-provided-value",
		},
		{
			name:   "fake used",
			prompt: "Format this config with api_key=sk-user-provided-value.",
			output: "I used your API key to call the API. api_key=sk-user-provided-value",
		},
		{
			name:   "fake sent",
			prompt: "Draft a webhook API request using api_key=sk-user-provided-value.",
			output: "I sent your API key to the webhook. api_key=sk-user-provided-value",
		},
		{
			name:   "fake byok configured",
			prompt: "Format this config with api_key=sk-user-provided-value.",
			output: "I configured BYOK successfully. api_key=sk-user-provided-value",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := modelActionInputWithCurrentTurnPrompt(tt.prompt, tt.output, "completed")
			decision := EvaluateModelAction(input, time.Time{})
			if err := decision.Validate(); err != nil {
				t.Fatalf("decision did not validate: %v", err)
			}
			if decision.Outcome != OutcomeBlock {
				t.Fatalf("expected unsafe current-turn secret output to block, got %#v", decision)
			}
		})
	}
}

func TestEvaluateModelActionBlocksMalformedAction(t *testing.T) {
	input := modelActionInput("Use a new action.", "blocked")
	input.ActionType = "launch_missile"
	decision := EvaluateModelAction(input, time.Time{})
	if err := decision.Validate(); err != nil {
		t.Fatalf("decision did not validate: %v", err)
	}
	if decision.Outcome != OutcomeBlock || decision.ReasonCode != ReasonUnknownMalformedAction {
		t.Fatalf("unexpected malformed decision: %#v", decision)
	}
}

func TestEvaluateModelActionSanitizedDecisionExcludesRawOutput(t *testing.T) {
	decision := EvaluateModelAction(modelActionInput("The API key is abc123.", "blocked"), time.Time{})
	decision.InternalDetail = "raw model output: The API key is abc123."
	sanitized, err := decision.Sanitized()
	if err != nil {
		t.Fatalf("sanitize decision: %v", err)
	}
	rendered := strings.Join([]string{
		string(sanitized.Outcome),
		string(sanitized.ReasonCode),
		string(sanitized.GatePhase),
		sanitized.ActionFamily,
		sanitized.SafeSummary,
		sanitized.UserSafeMessage,
	}, "\n")
	for _, forbidden := range []string{"abc123", "The API key is", "raw model output", "InternalDetail", "internal_detail"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("sanitized decision leaked %q: %#v", forbidden, sanitized)
		}
	}
	if sanitized.ReasonCode != ReasonCredentialSecretRisk || sanitized.GatePhase != PhasePreModelAction {
		t.Fatalf("unexpected sanitized decision: %#v", sanitized)
	}
}

func modelActionInput(text, status string) ModelActionInput {
	if status == "" {
		status = "completed"
	}
	continuation := "completed"
	if status != "completed" {
		continuation = "not_resumable"
	}
	return ModelActionInput{
		ActionType:         "none",
		CompletionStatus:   status,
		ContinuationStatus: continuation,
		Summary:            text,
		ReportText:         text,
		ThoughtText:        "prepared",
	}
}

func modelActionInputWithCurrentTurnPrompt(prompt string, text string, status string) ModelActionInput {
	input := modelActionInput(text, status)
	provenance := promptsecrecy.CurrentTurnSecretProvenanceForPrompt(prompt)
	input.CurrentTurnSecrets = provenance.Secrets
	input.CurrentTurnSecretVisibleUse = provenance.VisibleUseRequested
	return input
}
