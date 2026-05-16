package runtimegate

import (
	"errors"
	"fmt"
	"time"

	"xmilo/sidecar-go/internal/promptsecrecy"
)

type Outcome string

const (
	OutcomeAllow        Outcome = "allow"
	OutcomeBlock        Outcome = "block"
	OutcomeClarify      Outcome = "clarify"
	OutcomeConfirm      Outcome = "confirm"
	OutcomeSafeRedirect Outcome = "safe_redirect"
)

type ReasonCode string

const (
	ReasonNone                            ReasonCode = "none"
	ReasonHarmfulRequest                  ReasonCode = "harmful_request"
	ReasonPromptInjectionAuthoritySpoof   ReasonCode = "prompt_injection_authority_spoof"
	ReasonExternalContentAttemptedCommand ReasonCode = "external_content_attempted_command"
	ReasonMissingPermissionOrCapability   ReasonCode = "missing_permission_or_capability"
	ReasonMissingToolProof                ReasonCode = "missing_tool_proof"
	ReasonMissingProviderAccessRoute      ReasonCode = "missing_provider_access_route"
	ReasonMissingApproval                 ReasonCode = "missing_approval"
	ReasonUnsafeAutomation                ReasonCode = "unsafe_automation"
	ReasonDestructiveAction               ReasonCode = "destructive_action"
	ReasonPrivacySurveillanceRisk         ReasonCode = "privacy_surveillance_risk"
	ReasonCredentialSecretRisk            ReasonCode = "credential_secret_risk"
	ReasonCompletionEvidenceMissing       ReasonCode = "completion_evidence_missing"
	ReasonUnknownMalformedAction          ReasonCode = "unknown_malformed_action"
	ReasonUnboundedConsumptionRisk        ReasonCode = "unbounded_consumption_risk"
)

type Phase string

const (
	PhasePreTask             Phase = "pre_task"
	PhasePreContextInjection Phase = "pre_context_injection"
	PhasePreModelAction      Phase = "pre_model_action"
	PhasePreToolAction       Phase = "pre_tool_action"
	PhasePreCompletion       Phase = "pre_completion"
	PhasePreMemoryWrite      Phase = "pre_memory_write"
)

type Decision struct {
	Outcome          Outcome    `json:"outcome"`
	ReasonCode       ReasonCode `json:"reason_code"`
	GatePhase        Phase      `json:"gate_phase"`
	ActionFamily     string     `json:"action_family,omitempty"`
	ActionName       string     `json:"action_name,omitempty"`
	EvidenceRequired bool       `json:"evidence_required"`
	SourceTrustTier  *int       `json:"source_trust_tier,omitempty"`
	UserSafeMessage  string     `json:"user_safe_message,omitempty"`
	SafeSummary      string     `json:"safe_summary,omitempty"`
	CreatedAt        string     `json:"created_at,omitempty"`
	InternalDetail   string     `json:"-"`
}

type SanitizedDecision struct {
	Outcome          Outcome    `json:"outcome"`
	ReasonCode       ReasonCode `json:"reason_code"`
	GatePhase        Phase      `json:"gate_phase"`
	ActionFamily     string     `json:"action_family,omitempty"`
	ActionName       string     `json:"action_name,omitempty"`
	EvidenceRequired bool       `json:"evidence_required"`
	SourceTrustTier  *int       `json:"source_trust_tier,omitempty"`
	UserSafeMessage  string     `json:"user_safe_message,omitempty"`
	SafeSummary      string     `json:"safe_summary,omitempty"`
	CreatedAt        string     `json:"created_at,omitempty"`
}

