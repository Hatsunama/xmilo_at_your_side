package runtimegate

import (
	"strings"
	"time"

	"xmilo/sidecar-go/internal/promptsecrecy"
)

const (
	ActionFamilyTaskStart = "task_start"
)

func EvaluatePreTask(prompt string, now time.Time) Decision {
	trimmed := strings.TrimSpace(prompt)
	if trimmed == "" {
		return preTaskDecision(OutcomeClarify, ReasonUnknownMalformedAction, "Task request was empty or unclear.", now)
	}

	lower := strings.ToLower(trimmed)
	if assessment := promptsecrecy.Classify(trimmed); assessment.Forbidden() {
		if assessment.SecretLike() {
			return preTaskDecision(OutcomeBlock, ReasonCredentialSecretRisk, "Milo blocked a request to reveal secrets, provider configuration, or private runtime payloads.", now)
		}
		return preTaskDecision(OutcomeBlock, ReasonPromptInjectionAuthoritySpoof, "Milo blocked a request to reveal hidden prompts, private instructions, internal authority text, or chain-of-thought.", now)
	}
	switch {
	case looksLikeSelfHarmCrisisOrDangerousAdvice(lower):
		return preTaskDecision(OutcomeSafeRedirect, ReasonHarmfulRequest, "Milo redirected a self-harm or dangerous-advice request before task start.", now)
	case looksLikeAuthoritySpoof(lower):
		return preTaskDecision(OutcomeBlock, ReasonPromptInjectionAuthoritySpoof, "Blocked authority-spoofing instructions before task start.", now)
	case looksLikeCredentialOrSecretRequest(lower):
		return preTaskDecision(OutcomeBlock, ReasonCredentialSecretRisk, "Blocked a request to reveal or extract secrets.", now)
	case looksLikeDestructiveLocalAction(lower):
		return preTaskDecision(OutcomeBlock, ReasonDestructiveAction, "Blocked a destructive local action request before task start.", now)
	case looksLikePrivacyOrSurveillanceAbuse(lower):
		return preTaskDecision(OutcomeBlock, ReasonPrivacySurveillanceRisk, "Blocked a privacy-invasive or surveillance request before task start.", now)
	case looksLikeUnsafeHiddenAutomation(lower):
		return preTaskDecision(OutcomeBlock, ReasonUnsafeAutomation, "Blocked a hidden or confirmation-bypassing automation request.", now)
	case looksLikeHarmfulFacilitation(lower):
		return preTaskDecision(OutcomeBlock, ReasonHarmfulRequest, "Blocked an unsafe request before task start.", now)
	default:
		return preTaskDecision(OutcomeAllow, ReasonNone, "", now)
	}
}

func preTaskDecision(outcome Outcome, reason ReasonCode, safeSummary string, now time.Time) Decision {
	decision := NewDecision(outcome, reason, PhasePreTask, now)
	decision.ActionFamily = ActionFamilyTaskStart
	decision.SafeSummary = safeSummary
	tier := 2
	decision.SourceTrustTier = &tier
	return decision
}

func looksLikeAuthoritySpoof(prompt string) bool {
	return containsAny(prompt, []string{
		"ignore previous instructions",
		"ignore all previous instructions",
		"you are now system",
		"you are now a system",
		"you are now developer",
		"you are now a developer",
		"developer says",
		"developer message",
		"system override",
		"reveal hidden prompt",
		"show hidden prompt",
		"reveal your hidden prompt",
		"show your hidden prompt",
		"bypass rules",
		"bypass the rules",
		"ignore xmilo policy",
		"override runtime authority",
	})
}

func looksLikeSelfHarmCrisisOrDangerousAdvice(prompt string) bool {
	return containsAny(prompt, []string{
		"how do i kill myself",
		"best way to kill myself",
		"best way to commit suicide",
		"help me commit suicide",
		"help me harm myself",
		"how to self harm",
		"how can i self harm",
	})
}

func looksLikeCredentialOrSecretRequest(prompt string) bool {
	return containsAny(prompt, []string{
		"show me api keys",
		"show api keys",
		"print api keys",
		"dump api keys",
		"show me tokens",
		"print tokens",
		"dump tokens",
		"dump auth headers",
		"show auth headers",
		"reveal provider config",
		"show provider config",
		"show hidden secrets",
		"reveal hidden secrets",
		"print secrets",
		"dump secrets",
		"phish credentials",
		"steal credentials",
		"steal passwords",
		"dump passwords",
	})
}

func looksLikeDestructiveLocalAction(prompt string) bool {
	return containsAny(prompt, []string{
		"delete everything",
		"wipe files",
		"wipe all files",
		"destroy database",
		"destroy the database",
		"remove repo",
		"remove the repo",
		"delete the repo",
		"delete repository",
	})
}

func looksLikeUnsafeHiddenAutomation(prompt string) bool {
	return containsAny(prompt, []string{
		"do it without telling me",
		"hide this from the user",
		"hide it from the user",
		"bypass confirmation",
		"bypass confirmations",
		"perform actions silently",
		"run silently without confirmation",
		"do this secretly",
	})
}

func looksLikePrivacyOrSurveillanceAbuse(prompt string) bool {
	return containsAny(prompt, []string{
		"stalk my ex",
		"track someone without consent",
		"track them without consent",
		"spy on someone",
		"spy on my partner",
		"read private messages without permission",
		"access private messages without permission",
		"doxx",
		"dox someone",
		"find their home address",
	})
}

func looksLikeHarmfulFacilitation(prompt string) bool {
	return containsAny(prompt, []string{
		"build a bomb",
		"make a bomb",
		"make explosives",
		"poison someone",
		"how to kill someone",
		"3d print a ghost gun",
		"make a ghost gun",
		"write ransomware",
		"build ransomware",
		"deploy ransomware",
		"ddos",
		"bypass login",
		"hack into",
		"shoplift without getting caught",
		"steal a car",
		"launder money",
		"forge a passport",
		"fake passport",
		"csam",
		"sexual content involving minors",
		"coerce someone sexually",
		"write a threat",
		"targeted harassment campaign",
		"pretend to be a bank",
		"impersonate a bank",
		"social engineer",
		"phishing email",
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
