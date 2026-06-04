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

func TestRuntimeContractManifestContainsPhase18HandoffSections(t *testing.T) {
	manifestPath := filepath.Join("..", "..", "..", "shared", "contracts", "runtime_contracts.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read runtime contract manifest: %v", err)
	}
	var manifest struct {
		Phase18ContractHandoff map[string]any  `json:"phase18_contract_handoff"`
		HTTPRouteMatrix        []contractRoute `json:"http_route_matrix"`
		WebSocketEventContract map[string]any  `json:"websocket_event_contract"`
		TaskRuntimeStatusEnums struct {
			TaskStatus            []string       `json:"task_status"`
			CompletionStatus      []string       `json:"completion_status"`
			ContinuationStatus    []string       `json:"continuation_status"`
			ActionType            []string       `json:"action_type"`
			ResumeCheckpointState []string       `json:"resume_checkpoint_state"`
			Semantics             map[string]any `json:"semantics"`
		} `json:"task_runtime_status_enums"`
		ResumeCheckpointRenderingContract struct {
			DisplayableFields             []string `json:"displayable_fields"`
			InternalFields                []string `json:"internal_fields"`
			AllowedNextStepType           []string `json:"allowed_next_step_type"`
			RenderCheckState              string   `json:"render_check_state"`
			RenderEmitMessage             string   `json:"render_emit_message"`
			RenderAwaitUserChoice         string   `json:"render_await_user_choice"`
			RenderUnsupportedOrExpired    string   `json:"render_unsupported_or_expired"`
			AppMustNotInventResumeActions bool     `json:"app_must_not_invent_resume_actions"`
			ExplicitResumeActionRoute     string   `json:"explicit_resume_action_route"`
		} `json:"resume_checkpoint_rendering_contract"`
		EvidenceBoundaryRenderingContract struct {
			VerifiedFacts           map[string]any `json:"verified_facts"`
			ExecutedSteps           map[string]any `json:"executed_steps"`
			UnverifiedClaims        map[string]any `json:"unverified_claims"`
			BlockedReasons          map[string]any `json:"blocked_reasons"`
			NextVerificationStep    map[string]any `json:"next_verification_step"`
			AppBridgeProofDisplay   map[string]any `json:"app_bridge_proof_display"`
			EvidenceMissingBehavior string         `json:"evidence_missing_behavior"`
			EvidencePresentBehavior string         `json:"evidence_present_behavior"`
			NoUnverifiedToCompleted bool           `json:"no_unverified_to_completed"`
		} `json:"evidence_boundary_rendering_contract"`
		CompletionProofContract struct {
			CompletedMayRenderDoneWhen         []string `json:"completed_may_render_done_when"`
			AttemptedUnverifiedRendering       string   `json:"attempted_unverified_rendering"`
			RealActionProofRequired            []string `json:"real_action_proof_required"`
			ProviderModelResponseProofRequired []string `json:"provider_model_response_proof_required"`
			CapabilityToolClaimProofRequired   []string `json:"capability_tool_claim_proof_required"`
			ProofMissingBehavior               string   `json:"proof_missing_behavior"`
			StableErrorRelation                string   `json:"stable_error_relation"`
		} `json:"completion_proof_contract"`
		MemoryContract struct {
			AllowedMemoryClassIdentifiers []string `json:"allowed_memory_class_identifiers"`
			MemoryIntentStatuses          []string `json:"memory_intent_statuses"`
			QuarantineStates              []string `json:"quarantine_states"`
			SuppressionStates             []string `json:"suppression_states"`
		} `json:"memory_contract"`
		StorageResponsibilityBoundaries map[string]any      `json:"storage_responsibility_boundaries"`
		StableUIErrorCodes              map[string][]string `json:"stable_ui_error_codes"`
		ContractRequiredSections        []string            `json:"contract_required_sections"`
		DeferredNonGoals                []string            `json:"deferred_non_goals"`
		RuntimeEvents                   map[string]any      `json:"runtime_events"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode runtime contract manifest: %v", err)
	}

	requireMapSection(t, "phase18_contract_handoff", manifest.Phase18ContractHandoff, "contract_artifact", "human_packet_artifact", "do_not_guess")
	requireMapSection(t, "websocket_event_contract", manifest.WebSocketEventContract, "transport", "envelope_required_fields", "event_types", "source_truth_scopes")
	requireMapSection(t, "storage_responsibility_boundaries", manifest.StorageResponsibilityBoundaries, "sidecar_sqlite", "app_local", "relay_hosted", "not_durable_runtime_truth")
	requireMapSection(t, "runtime_events", manifest.RuntimeEvents, "runtime.ready", "task.completed", "task.stuck", "report.ready")

	if len(manifest.HTTPRouteMatrix) == 0 {
		t.Fatal("http_route_matrix missing routes")
	}
	for _, want := range []contractRoute{
		{Component: "sidecar", Route: "/task/start", Method: "POST"},
		{Component: "sidecar", Route: "/state", Method: "GET"},
		{Component: "sidecar", Route: "/ws", Method: "GET"},
		{Component: "relay", Route: "/session/start", Method: "POST"},
		{Component: "relay", Route: "/llm/turn", Method: "POST"},
		{Component: "relay", Route: "/report/settings", Method: "POST"},
	} {
		if !containsRoute(manifest.HTTPRouteMatrix, want) {
			t.Fatalf("http_route_matrix missing %#v", want)
		}
	}
	for _, route := range manifest.HTTPRouteMatrix {
		if route.AuthBoundary == "" || route.RequestSchemaSummary == "" || route.ResponseSchemaSummary == "" || len(route.StableErrorCodes) == 0 || route.UserFacingTruthNote == "" {
			t.Fatalf("route contract incomplete for %#v", route)
		}
	}

	requireStringSet(t, "task_status", manifest.TaskRuntimeStatusEnums.TaskStatus, "queued", "running", "awaiting_user_choice", "interrupted", "stuck", "blocked", "cancelled", "completed")
	requireStringSet(t, "completion_status", manifest.TaskRuntimeStatusEnums.CompletionStatus, "completed", "blocked", "needs_user_choice", "attempted_unverified")
	requireStringSet(t, "continuation_status", manifest.TaskRuntimeStatusEnums.ContinuationStatus, "completed", "blocked", "awaiting_user_choice", "needs_check", "resumable", "not_resumable")
	requireStringSet(t, "action_type", manifest.TaskRuntimeStatusEnums.ActionType, "none", "await_user_choice", "emit_message", "resume_checkpoint", "check_state")
	requireStringSet(t, "resume_checkpoint_state", manifest.TaskRuntimeStatusEnums.ResumeCheckpointState, "resumable", "blocked", "awaiting_user_choice", "completed", "expired", "invalid")
	requireMapSection(t, "task_runtime_status_enums.semantics", manifest.TaskRuntimeStatusEnums.Semantics, "completed", "blocked", "attempted_unverified", "stuck", "resumable")
	requireStringSet(t, "resume_checkpoint_rendering_contract.displayable_fields", manifest.ResumeCheckpointRenderingContract.DisplayableFields, "task_id", "attempt_id", "next_step_type", "choices", "evidence_boundary")
	requireStringSet(t, "resume_checkpoint_rendering_contract.internal_fields", manifest.ResumeCheckpointRenderingContract.InternalFields, "context_hash", "raw_prompt", "provider_config", "auth_headers")
	requireStringSet(t, "resume_checkpoint_rendering_contract.allowed_next_step_type", manifest.ResumeCheckpointRenderingContract.AllowedNextStepType, "check_state", "emit_message", "await_user_choice", "none")
	requireNonEmpty(t, "resume_checkpoint_rendering_contract.render_check_state", manifest.ResumeCheckpointRenderingContract.RenderCheckState)
	requireNonEmpty(t, "resume_checkpoint_rendering_contract.render_emit_message", manifest.ResumeCheckpointRenderingContract.RenderEmitMessage)
	requireNonEmpty(t, "resume_checkpoint_rendering_contract.render_await_user_choice", manifest.ResumeCheckpointRenderingContract.RenderAwaitUserChoice)
	requireNonEmpty(t, "resume_checkpoint_rendering_contract.render_unsupported_or_expired", manifest.ResumeCheckpointRenderingContract.RenderUnsupportedOrExpired)
	if !manifest.ResumeCheckpointRenderingContract.AppMustNotInventResumeActions {
		t.Fatal("resume_checkpoint_rendering_contract must forbid invented resume actions")
	}
	if manifest.ResumeCheckpointRenderingContract.ExplicitResumeActionRoute != "/task/resume_queue" {
		t.Fatalf("resume checkpoint explicit route = %q, want /task/resume_queue", manifest.ResumeCheckpointRenderingContract.ExplicitResumeActionRoute)
	}
	requireMapSection(t, "evidence_boundary_rendering_contract.verified_facts", manifest.EvidenceBoundaryRenderingContract.VerifiedFacts, "field", "rendering_rule")
	requireMapSection(t, "evidence_boundary_rendering_contract.executed_steps", manifest.EvidenceBoundaryRenderingContract.ExecutedSteps, "field", "rendering_rule")
	requireMapSection(t, "evidence_boundary_rendering_contract.unverified_claims", manifest.EvidenceBoundaryRenderingContract.UnverifiedClaims, "field", "rendering_rule")
	requireMapSection(t, "evidence_boundary_rendering_contract.blocked_reasons", manifest.EvidenceBoundaryRenderingContract.BlockedReasons, "field", "rendering_rule")
	requireMapSection(t, "evidence_boundary_rendering_contract.next_verification_step", manifest.EvidenceBoundaryRenderingContract.NextVerificationStep, "field", "rendering_rule")
	requireMapSection(t, "evidence_boundary_rendering_contract.app_bridge_proof_display", manifest.EvidenceBoundaryRenderingContract.AppBridgeProofDisplay, "fields", "rendering_rule")
	requireNonEmpty(t, "evidence_boundary_rendering_contract.evidence_missing_behavior", manifest.EvidenceBoundaryRenderingContract.EvidenceMissingBehavior)
	requireNonEmpty(t, "evidence_boundary_rendering_contract.evidence_present_behavior", manifest.EvidenceBoundaryRenderingContract.EvidencePresentBehavior)
	if !manifest.EvidenceBoundaryRenderingContract.NoUnverifiedToCompleted {
		t.Fatal("evidence_boundary_rendering_contract must forbid unverified claims becoming completed UI")
	}
	requireStringSet(t, "completion_proof_contract.completed_may_render_done_when", manifest.CompletionProofContract.CompletedMayRenderDoneWhen, "completion_status is completed", "continuation_status is completed")
	requireNonEmpty(t, "completion_proof_contract.attempted_unverified_rendering", manifest.CompletionProofContract.AttemptedUnverifiedRendering)
	requireStringSet(t, "completion_proof_contract.real_action_proof_required", manifest.CompletionProofContract.RealActionProofRequired, "expected_check must pass when present")
	requireStringSet(t, "completion_proof_contract.provider_model_response_proof_required", manifest.CompletionProofContract.ProviderModelResponseProofRequired, "provider/model response proves text generation only")
	requireStringSet(t, "completion_proof_contract.capability_tool_claim_proof_required", manifest.CompletionProofContract.CapabilityToolClaimProofRequired, "permission/setup success must not be inferred from button taps or labels")
	requireNonEmpty(t, "completion_proof_contract.proof_missing_behavior", manifest.CompletionProofContract.ProofMissingBehavior)
	requireNonEmpty(t, "completion_proof_contract.stable_error_relation", manifest.CompletionProofContract.StableErrorRelation)
	requireStringSet(t, "allowed_memory_class_identifiers", manifest.MemoryContract.AllowedMemoryClassIdentifiers, "durable_user_preference", "task_continuity", "quarantined_suppressed")
	requireStringSet(t, "memory_intent_statuses", manifest.MemoryContract.MemoryIntentStatuses, "allowed", "needs_confirmation", "blocked", "quarantined", "suppressed")
	requireStringSet(t, "quarantine_states", manifest.MemoryContract.QuarantineStates, "clean", "quarantined", "blocked", "unknown")
	requireStringSet(t, "suppression_states", manifest.MemoryContract.SuppressionStates, "active", "suppressed", "demoted", "rolled_back")

	for _, errorGroup := range []string{"task_start", "command_submit", "task_current", "task_choice", "interrupt_cancel", "resume_queue", "runtime_evidence_app_bridge", "capability_state", "context_set", "report_settings", "storage_stats", "auth_session_refresh", "checkpoint_action", "generic"} {
		if len(manifest.StableUIErrorCodes[errorGroup]) == 0 {
			t.Fatalf("stable_ui_error_codes missing %q", errorGroup)
		}
	}

	requireStringSet(t, "contract_required_sections", manifest.ContractRequiredSections,
		"phase18_contract_handoff",
		"http_route_matrix",
		"websocket_event_contract",
		"runtime_events",
		"task_runtime_status_enums",
		"resume_checkpoint_rendering_contract",
		"evidence_boundary_rendering_contract",
		"completion_proof_contract",
		"bounded_consolidation_contract",
		"memory_contract",
		"storage_responsibility_boundaries",
		"stable_ui_error_codes",
		"deferred_non_goals",
		"safety_decision_contract",
		"app_bridge_evidence_fields",
		"provider_context_dtos",
	)
	requireStringSet(t, "deferred_non_goals", manifest.DeferredNonGoals,
		"no app UI implementation in Mind lane",
		"no candidate extraction in this step",
		"no LLM reflection in this step",
		"no direct memory promotion in this step",
		"no rollback UI in this step",
		"no App Build rendering implementation in this step",
		"no human markdown handoff packet",
		"no live phone validation",
		"no website/hosting/DNS/provider work",
		"no private admin UI",
		"no invented TypeScript semantics",
		"no App Build guessing beyond JSON contract",
	)
}

func TestBoundedConsolidationContractPresent(t *testing.T) {
	manifestPath := filepath.Join("..", "..", "..", "shared", "contracts", "runtime_contracts.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read runtime contract manifest: %v", err)
	}
	var manifest struct {
		BoundedConsolidationContract struct {
			Scope                   string   `json:"scope"`
			RunStatuses             []string `json:"run_statuses"`
			RunLedgerFields         []string `json:"run_ledger_fields"`
			OutputClasses           []string `json:"output_classes"`
			CurrentSummaryOnlyRules []string `json:"current_summary_only_rules"`
			NonMutationRules        []string `json:"non_mutation_rules"`
			AppRenderingRules       []string `json:"app_rendering_rules"`
			ExplicitDeferrals       []string `json:"explicit_deferrals"`
		} `json:"bounded_consolidation_contract"`
		ContractRequiredSections []string `json:"contract_required_sections"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode runtime contract manifest: %v", err)
	}
	requireNonEmpty(t, "bounded_consolidation_contract.scope", manifest.BoundedConsolidationContract.Scope)
	requireStringSet(t, "bounded_consolidation_contract.run_statuses", manifest.BoundedConsolidationContract.RunStatuses,
		"deferred_active_task",
		"running",
		"completed_summary_only",
		"failed_safe",
	)
	requireStringSet(t, "bounded_consolidation_contract.run_ledger_fields", manifest.BoundedConsolidationContract.RunLedgerFields,
		"run_id",
		"archive_date",
		"trigger",
		"status",
		"input_task_history_count",
		"archive_record_id",
		"summary_record_count",
		"candidate_count",
		"quarantined_count",
		"suppressed_count",
		"active_task_id",
		"deferred_reason",
		"error_code",
	)
	requireStringSet(t, "bounded_consolidation_contract.output_classes", manifest.BoundedConsolidationContract.OutputClasses,
		"archive_summary",
	)
	requireStringSet(t, "bounded_consolidation_contract.current_summary_only_rules", manifest.BoundedConsolidationContract.CurrentSummaryOnlyRules,
		"candidate_count must remain 0 in the current approved path",
		"archive-derived claims must not become current runtime truth",
	)
	requireStringSet(t, "bounded_consolidation_contract.non_mutation_rules", manifest.BoundedConsolidationContract.NonMutationRules,
		"must not delete active tasks",
		"must not mutate active task slots",
		"must not delete active memory",
		"must not mutate canon or policy records",
		"must not directly promote LLM reflection output",
		"must not mark failed runs completed",
	)
	requireStringSet(t, "bounded_consolidation_contract.app_rendering_rules", manifest.BoundedConsolidationContract.AppRenderingRules,
		"future-safe only until App Build is assigned a rendering surface",
		"App Build must not treat archive-derived claims as current truth",
		"App Build must not invent candidate, promotion, quarantine, suppression, or rollback semantics",
	)
	requireStringSet(t, "bounded_consolidation_contract.explicit_deferrals", manifest.BoundedConsolidationContract.ExplicitDeferrals,
		"candidate extraction",
		"LLM reflection",
		"memory promotion",
		"rollback UI",
		"App Build rendering",
	)
	requireStringSet(t, "contract_required_sections", manifest.ContractRequiredSections, "bounded_consolidation_contract")
}

