package tasks

import (
	"fmt"
	"strings"
	"time"

	"xmilo/sidecar-go/internal/runtime"
	"xmilo/sidecar-go/shared/contracts"
)

const (
	proofModelTextOnly             = "model_text_only"
	proofRuntimeStateVerified      = "runtime_state_verified"
	proofSidecarActionVerified     = "sidecar_action_verified"
	proofAppBridgeVerified         = "app_bridge_verified"
	proofProviderResponseVerified  = "provider_response_verified"
	proofExternalEffectUnverified  = "external_effect_unverified"
	proofUserConfirmationRequired  = "user_confirmation_required"
	proofUnsupportedNoProofSurface = "unsupported_no_proof_surface"
	proofPartialEffectVerified     = "partial_effect_verified"

	AppBridgeEvidenceFreshness = 5 * time.Minute
)

func (e *Engine) enforceCompletionEvidence(task *runtime.TaskSnapshot, checkpoint *runtime.ResumeCheckpoint, resp contracts.RelayTurnResponse) contracts.RelayTurnResponse {
	if task == nil {
		return resp
	}
	boundary := e.ensureEvidenceBoundary(task, checkpoint)
	evidence := completionEvidenceFor(task, checkpoint, resp, boundary, time.Now().UTC())
	if boundary != nil {
		boundary.CompletionEvidence = evidence
	}
	if !claimsCompletion(resp) {
		return resp
	}
	if evidence.Verified {
		return resp
	}

	reason := evidence.BlockingReason
	if strings.TrimSpace(reason) == "" {
		reason = "completion_evidence_missing:" + evidence.ProofClass
	}
	resp.CompletionStatus = "blocked"
	resp.ContinuationStatus = "blocked"
	resp.NextBlocker = reason
	resp.ExecutionResult = &contracts.ExecutionResult{
		Status:         "blocked",
		Verified:       false,
		ResultSummary:  evidence.Summary,
		BlockingReason: reason,
	}
	if boundary != nil {
		boundary.BlockedReasons = appendUnique(boundary.BlockedReasons, reason)
		if strings.TrimSpace(resp.Summary) != "" {
			boundary.UnverifiedClaims = appendUnique(boundary.UnverifiedClaims, resp.Summary)
		}
		if strings.TrimSpace(resp.ReportText) != "" {
			boundary.UnverifiedClaims = appendUnique(boundary.UnverifiedClaims, resp.ReportText)
		}
	}
	return resp
}

func completionEvidenceFor(task *runtime.TaskSnapshot, checkpoint *runtime.ResumeCheckpoint, resp contracts.RelayTurnResponse, boundary *runtime.EvidenceBoundary, checkedAt time.Time) *runtime.CompletionEvidence {
	required := requiredProofClass(task, checkpoint, resp)
	evidence := &runtime.CompletionEvidence{
		ProofClass:            required,
		RequiredForCompletion: true,
		Verified:              false,
		Source:                "sidecar_completion_gate",
		Summary:               completionEvidenceSummary(required, false),
		BlockingReason:        "completion_evidence_missing:" + required,
		CheckedAt:             checkedAt.Format(time.RFC3339),
	}

	if !claimsCompletion(resp) {
		evidence.RequiredForCompletion = false
		evidence.BlockingReason = ""
		evidence.Summary = "Completion evidence was not required because the planner did not claim completion."
		return evidence
	}

	if required == proofModelTextOnly {
		evidence.Verified = true
		evidence.BlockingReason = ""
		evidence.Summary = completionEvidenceSummary(required, true)
		return evidence
	}

	if resp.ExecutionResult != nil {
		status := strings.ToLower(strings.TrimSpace(resp.ExecutionResult.Status))
		if status == "partial" {
			evidence.ProofClass = proofPartialEffectVerified
			evidence.Summary = completionEvidenceSummary(proofPartialEffectVerified, false)
			evidence.BlockingReason = "completion_evidence_missing:" + proofPartialEffectVerified
			return evidence
		}
		if resp.ExecutionResult.Verified && executionResultSatisfies(required, resp) {
			evidence.Verified = true
			evidence.BlockingReason = ""
			evidence.Summary = completionEvidenceSummary(required, true)
			return evidence
		}
	}
	if required == proofAppBridgeVerified && appBridgeEvidenceSatisfies(task, boundary, checkedAt) {
		evidence.Verified = true
		evidence.Source = "android_bridge"
		evidence.BlockingReason = ""
		evidence.Summary = completionEvidenceSummary(required, true)
		return evidence
	}
	return evidence
}

