package runtimegate

import (
	"strings"
	"testing"
	"time"
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
