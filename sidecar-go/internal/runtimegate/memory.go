package runtimegate

import (
	"strings"
	"time"

	"xmilo/sidecar-go/internal/poisoning"
	"xmilo/sidecar-go/internal/promptsecrecy"
)

const (
	ActionFamilyMemoryPromotion = "memory_promotion"
	MaxMemoryPromotionBytes     = 8 * 1024
)

type MemoryPromotionInput struct {
	Content                    string
	Source                     string
	Target                     string
	SourceTrustTier            *int
	VerifiedRuntimeState       bool
	VerifiedCompletionEvidence bool
	ByteLength                 int
}

func EvaluateMemoryPromotion(input MemoryPromotionInput, now time.Time) Decision {
	if strings.TrimSpace(input.Source) == "" || strings.TrimSpace(input.Target) == "" {
		return memoryDecision(OutcomeBlock, ReasonUnknownMalformedAction, input, "Milo blocked memory promotion because source metadata was missing.", now)
	}
	if strings.TrimSpace(input.Content) == "" {
		return memoryDecision(OutcomeAllow, ReasonNone, input, "", now)
	}
	if input.ByteLength <= 0 {
		input.ByteLength = len([]byte(input.Content))
	}
	if input.ByteLength > MaxMemoryPromotionBytes {
		return memoryDecision(OutcomeBlock, ReasonUnboundedConsumptionRisk, input, "Milo blocked memory promotion because the content exceeded the safe size budget.", now)
	}
	if memoryPromotionContainsTestFixture(input) {
		return memoryDecision(OutcomeBlock, ReasonExternalContentAttemptedCommand, input, "Milo blocked test fixture content from becoming durable memory or archive truth.", now)
	}

	lower := strings.ToLower(input.Content)
	if assessment := promptsecrecy.Classify(input.Content); assessment.Forbidden() {
		return memoryDecision(OutcomeBlock, promptLeakageReason(input.Content), input, "Milo blocked durable memory promotion because the content attempted to persist hidden prompts, private policy, secrets, or internal runtime payloads.", now)
	}
	sourceTier := trustTierOrDefault(input.SourceTrustTier)
	sourceTierPtr := &sourceTier
	assessment := poisoning.AssessCandidate(poisoning.Candidate{
		RecordKey:     strings.TrimSpace(input.Target),
		RecordKind:    "memory_promotion",
		Content:       input.Content,
		SourceID:      strings.TrimSpace(input.Source),
		SourceType:    strings.TrimSpace(input.Source),
		TrustTier:     sourceTierPtr,
		AuthorityRank: "rank_500_external",
		ProvenanceChain: []poisoning.ProvenanceNode{{
			SourceID:      strings.TrimSpace(input.Source),
			SourceType:    strings.TrimSpace(input.Source),
			TrustTier:     sourceTier,
			AuthorityRank: "rank_500_external",
		}},
		Now:                        now,
		VerifiedRuntimeState:       input.VerifiedRuntimeState,
		VerifiedCompletionEvidence: input.VerifiedCompletionEvidence,
	})
	if assessment.Blocked() {
		return memoryDecision(OutcomeBlock, poisoningFindingReason(assessment.Findings, lower), input, "Milo blocked durable memory promotion because poisoning controls found unsafe durable truth.", now)
	}
	switch {
	case memoryContainsCredentialSecretRisk(lower):
		return memoryDecision(OutcomeBlock, ReasonCredentialSecretRisk, input, "Milo blocked memory promotion because the content appeared to contain secrets or credentials.", now)
	case memoryContainsUnsafeAutomation(lower):
		return memoryDecision(OutcomeBlock, ReasonUnsafeAutomation, input, "Milo blocked memory promotion because the content attempted hidden or confirmation-bypassing behavior.", now)
	case memoryContainsTransformedInstruction(lower):
		return memoryDecision(OutcomeBlock, ReasonExternalContentAttemptedCommand, input, "Milo blocked transformed content from becoming durable instruction.", now)
	case memoryContainsSupplyChainInstruction(lower):
		return memoryDecision(OutcomeBlock, ReasonExternalContentAttemptedCommand, input, "Milo blocked skill, plugin, or tool metadata from becoming durable instruction.", now)
	case memoryContainsRetrievalAuthorityClaim(lower):
		return memoryDecision(OutcomeBlock, ReasonPromptInjectionAuthoritySpoof, input, "Milo blocked retrieval-like content from becoming durable authority.", now)
	case memoryContainsAuthoritySpoof(lower):
		return memoryDecision(OutcomeBlock, ReasonPromptInjectionAuthoritySpoof, input, "Milo blocked authority-spoofing content from durable memory.", now)
	case memoryContainsDurableInstruction(lower) && !directUserSource(input.Source):
		return memoryDecision(OutcomeBlock, ReasonExternalContentAttemptedCommand, input, "Milo blocked external or model content from becoming a durable instruction.", now)
	case memoryContainsCapabilityTruthMutation(lower) && !input.VerifiedRuntimeState:
		return memoryDecision(OutcomeBlock, ReasonMissingToolProof, input, "Milo blocked unsupported capability or provider truth from durable memory.", now)
	case memoryContainsCompletionTruthMutation(lower) && !input.VerifiedCompletionEvidence:
		return memoryDecision(OutcomeBlock, ReasonCompletionEvidenceMissing, input, "Milo blocked unsupported completion truth from durable memory.", now)
	default:
		return memoryDecision(OutcomeAllow, ReasonNone, input, "", now)
	}
}