func claimsCompletion(resp contracts.RelayTurnResponse) bool {
	return strings.EqualFold(strings.TrimSpace(resp.CompletionStatus), "completed") ||
		strings.EqualFold(strings.TrimSpace(resp.ContinuationStatus), "completed")
}

func requiredProofClass(task *runtime.TaskSnapshot, checkpoint *runtime.ResumeCheckpoint, resp contracts.RelayTurnResponse) string {
	if task != nil && task.IntakeAssessment != nil {
		switch strings.TrimSpace(task.IntakeAssessment.ChosenClosedAction) {
		case "ANSWER":
			return proofModelTextOnly
		case "REQUEST_PERMISSION":
			return proofUserConfirmationRequired
		case "REFUSE", "SAFE_FALLBACK":
			return proofUnsupportedNoProofSurface
		}
	}

	action := strings.ToLower(strings.TrimSpace(resp.ActionType))
	if action == "check_state" || (checkpoint != nil && strings.EqualFold(strings.TrimSpace(checkpoint.NextStepType), "check_state")) {
		return proofRuntimeStateVerified
	}
	if action == "emit_message" {
		return proofExternalEffectUnverified
	}
	if action != "" && action != "none" {
		return proofSidecarActionVerified
	}

	prompt := ""
	if task != nil {
		prompt = task.Prompt
	}
	lower := strings.ToLower(prompt)
	switch {
	case asksForUserChoice(lower):
		return proofUserConfirmationRequired
	case asksForDeviceOrBridgeEffect(lower):
		return proofAppBridgeVerified
	case asksForExternalEffect(lower):
		return proofUnsupportedNoProofSurface
	default:
		return proofModelTextOnly
	}
}

func executionResultSatisfies(required string, resp contracts.RelayTurnResponse) bool {
	if resp.ExecutionResult == nil || !resp.ExecutionResult.Verified {
		return false
	}
	action := strings.ToLower(strings.TrimSpace(resp.ActionType))
	switch required {
	case proofRuntimeStateVerified:
		return action == "check_state"
	case proofSidecarActionVerified:
		return action != "" && action != "none" && action != "emit_message"
	case proofAppBridgeVerified:
		return false
	case proofProviderResponseVerified:
		return false
	case proofModelTextOnly:
		return true
	default:
		return false
	}
}

func appBridgeEvidenceSatisfies(task *runtime.TaskSnapshot, boundary *runtime.EvidenceBoundary, now time.Time) bool {
	if task == nil || boundary == nil {
		return false
	}
	for i := len(boundary.AppBridgeEvidence) - 1; i >= 0; i-- {
		evidence := boundary.AppBridgeEvidence[i]
		if !evidence.Verified ||
			evidence.ProofClass != proofAppBridgeVerified ||
			evidence.Source != "android_bridge" ||
			!IsAllowedAppBridgeEvidenceOperation(evidence.Operation) {
			continue
		}
		if evidence.TaskID != "" && evidence.TaskID != task.TaskID {
			continue
		}
		checkedAt, err := time.Parse(time.RFC3339, evidence.CheckedAt)
		if err != nil {
			continue
		}
		if checkedAt.After(now.Add(time.Minute)) || now.Sub(checkedAt) > AppBridgeEvidenceFreshness {
			continue
		}
		return true
	}
	return false
}

func IsAllowedAppBridgeEvidenceOperation(operation string) bool {
	switch strings.TrimSpace(operation) {
	case "runtime_host_status",
		"runtime_host_start",
		"runtime_host_restart",
		"native_sidecar_payload_ready",
		"sidecar_ready_probe",
		"task_route_surface_ready",
		"byok_key_storage",
		"permission_state_snapshot":
		return true
	default:
		return false
	}
}

func asksForUserChoice(prompt string) bool {
	return strings.Contains(prompt, "choose") || strings.Contains(prompt, "pick one") || strings.Contains(prompt, "which should")
}

func asksForDeviceOrBridgeEffect(prompt string) bool {
	return containsAny(prompt, []string{
		"phone", "device", "settings", "permission", "notification", "open app", "tap ",
		"screen", "install", "uninstall", "camera", "microphone", "bluetooth", "wifi",
	})
}

func asksForExternalEffect(prompt string) bool {
	return containsAny(prompt, []string{
		"send ", "email ", "text ", "message ", "post ", "publish ", "upload ", "download ",
		"book ", "buy ", "purchase ", "delete ", "move ", "rename ", "change ", "mutate ",
		"edit file", "write file", "save file", "turn on", "turn off",
	})
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func completionEvidenceSummary(proofClass string, verified bool) string {
	if verified {
		return fmt.Sprintf("Completion allowed with verified proof class %s.", proofClass)
	}
	return fmt.Sprintf("Completion blocked because proof class %s is not verified.", proofClass)
}
