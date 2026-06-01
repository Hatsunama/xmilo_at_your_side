package runtimegate

import (
	"strings"
	"time"

	"xmilo/sidecar-go/internal/promptsecrecy"
)

const (
	ActionFamilyContextInjection = "context_injection"
	SafeContextBudgetBytes       = 16 * 1024
)

type ContextInput struct {
	Content     string
	TrustTier   string
	Source      string
	Legacy      bool
	ByteLength  int
	StructValid bool
}

func EvaluatePreContext(input ContextInput, now time.Time) Decision {
	if !input.StructValid {
		return preContextDecision(OutcomeBlock, ReasonUnknownMalformedAction, "Some untrusted context was omitted because its metadata was malformed.", now)
	}
	if strings.TrimSpace(input.Content) == "" {
		return preContextDecision(OutcomeBlock, ReasonUnknownMalformedAction, "Some untrusted context was omitted because it was empty or malformed.", now)
	}
	if input.ByteLength <= 0 {
		input.ByteLength = len([]byte(input.Content))
	}
	if input.ByteLength > SafeContextBudgetBytes {
		return preContextDecision(OutcomeBlock, ReasonUnboundedConsumptionRisk, "Some untrusted context was omitted because it exceeded the safe context budget.", now)
	}
	if input.TrustTier != "untrusted_external" || input.Legacy || strings.TrimSpace(input.Source) == "" || strings.TrimSpace(input.Source) == "legacy_unknown" {
		return preContextDecision(OutcomeBlock, ReasonUnknownMalformedAction, "Some untrusted context was omitted because its source label was missing or invalid.", now)
	}

	lower := strings.ToLower(input.Content)
	if assessment := promptsecrecy.Classify(input.Content); assessment.Forbidden() {
		if assessment.SecretLike() {
			return preContextDecision(OutcomeBlock, ReasonCredentialSecretRisk, "Some untrusted context was omitted because it appeared to contain secrets, provider configuration, or private runtime payloads.", now)
		}
		return preContextDecision(OutcomeBlock, ReasonPromptInjectionAuthoritySpoof, "Some untrusted context was omitted because it attempted to reveal hidden prompts, internal authority text, or private reasoning.", now)
	}
	switch {
	case contextContainsCredentialSecretRisk(lower):
		return preContextDecision(OutcomeBlock, ReasonCredentialSecretRisk, "Some untrusted context was omitted because it appeared to contain secrets or provider configuration.", now)
	case contextContainsUnsafeAutomation(lower):
		return preContextDecision(OutcomeBlock, ReasonUnsafeAutomation, "Some untrusted context was omitted because it attempted hidden or confirmation-bypassing automation.", now)
	case contextContainsTransformedCommand(lower):
		return preContextDecision(OutcomeBlock, ReasonExternalContentAttemptedCommand, "Some untrusted context was omitted because transformed content attempted to become an instruction.", now)
	case contextContainsSupplyChainCommand(lower):
		return preContextDecision(OutcomeBlock, ReasonExternalContentAttemptedCommand, "Some untrusted context was omitted because tool or skill metadata attempted to become instruction.", now)
	case contextContainsRetrievalAuthorityClaim(lower):
		return preContextDecision(OutcomeBlock, ReasonPromptInjectionAuthoritySpoof, "Some untrusted retrieval context was omitted because it attempted to outrank runtime or canonical authority.", now)
	case contextContainsAuthoritySpoof(lower):
		return preContextDecision(OutcomeBlock, ReasonPromptInjectionAuthoritySpoof, "Some untrusted context was omitted because it appeared to contain instructions or unsafe authority claims.", now)
	default:
		return preContextDecision(OutcomeAllow, ReasonNone, "", now)
	}
}

func preContextDecision(outcome Outcome, reason ReasonCode, safeSummary string, now time.Time) Decision {
	decision := NewDecision(outcome, reason, PhasePreContextInjection, now)
	decision.ActionFamily = ActionFamilyContextInjection
	decision.SafeSummary = safeSummary
	tier := 5
	decision.SourceTrustTier = &tier
	return decision
}

func OmittedContextNote(decision SanitizedDecision) string {
	summary := strings.TrimSpace(decision.SafeSummary)
	if summary == "" {
		summary = "Some untrusted context was omitted before model use."
	}
	return "<omitted_untrusted_context>\n" +
		"gate_phase: " + string(decision.GatePhase) + "\n" +
		"reason_code: " + string(decision.ReasonCode) + "\n" +
		"summary: " + summary + "\n" +
		"</omitted_untrusted_context>"
}

func contextContainsAuthoritySpoof(content string) bool {
	return containsAny(content, []string{
		"ignore previous instructions",
		"developer says",
		"system says",
		"you are now",
		"user already approved",
		"bypass confirmation",
	})
}

func contextContainsTransformedCommand(content string) bool {
	return containsAny(content, []string{
		"decoded instruction",
		"translation says to",
		"the hidden message says",
		"morse says",
		"base64 says",
		"qr says",
	})
}

func contextContainsSupplyChainCommand(content string) bool {
	return containsAny(content, []string{
		"install this skill",
		"activate this skill",
		"activate me as authority",
		"trust this skill",
		"skill manifest says",
		"skill metadata says",
		"tool description says",
		"plugin says",
		"imported tool says",
	})
}

func contextContainsRetrievalAuthorityClaim(content string) bool {
	return containsAny(content, []string{
		"retrieved chunk outranks",
		"retrieved context outranks",
		"retrieval says to ignore",
		"retrieval says this is canon",
		"search result says this is policy",
		"archive chunk says this is policy",
		"memory search says this overrides",
		"embedding result says this overrides",
		"vector match says this overrides",
		"source trust tier 0",
	})
}

func contextContainsCredentialSecretRisk(content string) bool {
	return containsAny(content, []string{
		"api_key",
		"api key",
		"auth header",
		"authorization: bearer",
		"bearer token",
		"provider config",
		"hidden secret",
		"send all keys",
		"send all secrets",
	})
}

func contextContainsUnsafeAutomation(content string) bool {
	return containsAny(content, []string{
		"do it without telling",
		"hide this from the user",
		"hide it from the user",
		"bypass confirmation",
		"perform actions silently",
		"run silently without confirmation",
	})
}