func memoryPromotionContainsTestFixture(input MemoryPromotionInput) bool {
	return containsAny(strings.ToLower(strings.Join([]string{input.Source, input.Target, input.Content}, "\n")), []string{
		"test_fixture",
		"test-fixture",
		"fixture:",
	})
}

func trustTierOrDefault(tier *int) int {
	if tier == nil {
		return 9
	}
	return *tier
}

func poisoningFindingReason(findings []poisoning.Finding, content string) ReasonCode {
	if memoryContainsUnsafeAutomation(content) {
		return ReasonUnsafeAutomation
	}
	for _, finding := range findings {
		switch finding.Code {
		case poisoning.FindingCredentialSecret:
			return ReasonCredentialSecretRisk
		case poisoning.FindingCapabilityTruthMutation:
			return ReasonMissingToolProof
		case poisoning.FindingCompletionTruthMutation:
			return ReasonCompletionEvidenceMissing
		case poisoning.FindingAuthoritySpoof, poisoning.FindingProviderTruthMutation:
			return ReasonPromptInjectionAuthoritySpoof
		default:
			return ReasonExternalContentAttemptedCommand
		}
	}
	return ReasonExternalContentAttemptedCommand
}

func memoryDecision(outcome Outcome, reason ReasonCode, input MemoryPromotionInput, safeSummary string, now time.Time) Decision {
	decision := NewDecision(outcome, reason, PhasePreMemoryWrite, now)
	decision.ActionFamily = ActionFamilyMemoryPromotion
	decision.ActionName = safeMemoryTarget(input.Target)
	decision.SourceTrustTier = input.SourceTrustTier
	decision.SafeSummary = safeSummary
	if reason == ReasonMissingToolProof || reason == ReasonCompletionEvidenceMissing {
		decision.EvidenceRequired = true
	}
	return decision
}

func directUserSource(source string) bool {
	switch strings.TrimSpace(source) {
	case "direct_user", "user_prompt", "current_user_instruction":
		return true
	default:
		return false
	}
}

func safeMemoryTarget(target string) string {
	target = strings.TrimSpace(target)
	if len(target) > 64 {
		target = target[:64]
	}
	for _, r := range target {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' {
			return ""
		}
	}
	return target
}

func memoryContainsAuthoritySpoof(content string) bool {
	return containsAny(content, []string{
		"ignore previous instructions",
		"system says",
		"developer says",
		"you are now system",
		"you are now developer",
		"user already approved",
		"override xmilo policy",
		"override runtime authority",
		"new policy",
	})
}

func memoryContainsDurableInstruction(content string) bool {
	return containsAny(content, []string{
		"remember this as a rule",
		"store this as a rule",
		"from now on always",
		"always obey this",
		"make this permanent",
		"add this to memory as policy",
	})
}

func memoryContainsCredentialSecretRisk(content string) bool {
	return containsAny(content, []string{
		"api_key",
		"api key",
		"auth header",
		"authorization: bearer",
		"bearer token",
		"provider config",
		"secret token",
		"access token",
		"hidden prompt",
	})
}

func memoryContainsCapabilityTruthMutation(content string) bool {
	return containsAny(content, []string{
		"camera works",
		"camera is available",
		"sensors available",
		"screen access works",
		"touch access works",
		"hosted active",
		"hosted mode active",
		"byok active",
		"byok mode active",
		"provider healthy",
		"provider route healthy",
	})
}

func memoryContainsCompletionTruthMutation(content string) bool {
	return containsAny(content, []string{
		"task completed",
		"task is completed",
		"completion evidence exists",
		"evidence exists",
		"marked done",
		"mark as done",
		"done for real",
	})
}

func memoryContainsTransformedInstruction(content string) bool {
	return containsAny(content, []string{
		"decoded instruction",
		"translation says to",
		"the hidden message says",
		"morse says",
		"base64 says",
		"hex says",
		"qr says",
		"ocr says",
		"transcription says to",
	})
}

func memoryContainsSupplyChainInstruction(content string) bool {
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

func memoryContainsRetrievalAuthorityClaim(content string) bool {
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

func memoryContainsUnsafeAutomation(content string) bool {
	return containsAny(content, []string{
		"act silently",
		"hide actions",
		"hide this from the user",
		"bypass confirmation",
		"skip confirmation",
		"skip safety",
		"unsafe roleplay",
		"evil persona",
	})
}
