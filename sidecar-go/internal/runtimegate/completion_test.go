package runtimegate

import (
	"strings"
	"testing"
	"time"
)

func TestEvaluateCompletionAllowsNonCompletionAndVerifiedCompletion(t *testing.T) {
	tests := []CompletionInput{
		{ClaimsCompletion: false},
		{ClaimsCompletion: true, EvidenceVerified: true, ProofClass: "app_bridge_verified"},
	}
	for _, input := range tests {
		decision := EvaluateCompletion(input, time.Time{})
		if err := decision.Validate(); err != nil {
			t.Fatalf("decision did not validate: %v", err)
		}
		if decision.Outcome != OutcomeAllow || decision.ReasonCode != ReasonNone || decision.GatePhase != PhasePreCompletion {
			t.Fatalf("unexpected decision for %#v: %#v", input, decision)
		}
	}
}

func TestEvaluateCompletionBlocksMissingEvidence(t *testing.T) {
	decision := EvaluateCompletion(CompletionInput{
		ClaimsCompletion: true,
		EvidenceVerified: false,
		ProofClass:       "app_bridge_verified",
		BlockingReason:   "completion_evidence_missing:app_bridge_verified",
	}, time.Time{})
	if err := decision.Validate(); err != nil {
		t.Fatalf("decision did not validate: %v", err)
	}
	if decision.Outcome != OutcomeBlock || decision.ReasonCode != ReasonCompletionEvidenceMissing || decision.GatePhase != PhasePreCompletion {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if !decision.EvidenceRequired {
		t.Fatalf("expected evidence required")
	}
}

func TestEvaluateCompletionSanitizedDecisionExcludesRawOutput(t *testing.T) {
	decision := EvaluateCompletion(CompletionInput{
		ClaimsCompletion: true,
		EvidenceVerified: false,
		ProofClass:       "app_bridge_verified",
	}, time.Time{})
	decision.InternalDetail = "raw model said: done with api_key=abc123"
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
	}, "\n")
	for _, forbidden := range []string{"api_key", "abc123", "raw model", "InternalDetail", "internal_detail"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("sanitized decision leaked %q: %#v", forbidden, sanitized)
		}
	}
}
