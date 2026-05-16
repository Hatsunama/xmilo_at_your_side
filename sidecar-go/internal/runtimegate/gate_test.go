package runtimegate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xmilo/sidecar-go/internal/promptsecrecy"
)

func TestApprovedOutcomesValidate(t *testing.T) {
	want := []Outcome{
		OutcomeAllow,
		OutcomeBlock,
		OutcomeClarify,
		OutcomeConfirm,
		OutcomeSafeRedirect,
	}
	if got := Outcomes(); !sameOutcomes(got, want) {
		t.Fatalf("outcome contract drifted: got %#v want %#v", got, want)
	}
	for _, outcome := range want {
		if err := ValidateOutcome(outcome); err != nil {
			t.Fatalf("approved outcome %q did not validate: %v", outcome, err)
		}
	}
}

func TestUnknownOutcomeFails(t *testing.T) {
	if err := ValidateOutcome("pause_and_guess"); err == nil || err.Error() != "invalid_gate_outcome:pause_and_guess" {
		t.Fatalf("expected invalid outcome error, got %v", err)
	}
}

func TestApprovedReasonCodesValidate(t *testing.T) {
	want := []ReasonCode{
		ReasonNone,
		ReasonHarmfulRequest,
		ReasonPromptInjectionAuthoritySpoof,
		ReasonExternalContentAttemptedCommand,
		ReasonMissingPermissionOrCapability,
		ReasonMissingToolProof,
		ReasonMissingProviderAccessRoute,
		ReasonMissingApproval,
		ReasonUnsafeAutomation,
		ReasonDestructiveAction,
		ReasonPrivacySurveillanceRisk,
		ReasonCredentialSecretRisk,
		ReasonCompletionEvidenceMissing,
		ReasonUnknownMalformedAction,
		ReasonUnboundedConsumptionRisk,
	}
	if got := ReasonCodes(); !sameReasonCodes(got, want) {
		t.Fatalf("reason-code contract drifted: got %#v want %#v", got, want)
	}
	for _, reason := range want {
		if err := ValidateReasonCode(reason); err != nil {
			t.Fatalf("approved reason code %q did not validate: %v", reason, err)
		}
	}
}

func TestUnknownReasonCodeFails(t *testing.T) {
	if err := ValidateReasonCode("misc_badness"); err == nil || err.Error() != "invalid_gate_reason_code:misc_badness" {
		t.Fatalf("expected invalid reason code error, got %v", err)
	}
}

func TestApprovedGatePhasesValidate(t *testing.T) {
	want := []Phase{
		PhasePreTask,
		PhasePreContextInjection,
		PhasePreModelAction,
		PhasePreToolAction,
		PhasePreCompletion,
		PhasePreMemoryWrite,
	}
	if got := Phases(); !samePhases(got, want) {
		t.Fatalf("phase contract drifted: got %#v want %#v", got, want)
	}
	for _, phase := range want {
		if err := ValidatePhase(phase); err != nil {
			t.Fatalf("approved phase %q did not validate: %v", phase, err)
		}
	}
}

func TestUnknownGatePhaseFails(t *testing.T) {
	if err := ValidatePhase("after_everything"); err == nil || err.Error() != "invalid_gate_phase:after_everything" {
		t.Fatalf("expected invalid phase error, got %v", err)
	}
}

func TestAllowWithNoneIsValid(t *testing.T) {
	decision := NewDecision(OutcomeAllow, ReasonNone, PhasePreTask, time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC))
	if err := decision.Validate(); err != nil {
		t.Fatalf("allow + none should be valid: %v", err)
	}
}

func TestAllowWithBlockingReasonIsInvalid(t *testing.T) {
	decision := NewDecision(OutcomeAllow, ReasonHarmfulRequest, PhasePreTask, time.Time{})
	if err := decision.Validate(); err == nil || err.Error() != "allow_requires_none_reason_code" {
		t.Fatalf("expected allow reason error, got %v", err)
	}
}

func TestBlockingOutcomesRequireNonNoneReason(t *testing.T) {
	for _, outcome := range []Outcome{OutcomeBlock, OutcomeClarify, OutcomeConfirm, OutcomeSafeRedirect} {
		decision := NewDecision(outcome, ReasonNone, PhasePreTask, time.Time{})
		if err := decision.Validate(); err == nil || err.Error() != string(outcome)+"_requires_non_none_reason_code" {
			t.Fatalf("expected non-none reason error for %q, got %v", outcome, err)
		}
	}
}

