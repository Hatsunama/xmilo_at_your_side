package runtimegate

import (
	"strings"
	"time"

	"xmilo/sidecar-go/internal/promptsecrecy"
)

const (
	ActionFamilyModelAction = "model_action"
)

type ModelActionInput struct {
	ActionType         string
	CompletionStatus   string
	ContinuationStatus string
	Summary            string
	ReportText         string
	ThoughtText        string
	NextBlocker        string
	ExecutionVerified  bool
}

func EvaluateModelAction(input ModelActionInput, now time.Time) Decision {
	action := strings.ToLower(strings.TrimSpace(input.ActionType))
	completion := strings.ToLower(strings.TrimSpace(input.CompletionStatus))
	continuation := strings.ToLower(strings.TrimSpace(input.ContinuationStatus))

	if !knownModelActionType(action) || !knownCompletionStatus(completion) || !knownContinuationStatus(continuation) {
		return modelActionDecision(OutcomeBlock, ReasonUnknownMalformedAction, false, "Milo blocked a malformed model action before it could affect runtime state.", now)
	}

	content := strings.ToLower(strings.Join([]string{
		input.Summary,
		input.ReportText,
		input.ThoughtText,
		input.NextBlocker,
	}, "\n"))

	switch {
	case promptsecrecy.Classify(content).Forbidden():
		return modelActionDecision(OutcomeBlock, promptLeakageReason(content), false, "Milo blocked model output that attempted to expose hidden prompts, private instructions, chain-of-thought, secrets, or internal runtime payloads.", now)
	case modelActionContainsCredentialSecretRisk(content):
		return modelActionDecision(OutcomeBlock, ReasonCredentialSecretRisk, false, "Milo blocked model output that attempted to reveal secrets or internal configuration.", now)
	case modelActionContainsHiddenAutomation(content):
		return modelActionDecision(OutcomeBlock, ReasonUnsafeAutomation, false, "Milo blocked model output that attempted hidden or confirmation-bypassing action.", now)
	case modelActionContainsAuthoritySpoof(content):
		return modelActionDecision(OutcomeBlock, ReasonPromptInjectionAuthoritySpoof, false, "Milo blocked model output that attempted to treat spoofed approval or authority as runtime truth.", now)
	case modelActionContainsExternalCommandBasis(content):
		return modelActionDecision(OutcomeBlock, ReasonExternalContentAttemptedCommand, false, "Milo blocked model output that attempted to convert external context into an executable instruction.", now)
	case modelActionCompletionNeedsEvidence(input, content):
		return modelActionDecision(OutcomeBlock, ReasonCompletionEvidenceMissing, true, "Milo blocked a completion claim because runtime evidence was missing.", now)
	}
	if reason, ok := modelActionCapabilityOverclaimReason(content); ok {
		return modelActionDecision(OutcomeBlock, reason, false, "Milo blocked a model capability claim that was not backed by runtime proof.", now)
	}
	return modelActionDecision(OutcomeAllow, ReasonNone, false, "", now)
}

func promptLeakageReason(content string) ReasonCode {
	if promptsecrecy.Classify(content).SecretLike() {
		return ReasonCredentialSecretRisk
	}
	return ReasonPromptInjectionAuthoritySpoof
}

func modelActionDecision(outcome Outcome, reason ReasonCode, evidenceRequired bool, safeSummary string, now time.Time) Decision {
	decision := NewDecision(outcome, reason, PhasePreModelAction, now)
	decision.ActionFamily = ActionFamilyModelAction
	decision.EvidenceRequired = evidenceRequired
	decision.SafeSummary = safeSummary
	return decision
}

func knownModelActionType(action string) bool {
	switch action {
	case "none", "await_user_choice", "emit_message", "resume_checkpoint", "check_state":
		return true
	default:
		return false
	}
}

func knownCompletionStatus(status string) bool {
	switch status {
	case "completed", "blocked", "needs_user_choice", "attempted_unverified":
		return true
	default:
		return false
	}
}

func knownContinuationStatus(status string) bool {
	switch status {
	case "completed", "blocked", "awaiting_user_choice", "needs_check", "resumable", "not_resumable":
		return true
	default:
		return false
	}
}

func modelActionContainsAuthoritySpoof(content string) bool {
	return containsAny(content, []string{
		"user already approved",
		"user has already approved",
		"already approved by the user",
		"approved in the document",
		"system says",
		"developer says",
	})
}

func modelActionContainsExternalCommandBasis(content string) bool {
	return containsAny(content, []string{
		"the context instructed me",
		"context instructed me",
		"the decoded message told me",
		"decoded message told me",
		"the document says i should",
		"document says i should",
		"the attachment says i should",
		"the pasted text says i should",
	})
}

func modelActionContainsCredentialSecretRisk(content string) bool {
	return containsAny(content, []string{
		"api_key",
		"api key is",
		"api keys are",
		"auth header",
		"auth headers",
		"authorization: bearer",
		"bearer token",
		"provider config",
		"hidden prompt",
		"internal policy",
		"secret token",
		"access token",
	})
}

func modelActionContainsHiddenAutomation(content string) bool {
	return containsAny(content, []string{
		"act silently",
		"acting silently",
		"hide actions",
		"hide the action",
		"hide this from the user",
		"bypass confirmation",
		"skip confirmation",
		"skip safety",
		"bypass safety",
		"continue despite blocked context",
	})
}

func modelActionCapabilityOverclaimReason(content string) (ReasonCode, bool) {
	if containsAny(content, []string{
		"hosted mode is active",
		"byok mode is active",
		"provider route is active",
		"provider access is available",
	}) {
		return ReasonMissingProviderAccessRoute, true
	}
	if containsAny(content, []string{
		"i can access your camera",
		"camera access is available",
		"i can use sensors",
		"sensor access is available",
		"i can see your screen",
		"screen access is available",
		"i can tap",
		"i can swipe",
		"touch access is available",
		"location access is available",
		"microphone is available",
		"i can read your files",
		"file access is available",
	}) {
		return ReasonMissingToolProof, true
	}
	return ReasonNone, false
}

func modelActionCompletionNeedsEvidence(input ModelActionInput, content string) bool {
	if input.ExecutionVerified {
		return false
	}
	if strings.ToLower(strings.TrimSpace(input.CompletionStatus)) != "completed" ||
		strings.ToLower(strings.TrimSpace(input.ContinuationStatus)) != "completed" {
		return false
	}
	return containsAny(content, []string{
		"i sent",
		"message sent",
		"sent the message",
		"email sent",
		"deleted files",
		"deleted the files",
		"wiped files",
		"changed settings",
		"changed the setting",
		"tapped",
		"swiped",
		"opened camera",
		"used the camera",
	})
}