type contractRoute struct {
	Component             string   `json:"component"`
	Route                 string   `json:"route"`
	Method                string   `json:"method"`
	AuthBoundary          string   `json:"auth_boundary"`
	RequestSchemaSummary  string   `json:"request_schema_summary"`
	ResponseSchemaSummary string   `json:"response_schema_summary"`
	StableErrorCodes      []string `json:"stable_error_codes"`
	UserFacingTruthNote   string   `json:"user_facing_truth_note"`
}

func containsRoute(routes []contractRoute, target contractRoute) bool {
	for _, route := range routes {
		if route.Component == target.Component && route.Route == target.Route && route.Method == target.Method {
			return true
		}
	}
	return false
}

func requireMapSection(t *testing.T, name string, section map[string]any, keys ...string) {
	t.Helper()
	if len(section) == 0 {
		t.Fatalf("%s missing or empty", name)
	}
	for _, key := range keys {
		if _, ok := section[key]; !ok {
			t.Fatalf("%s missing key %q", name, key)
		}
	}
}

func requireStringSet(t *testing.T, name string, values []string, required ...string) {
	t.Helper()
	if len(values) == 0 {
		t.Fatalf("%s missing or empty", name)
	}
	for _, value := range required {
		if !containsString(values, value) {
			t.Fatalf("%s missing %q: %#v", name, value, values)
		}
	}
}

func requireNonEmpty(t *testing.T, name, value string) {
	t.Helper()
	if strings.TrimSpace(value) == "" {
		t.Fatalf("%s missing or empty", name)
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