var (
	validOutcomes = map[Outcome]struct{}{
		OutcomeAllow:        {},
		OutcomeBlock:        {},
		OutcomeClarify:      {},
		OutcomeConfirm:      {},
		OutcomeSafeRedirect: {},
	}

	validReasonCodes = map[ReasonCode]struct{}{
		ReasonNone:                            {},
		ReasonHarmfulRequest:                  {},
		ReasonPromptInjectionAuthoritySpoof:   {},
		ReasonExternalContentAttemptedCommand: {},
		ReasonMissingPermissionOrCapability:   {},
		ReasonMissingToolProof:                {},
		ReasonMissingProviderAccessRoute:      {},
		ReasonMissingApproval:                 {},
		ReasonUnsafeAutomation:                {},
		ReasonDestructiveAction:               {},
		ReasonPrivacySurveillanceRisk:         {},
		ReasonCredentialSecretRisk:            {},
		ReasonCompletionEvidenceMissing:       {},
		ReasonUnknownMalformedAction:          {},
		ReasonUnboundedConsumptionRisk:        {},
	}

	validPhases = map[Phase]struct{}{
		PhasePreTask:             {},
		PhasePreContextInjection: {},
		PhasePreModelAction:      {},
		PhasePreToolAction:       {},
		PhasePreCompletion:       {},
		PhasePreMemoryWrite:      {},
	}
)

func Outcomes() []Outcome {
	return []Outcome{
		OutcomeAllow,
		OutcomeBlock,
		OutcomeClarify,
		OutcomeConfirm,
		OutcomeSafeRedirect,
	}
}

func ReasonCodes() []ReasonCode {
	return []ReasonCode{
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
}

func Phases() []Phase {
	return []Phase{
		PhasePreTask,
		PhasePreContextInjection,
		PhasePreModelAction,
		PhasePreToolAction,
		PhasePreCompletion,
		PhasePreMemoryWrite,
	}
}

func ValidateOutcome(outcome Outcome) error {
	if _, ok := validOutcomes[outcome]; !ok {
		return fmt.Errorf("invalid_gate_outcome:%s", outcome)
	}
	return nil
}

func ValidateReasonCode(reason ReasonCode) error {
	if _, ok := validReasonCodes[reason]; !ok {
		return fmt.Errorf("invalid_gate_reason_code:%s", reason)
	}
	return nil
}

func ValidatePhase(phase Phase) error {
	if _, ok := validPhases[phase]; !ok {
		return fmt.Errorf("invalid_gate_phase:%s", phase)
	}
	return nil
}

func (d Decision) Validate() error {
	if err := ValidateOutcome(d.Outcome); err != nil {
		return err
	}
	if err := ValidateReasonCode(d.ReasonCode); err != nil {
		return err
	}
	if err := ValidatePhase(d.GatePhase); err != nil {
		return err
	}
	if d.Outcome == OutcomeAllow && d.ReasonCode != ReasonNone {
		return errors.New("allow_requires_none_reason_code")
	}
	if d.Outcome != OutcomeAllow && d.ReasonCode == ReasonNone {
		return fmt.Errorf("%s_requires_non_none_reason_code", d.Outcome)
	}
	return nil
}

func (d Decision) Sanitized() (SanitizedDecision, error) {
	if err := d.Validate(); err != nil {
		return SanitizedDecision{}, err
	}
	return SanitizedDecision{
		Outcome:          d.Outcome,
		ReasonCode:       d.ReasonCode,
		GatePhase:        d.GatePhase,
		ActionFamily:     d.ActionFamily,
		ActionName:       d.ActionName,
		EvidenceRequired: d.EvidenceRequired,
		SourceTrustTier:  d.SourceTrustTier,
		UserSafeMessage:  promptsecrecy.Redact(d.UserSafeMessage),
		SafeSummary:      promptsecrecy.Redact(d.SafeSummary),
		CreatedAt:        d.CreatedAt,
	}, nil
}

func NewDecision(outcome Outcome, reason ReasonCode, phase Phase, now time.Time) Decision {
	createdAt := ""
	if !now.IsZero() {
		createdAt = now.UTC().Format(time.RFC3339)
	}
	return Decision{
		Outcome:    outcome,
		ReasonCode: reason,
		GatePhase:  phase,
		CreatedAt:  createdAt,
	}
}
