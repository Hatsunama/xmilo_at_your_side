package runtimegate

import "time"

const (
	ActionFamilyCompletion = "completion"
)

type CompletionInput struct {
	ClaimsCompletion bool
	EvidenceVerified bool
	ProofClass       string
	BlockingReason   string
}

func EvaluateCompletion(input CompletionInput, now time.Time) Decision {
	if !input.ClaimsCompletion || input.EvidenceVerified {
		return completionDecision(OutcomeAllow, ReasonNone, false, "", now)
	}
	return completionDecision(OutcomeBlock, ReasonCompletionEvidenceMissing, true, "Milo blocked completion because verified runtime evidence was missing.", now)
}

func completionDecision(outcome Outcome, reason ReasonCode, evidenceRequired bool, safeSummary string, now time.Time) Decision {
	decision := NewDecision(outcome, reason, PhasePreCompletion, now)
	decision.ActionFamily = ActionFamilyCompletion
	decision.EvidenceRequired = evidenceRequired
	decision.SafeSummary = safeSummary
	return decision
}
