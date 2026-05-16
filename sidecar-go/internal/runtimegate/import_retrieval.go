package runtimegate

import (
	"fmt"
	"strings"
	"time"

	"xmilo/sidecar-go/internal/promptsecrecy"
)

const (
	ActionFamilyImportedToolActivation = "imported_tool_activation"
	ActionFamilyRetrievalContext       = "retrieval_context"
)

type ImportActivationInput struct {
	ImportID            string
	ToolID              string
	ActiveScoped        bool
	TrustTier           *int
	Provenance          map[string]any
	DescriptorText      string
	RequestedCapability string
	CapabilityState     map[string]any
}

type RetrievalContextInput struct {
	ChunkID                     string
	Content                     string
	SourceID                    string
	SourceType                  string
	TrustTier                   *int
	Provenance                  map[string]any
	QuarantineStatus            string
	ContainsSecret              bool
	ContainsExternalInstruction bool
}

func EvaluateImportActivation(input ImportActivationInput, now time.Time) Decision {
	actionName := safeActionName(strings.TrimSpace(input.ToolID))
	if actionName == "" {
		actionName = safeActionName(strings.TrimSpace(input.ImportID))
	}
	if !input.ActiveScoped {
		return importedToolDecision(actionName, OutcomeBlock, ReasonMissingApproval, false, "Milo blocked imported capability activation because it is not explicitly active and scoped.", now)
	}
	if input.TrustTier == nil || len(input.Provenance) == 0 {
		return importedToolDecision(actionName, OutcomeBlock, ReasonUnknownMalformedAction, false, "Milo blocked imported capability activation because trust or provenance metadata was missing.", now)
	}
	lower := strings.ToLower(input.DescriptorText)
	if toolActionContainsCredentialSecretRisk(lower) {
		return importedToolDecision(actionName, OutcomeBlock, ReasonCredentialSecretRisk, false, "Milo blocked imported capability activation because descriptor text requested secrets.", now)
	}
	if assessment := promptsecrecy.Classify(input.DescriptorText); assessment.Forbidden() {
		return importedToolDecision(actionName, OutcomeBlock, promptLeakageReason(input.DescriptorText), false, "Milo blocked imported capability activation because descriptor text attempted to expose hidden prompt, private policy, secret, or runtime payload material.", now)
	}
	if toolActionContainsAuthoritySpoof(lower) || contextContainsSupplyChainCommand(lower) {
		return importedToolDecision(actionName, OutcomeBlock, ReasonPromptInjectionAuthoritySpoof, false, "Milo blocked imported capability activation because descriptor text attempted runtime authority.", now)
	}
	if toolActionContainsHiddenAutomation(lower) {
		return importedToolDecision(actionName, OutcomeBlock, ReasonUnsafeAutomation, false, "Milo blocked imported capability activation because descriptor text attempted hidden execution.", now)
	}
	if capability := strings.TrimSpace(input.RequestedCapability); capability != "" && !CapabilityUsable(input.CapabilityState, capability) {
		return importedToolDecision(actionName, OutcomeBlock, ReasonMissingToolProof, true, "Milo blocked imported capability activation because usable capability proof was missing.", now)
	}
	return importedToolDecision(actionName, OutcomeAllow, ReasonNone, false, "", now)
}

func EvaluateRetrievalContext(input RetrievalContextInput, now time.Time) Decision {
	if input.TrustTier == nil || len(input.Provenance) == 0 || strings.TrimSpace(input.SourceID) == "" || strings.TrimSpace(input.SourceType) == "" {
		return retrievalContextDecision(input.ChunkID, OutcomeBlock, ReasonUnknownMalformedAction, false, "Milo omitted retrieved content because trust or provenance metadata was missing.", now)
	}
	if strings.TrimSpace(input.QuarantineStatus) != "clean" {
		return retrievalContextDecision(input.ChunkID, OutcomeBlock, ReasonExternalContentAttemptedCommand, false, "Milo omitted retrieved content because it was not clean for retrieval.", now)
	}
	if input.ContainsSecret {
		return retrievalContextDecision(input.ChunkID, OutcomeBlock, ReasonCredentialSecretRisk, false, "Milo omitted retrieved content because it was marked as secret-bearing.", now)
	}
	if input.ContainsExternalInstruction {
		return retrievalContextDecision(input.ChunkID, OutcomeBlock, ReasonExternalContentAttemptedCommand, false, "Milo omitted retrieved content because it contained external instructions.", now)
	}
	if assessment := promptsecrecy.Classify(input.Content); assessment.Forbidden() {
		return retrievalContextDecision(input.ChunkID, OutcomeBlock, promptLeakageReason(input.Content), false, "Milo omitted retrieved content because it attempted to expose hidden prompt, private policy, secret, or runtime payload material.", now)
	}
	preContext := EvaluatePreContext(ContextInput{
		Content:     input.Content,
		TrustTier:   "untrusted_external",
		Source:      "retrieval:" + strings.TrimSpace(input.ChunkID),
		StructValid: true,
	}, now)
	preContext.ActionFamily = ActionFamilyRetrievalContext
	preContext.ActionName = safeActionName(input.ChunkID)
	preContext.SourceTrustTier = input.TrustTier
	return preContext
}

func importedToolDecision(actionName string, outcome Outcome, reason ReasonCode, evidenceRequired bool, safeSummary string, now time.Time) Decision {
	decision := NewDecision(outcome, reason, PhasePreToolAction, now)
	decision.ActionFamily = ActionFamilyImportedToolActivation
	decision.ActionName = actionName
	decision.EvidenceRequired = evidenceRequired
	decision.SafeSummary = safeSummary
	return decision
}

func retrievalContextDecision(chunkID string, outcome Outcome, reason ReasonCode, evidenceRequired bool, safeSummary string, now time.Time) Decision {
	decision := NewDecision(outcome, reason, PhasePreContextInjection, now)
	decision.ActionFamily = ActionFamilyRetrievalContext
	decision.ActionName = safeActionName(chunkID)
	decision.EvidenceRequired = evidenceRequired
	decision.SafeSummary = safeSummary
	return decision
}

func ProvenanceFromStrings(values map[string]string) map[string]any {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func TrustTierPtr(tier int) *int {
	return &tier
}

func RetrievalGateSafeSummary(decision Decision) string {
	if strings.TrimSpace(decision.SafeSummary) != "" {
		return decision.SafeSummary
	}
	return fmt.Sprintf("retrieval gate %s: %s", decision.Outcome, decision.ReasonCode)
}