func TestSanitizedOutputExcludesInternalDetails(t *testing.T) {
	tier := 5
	decision := NewDecision(OutcomeBlock, ReasonCredentialSecretRisk, PhasePreToolAction, time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC))
	decision.ActionFamily = "tool"
	decision.ActionName = "send_message"
	decision.EvidenceRequired = true
	decision.SourceTrustTier = &tier
	decision.UserSafeMessage = "I cannot use secrets or credentials that way."
	decision.SafeSummary = "Blocked credential/secret risk."
	decision.InternalDetail = "raw provider header Authorization: Bearer secret"

	sanitized, err := decision.Sanitized()
	if err != nil {
		t.Fatalf("sanitize decision: %v", err)
	}
	raw, err := json.Marshal(sanitized)
	if err != nil {
		t.Fatalf("marshal sanitized decision: %v", err)
	}
	out := string(raw)
	for _, forbidden := range []string{"InternalDetail", "internal_detail", "Authorization", "Bearer", "raw provider header"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("sanitized output leaked %q: %s", forbidden, out)
		}
	}
	for _, required := range []string{"outcome", "reason_code", "gate_phase", "action_family", "action_name", "evidence_required", "source_trust_tier", "user_safe_message", "safe_summary", "created_at"} {
		if !strings.Contains(out, required) {
			t.Fatalf("sanitized output missing stable field %q: %s", required, out)
		}
	}
}

func TestReasonCodeStringsRemainStable(t *testing.T) {
	stable := map[ReasonCode]string{
		ReasonNone:                            "none",
		ReasonHarmfulRequest:                  "harmful_request",
		ReasonPromptInjectionAuthoritySpoof:   "prompt_injection_authority_spoof",
		ReasonExternalContentAttemptedCommand: "external_content_attempted_command",
		ReasonMissingPermissionOrCapability:   "missing_permission_or_capability",
		ReasonMissingToolProof:                "missing_tool_proof",
		ReasonMissingProviderAccessRoute:      "missing_provider_access_route",
		ReasonMissingApproval:                 "missing_approval",
		ReasonUnsafeAutomation:                "unsafe_automation",
		ReasonDestructiveAction:               "destructive_action",
		ReasonPrivacySurveillanceRisk:         "privacy_surveillance_risk",
		ReasonCredentialSecretRisk:            "credential_secret_risk",
		ReasonCompletionEvidenceMissing:       "completion_evidence_missing",
		ReasonUnknownMalformedAction:          "unknown_malformed_action",
		ReasonUnboundedConsumptionRisk:        "unbounded_consumption_risk",
	}
	for reason, want := range stable {
		if string(reason) != want {
			t.Fatalf("reason string drifted for %q: got %q want %q", reason, string(reason), want)
		}
	}
}

func TestRuntimeContractManifestMatchesGateConstants(t *testing.T) {
	manifestPath := filepath.Join("..", "..", "..", "shared", "contracts", "runtime_contracts.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read runtime contract manifest: %v", err)
	}
	var manifest struct {
		SafetyDecisionContract struct {
			Outcomes        []Outcome    `json:"outcomes"`
			ReasonCodes     []ReasonCode `json:"reason_codes"`
			GatePhases      []Phase      `json:"gate_phases"`
			SanitizedFields []string     `json:"sanitized_fields"`
			ForbiddenFields []string     `json:"forbidden_visible_fields"`
		} `json:"safety_decision_contract"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode runtime contract manifest: %v", err)
	}
	if !sameOutcomes(manifest.SafetyDecisionContract.Outcomes, Outcomes()) {
		t.Fatalf("manifest outcomes drifted: got %#v want %#v", manifest.SafetyDecisionContract.Outcomes, Outcomes())
	}
	if !sameReasonCodes(manifest.SafetyDecisionContract.ReasonCodes, ReasonCodes()) {
		t.Fatalf("manifest reason codes drifted: got %#v want %#v", manifest.SafetyDecisionContract.ReasonCodes, ReasonCodes())
	}
	if !samePhases(manifest.SafetyDecisionContract.GatePhases, Phases()) {
		t.Fatalf("manifest phases drifted: got %#v want %#v", manifest.SafetyDecisionContract.GatePhases, Phases())
	}
	for _, field := range []string{"outcome", "reason_code", "gate_phase", "evidence_required", "safe_summary"} {
		if !containsString(manifest.SafetyDecisionContract.SanitizedFields, field) {
			t.Fatalf("manifest sanitized fields missing %q: %#v", field, manifest.SafetyDecisionContract.SanitizedFields)
		}
	}
	for _, field := range promptsecrecy.ForbiddenVisibleFields() {
		if !containsString(manifest.SafetyDecisionContract.ForbiddenFields, field) {
			t.Fatalf("manifest forbidden fields missing %q: %#v", field, manifest.SafetyDecisionContract.ForbiddenFields)
		}
	}
}

func sameOutcomes(a, b []Outcome) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sameReasonCodes(a, b []ReasonCode) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func samePhases(a, b []Phase) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
