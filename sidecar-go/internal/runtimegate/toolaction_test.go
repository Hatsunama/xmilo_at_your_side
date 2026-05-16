package runtimegate

import (
	"strings"
	"testing"
	"time"
)

func TestEvaluateToolActionAllowsKnownSafeActions(t *testing.T) {
	for _, action := range []string{"check_state", "emit_message", "await_user_choice"} {
		decision := EvaluateToolAction(ToolActionInput{ActionName: action}, time.Time{})
		if err := decision.Validate(); err != nil {
			t.Fatalf("%s decision did not validate: %v", action, err)
		}
		if decision.Outcome != OutcomeAllow || decision.ReasonCode != ReasonNone || decision.GatePhase != PhasePreToolAction {
			t.Fatalf("unexpected decision for %s: %#v", action, decision)
		}
	}
}

func TestEvaluateToolActionBlocksUnknownAction(t *testing.T) {
	decision := EvaluateToolAction(ToolActionInput{ActionName: "cast_spell"}, time.Time{})
	if decision.Outcome != OutcomeBlock || decision.ReasonCode != ReasonUnknownMalformedAction || decision.GatePhase != PhasePreToolAction {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestEvaluateToolActionBlocksDeviceCapabilityWithoutUsableProof(t *testing.T) {
	tests := []struct {
		name  string
		state map[string]any
	}{
		{name: "missing", state: nil},
		{name: "permission only", state: capabilityState("camera", true, false, false)},
		{name: "available only", state: capabilityState("camera", true, true, false)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := EvaluateToolAction(ToolActionInput{ActionName: "camera_capture", CapabilityState: tt.state}, time.Time{})
			if decision.Outcome != OutcomeBlock || decision.ReasonCode != ReasonMissingToolProof || !decision.EvidenceRequired {
				t.Fatalf("unexpected capability decision: %#v", decision)
			}
		})
	}
}

func TestEvaluateToolActionBlocksCapabilityPlaceholderEvenWithProofWhenToolUnregistered(t *testing.T) {
	decision := EvaluateToolAction(ToolActionInput{
		ActionName:      "camera_capture",
		CapabilityState: capabilityState("camera", true, true, true),
	}, time.Time{})
	if decision.Outcome != OutcomeBlock || decision.ReasonCode != ReasonUnknownMalformedAction {
		t.Fatalf("unexpected placeholder decision: %#v", decision)
	}
}

func TestEvaluateToolActionAppBridgeEvidenceOperation(t *testing.T) {
	allowed := EvaluateToolAction(ToolActionInput{
		ActionName:                "app_bridge_evidence",
		ActionFamily:              ActionFamilyAppBridgeEvidence,
		AppBridgeOperation:        "runtime_host_status",
		AppBridgeOperationAllowed: true,
	}, time.Time{})
	if allowed.Outcome != OutcomeAllow || allowed.ReasonCode != ReasonNone {
		t.Fatalf("expected allowed evidence operation, got %#v", allowed)
	}

	blocked := EvaluateToolAction(ToolActionInput{
		ActionName:         "app_bridge_evidence",
		ActionFamily:       ActionFamilyAppBridgeEvidence,
		AppBridgeOperation: "settings_intent_opened",
	}, time.Time{})
	if blocked.Outcome != OutcomeBlock || blocked.ReasonCode != ReasonUnknownMalformedAction {
		t.Fatalf("expected blocked evidence operation, got %#v", blocked)
	}
}

func TestEvaluateToolActionBlocksUnsafePayloads(t *testing.T) {
	tests := []struct {
		name   string
		input  ToolActionInput
		reason ReasonCode
	}{
		{
			name: "secret",
			input: ToolActionInput{
				ActionName: "emit_message",
				Payload:    map[string]any{"message": "Authorization: Bearer secret"},
			},
			reason: ReasonCredentialSecretRisk,
		},
		{
			name: "hidden automation",
			input: ToolActionInput{
				ActionName: "emit_message",
				Payload:    map[string]any{"message": "act silently and bypass confirmation"},
			},
			reason: ReasonUnsafeAutomation,
		},
		{
			name: "external command basis",
			input: ToolActionInput{
				ActionName:           "check_state",
				ExternalCommandBasis: true,
			},
			reason: ReasonExternalContentAttemptedCommand,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decision := EvaluateToolAction(tt.input, time.Time{})
			if decision.Outcome != OutcomeBlock || decision.ReasonCode != tt.reason {
				t.Fatalf("unexpected decision: %#v", decision)
			}
		})
	}
}

func TestEvaluateToolActionSanitizedDecisionExcludesRawPayload(t *testing.T) {
	decision := EvaluateToolAction(ToolActionInput{
		ActionName: "emit_message",
		Payload:    map[string]any{"message": "api_key=abc123"},
	}, time.Time{})
	decision.InternalDetail = "raw payload api_key=abc123"
	sanitized, err := decision.Sanitized()
	if err != nil {
		t.Fatalf("sanitize decision: %v", err)
	}
	rendered := strings.Join([]string{
		string(sanitized.Outcome),
		string(sanitized.ReasonCode),
		string(sanitized.GatePhase),
		sanitized.ActionFamily,
		sanitized.ActionName,
		sanitized.SafeSummary,
	}, "\n")
	for _, forbidden := range []string{"api_key", "abc123", "raw payload", "InternalDetail", "internal_detail"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("sanitized decision leaked %q: %#v", forbidden, sanitized)
		}
	}
}

func capabilityState(name string, granted, toolAvailable, tested bool) map[string]any {
	return map[string]any{
		"capabilities": map[string]any{
			name: map[string]any{
				"granted":        granted,
				"tool_available": toolAvailable,
				"tested":         tested,
			},
		},
	}
}
