package retrieval

import (
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"time"

	"xmilo/sidecar-go/internal/db"
	"xmilo/sidecar-go/internal/promptsecrecy"
	"xmilo/sidecar-go/internal/runtimegate"
)

const (
	DefaultTypedMemoryPackBudgetTokens = 2048

	WarningFreshnessNeedsVerification = "freshness_needs_verification"
	WarningSuspectedContradiction     = "suspected_contradiction"
	WarningMissingEvidence            = "missing_evidence"
	WarningDisplayBlockedEvidence     = "display_blocked_evidence"
	WarningPromotionBlockedEvidence   = "promotion_blocked_evidence"
	WarningLowConfidence              = "low_confidence"
	WarningSameLevelConflict          = "same_level_conflict"
	WarningExternalContentDataOnly    = "external_content_data_only"
	WarningBudgetExclusion            = "budget_exclusion"
	WarningAuthorityConflict          = "authority_conflict"

	ExclusionUnsafeStatus             = "unsafe_status"
	ExclusionRetrievalIneligible      = "retrieval_ineligible"
	ExclusionStaleOrExpired           = "stale_or_expired"
	ExclusionConfirmedContradiction   = "confirmed_contradiction"
	ExclusionQuarantined              = "quarantined"
	ExclusionSuppressed               = "suppressed"
	ExclusionCandidateOnly            = "candidate_only"
	ExclusionUnsafeSecretContent      = "unsafe_secret_content"
	ExclusionExternalContentUnsafe    = "external_content_unsafe"
	ExclusionBudgetExceeded           = "budget_exceeded"
	ExclusionUserVisibleBlocked       = "user_visible_blocked"
	ExclusionAuthoritySpoof           = "authority_spoof"
	ExclusionEvidenceDisplayBlocked   = "evidence_display_blocked"
	ExclusionEvidencePromotionBlocked = "evidence_promotion_blocked"
)

var typedMemoryAuthorityHeader = []string{
	"canon_source_of_truth",
	"main_hub_decision",
	"verified_runtime_system_state",
	"current_direct_user_instruction",
	"approved_structured_memory",
	"approved_summary",
	"episodic_history",
	"archive_history",
	"external_imported_content",
	"unknown_malformed_spoofed_content",
}

type TypedMemoryPackInput struct {
	QueryIntent       string
	RuntimeTruthItems []string
	CanonRefs         []string
	BudgetTokens      int
	MaxMemoryItems    int
	Now               time.Time
}

type TypedMemoryRetrievalPack struct {
	PackID                     string
	QueryIntent                string
	BudgetTokens               int
	UsedTokens                 int
	AuthorityHeader            []string
	RuntimeTruthItems          []string
	CanonRefs                  []string
	MemoryItems                []TypedMemoryPackItem
	ExcludedItems              []TypedMemoryPackExclusion
	WarningItems               []TypedMemoryPackWarning
	TruthStatus                string
	SourceLabels               map[string]string
	StaleConflictWarnings      []TypedMemoryPackWarning
	FinalContextInjectionOrder []string
}

type TypedMemoryPackItem struct {
	MemoryID           string
	MemoryClass        string
	Status             string
	SourceType         string
	SourceID           string
	SourceLabel        string
	TrustTier          int
	AuthorityRank      string
	Title              string
	Text               string
	EvidenceRefs       []TypedMemoryPackEvidenceRef
	FreshnessState     string
	Confidence         float64
	ContradictionState string
	WarningCodes       []string
	PackPosition       int
	TokenEstimate      int
	DataOnly           bool
}

type TypedMemoryPackEvidenceRef struct {
	EvidenceID       string
	SourceType       string
	SourceID         string
	SourceRef        string
	EvidenceKind     string
	TrustTier        int
	AuthorityRank    string
	RedactionStatus  string
	DisplayAllowed   bool
	PromotionAllowed bool
}

type TypedMemoryPackWarning struct {
	MemoryID string
	Code     string
	Summary  string
}

type TypedMemoryPackExclusion struct {
	MemoryID string
	Code     string
	Summary  string
}

func BuildTypedMemoryRetrievalPack(store *db.Store, input TypedMemoryPackInput) (TypedMemoryRetrievalPack, error) {
	if store == nil {
		return TypedMemoryRetrievalPack{}, errors.New("typed_memory_pack_missing_store")
	}
	input.QueryIntent = strings.TrimSpace(input.QueryIntent)
	if input.QueryIntent == "" {
		return TypedMemoryRetrievalPack{}, errors.New("typed_memory_pack_missing_query_intent")
	}
	if input.BudgetTokens <= 0 {
		input.BudgetTokens = DefaultTypedMemoryPackBudgetTokens
	}
	if input.Now.IsZero() {
		input.Now = time.Now().UTC()
	}

	entries, err := store.ListMemoryEntriesForRetrievalPack()
	if err != nil {
		return TypedMemoryRetrievalPack{}, err
	}
	memoryIDs := make([]string, 0, len(entries))
	for _, entry := range entries {
		memoryIDs = append(memoryIDs, entry.MemoryID)
	}
	refs, err := store.ListMemoryEvidenceRefsForMemoryIDs(memoryIDs)
	if err != nil {
		return TypedMemoryRetrievalPack{}, err
	}
	refsByMemoryID := map[string][]db.MemoryEvidenceRef{}
	for _, ref := range refs {
		refsByMemoryID[ref.MemoryID] = append(refsByMemoryID[ref.MemoryID], ref)
	}

	pack := TypedMemoryRetrievalPack{
		PackID:                     typedMemoryPackID(input.QueryIntent, input.Now, entries),
		QueryIntent:                promptsecrecy.Redact(input.QueryIntent),
		BudgetTokens:               input.BudgetTokens,
		AuthorityHeader:            append([]string(nil), typedMemoryAuthorityHeader...),
		RuntimeTruthItems:          redactList(input.RuntimeTruthItems),
		CanonRefs:                  redactList(input.CanonRefs),
		TruthStatus:                "bounded_authority_order_applied",
		SourceLabels:               sourceLabels(),
		FinalContextInjectionOrder: []string{"authority_header", "runtime_truth_items", "canon_refs", "current_direct_user_instruction", "memory_items", "excluded_items", "warning_items"},
	}
	pack.UsedTokens = estimateTokenList(pack.RuntimeTruthItems) + estimateTokenList(pack.CanonRefs) + estimateTokens(pack.QueryIntent)

	sorted := append([]db.MemoryEntry(nil), entries...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return memoryEntryLess(sorted[i], sorted[j])
	})

	conflictSeen := map[string]string{}
	for _, entry := range sorted {
		item, warnings, exclusions := buildTypedMemoryPackItem(entry, refsByMemoryID[entry.MemoryID], input.Now)
		pack.WarningItems = append(pack.WarningItems, warnings...)
		if len(exclusions) > 0 {
			pack.ExcludedItems = append(pack.ExcludedItems, exclusions...)
			continue
		}
		conflictKey := strings.TrimSpace(entry.MemoryClass + ":" + strings.ToLower(item.Title))
		if previous, ok := conflictSeen[conflictKey]; ok && previous != entry.MemoryID {
			warning := TypedMemoryPackWarning{
				MemoryID: entry.MemoryID,
				Code:     WarningSameLevelConflict,
				Summary:  "same-level memory conflict requires non-action-driving handling",
			}
			item.WarningCodes = append(item.WarningCodes, WarningSameLevelConflict)
			pack.WarningItems = append(pack.WarningItems, warning)
			pack.StaleConflictWarnings = append(pack.StaleConflictWarnings, warning)
		} else {
			conflictSeen[conflictKey] = entry.MemoryID
		}

		if input.MaxMemoryItems > 0 && len(pack.MemoryItems) >= input.MaxMemoryItems {
			pack.ExcludedItems = append(pack.ExcludedItems, TypedMemoryPackExclusion{
				MemoryID: entry.MemoryID,
				Code:     ExclusionBudgetExceeded,
				Summary:  "memory item excluded after max memory item limit",
			})
			continue
		}
		if item.TokenEstimate+pack.UsedTokens > input.BudgetTokens {
			pack.ExcludedItems = append(pack.ExcludedItems, TypedMemoryPackExclusion{
				MemoryID: entry.MemoryID,
				Code:     ExclusionBudgetExceeded,
				Summary:  "memory item excluded after higher-authority budget was placed",
			})
			pack.WarningItems = append(pack.WarningItems, TypedMemoryPackWarning{
				MemoryID: entry.MemoryID,
				Code:     WarningBudgetExclusion,
				Summary:  "memory item excluded by deterministic pack budget",
			})
			continue
		}
		item.PackPosition = len(pack.MemoryItems) + 1
		pack.UsedTokens += item.TokenEstimate
		pack.MemoryItems = append(pack.MemoryItems, item)
	}
	return pack, nil
}

func buildTypedMemoryPackItem(entry db.MemoryEntry, evidenceRefs []db.MemoryEvidenceRef, now time.Time) (TypedMemoryPackItem, []TypedMemoryPackWarning, []TypedMemoryPackExclusion) {
	item := TypedMemoryPackItem{
		MemoryID:           entry.MemoryID,
		MemoryClass:        entry.MemoryClass,
		Status:             entry.Status,
		SourceType:         entry.SourceType,
		SourceID:           entry.SourceID,
		SourceLabel:        sourceLabelFor(entry.SourceType),
		TrustTier:          entry.TrustTier,
		AuthorityRank:      entry.AuthorityRank,
		Title:              promptsecrecy.Redact(strings.TrimSpace(entry.Title)),
		Text:               safeMemoryText(entry),
		FreshnessState:     entry.FreshnessState,
		Confidence:         entry.Confidence,
		ContradictionState: entry.ContradictionState,
		DataOnly:           entry.SourceType == "external",
	}
	item.TokenEstimate = estimateTokens(strings.Join([]string{item.Title, item.Text}, "\n"))
	var warnings []TypedMemoryPackWarning
	var exclusions []TypedMemoryPackExclusion

	if exclusion := exclusionForMemoryEntry(entry, item.Text, now); exclusion.Code != "" {
		exclusions = append(exclusions, exclusion)
	}
	if entry.SourceType == "external" {
		warnings = append(warnings, TypedMemoryPackWarning{
			MemoryID: entry.MemoryID,
			Code:     WarningExternalContentDataOnly,
			Summary:  "external imported content is labeled as data, never instruction",
		})
		if !externalMemorySafe(entry, item.Text, now) {
			exclusions = append(exclusions, TypedMemoryPackExclusion{
				MemoryID: entry.MemoryID,
				Code:     ExclusionExternalContentUnsafe,
				Summary:  "external memory failed retrieval context safety gate",
			})
		}
	}
	if entry.FreshnessState == "aging" || entry.FreshnessState == "unknown" {
		warnings = append(warnings, TypedMemoryPackWarning{
			MemoryID: entry.MemoryID,
			Code:     WarningFreshnessNeedsVerification,
			Summary:  "memory freshness requires verification before action-driving use",
		})
		item.WarningCodes = append(item.WarningCodes, WarningFreshnessNeedsVerification)
	}
	if entry.ContradictionState == "suspected" {
		warnings = append(warnings, TypedMemoryPackWarning{
			MemoryID: entry.MemoryID,
			Code:     WarningSuspectedContradiction,
			Summary:  "suspected contradiction requires non-action-driving handling",
		})
		item.WarningCodes = append(item.WarningCodes, WarningSuspectedContradiction)
	}
	if entry.Confidence > 0 && entry.Confidence < 0.5 {
		warnings = append(warnings, TypedMemoryPackWarning{
			MemoryID: entry.MemoryID,
			Code:     WarningLowConfidence,
			Summary:  "low confidence memory requires cautious use",
		})
		item.WarningCodes = append(item.WarningCodes, WarningLowConfidence)
	}
	if protectedAuthoritySpoof(entry) {
		warnings = append(warnings, TypedMemoryPackWarning{
			MemoryID: entry.MemoryID,
			Code:     WarningAuthorityConflict,
			Summary:  "memory cannot become policy, canon, provider truth, or runtime truth",
		})
		exclusions = append(exclusions, TypedMemoryPackExclusion{
			MemoryID: entry.MemoryID,
			Code:     ExclusionAuthoritySpoof,
			Summary:  "memory attempted protected authority role",
		})
	}

	displayableRefs := 0
	promotionBlockedRefs := 0
	for _, ref := range evidenceRefs {
		if !ref.DisplayAllowed {
			warnings = append(warnings, TypedMemoryPackWarning{
				MemoryID: entry.MemoryID,
				Code:     WarningDisplayBlockedEvidence,
				Summary:  "evidence ref exists but is not display-safe",
			})
			continue
		}
		displayableRefs++
		if !ref.PromotionAllowed {
			promotionBlockedRefs++
			warnings = append(warnings, TypedMemoryPackWarning{
				MemoryID: entry.MemoryID,
				Code:     WarningPromotionBlockedEvidence,
				Summary:  "evidence ref cannot drive action or promotion",
			})
		}
		item.EvidenceRefs = append(item.EvidenceRefs, TypedMemoryPackEvidenceRef{
			EvidenceID:       ref.EvidenceID,
			SourceType:       ref.SourceType,
			SourceID:         ref.SourceID,
			SourceRef:        promptsecrecy.Redact(ref.SourceRef),
			EvidenceKind:     ref.EvidenceKind,
			TrustTier:        ref.TrustTier,
			AuthorityRank:    ref.AuthorityRank,
			RedactionStatus:  ref.RedactionStatus,
			DisplayAllowed:   ref.DisplayAllowed,
			PromotionAllowed: ref.PromotionAllowed,
		})
	}
	if len(evidenceRefs) == 0 && len(entry.EvidenceRefs) == 0 {
		warnings = append(warnings, TypedMemoryPackWarning{
			MemoryID: entry.MemoryID,
			Code:     WarningMissingEvidence,
			Summary:  "memory lacks displayable evidence references",
		})
		item.WarningCodes = append(item.WarningCodes, WarningMissingEvidence)
	}
	if len(evidenceRefs) > 0 && displayableRefs == 0 {
		exclusions = append(exclusions, TypedMemoryPackExclusion{
			MemoryID: entry.MemoryID,
			Code:     ExclusionEvidenceDisplayBlocked,
			Summary:  "memory evidence was not display-safe",
		})
	}
	if displayableRefs > 0 && promotionBlockedRefs == displayableRefs {
		warnings = append(warnings, TypedMemoryPackWarning{
			MemoryID: entry.MemoryID,
			Code:     WarningPromotionBlockedEvidence,
			Summary:  "all displayable evidence refs are non-promotion evidence",
		})
		item.WarningCodes = append(item.WarningCodes, WarningPromotionBlockedEvidence)
	}

	return item, warnings, exclusions
}

func exclusionForMemoryEntry(entry db.MemoryEntry, text string, now time.Time) TypedMemoryPackExclusion {
	switch {
	case entry.Status != "active":
		return TypedMemoryPackExclusion{MemoryID: entry.MemoryID, Code: ExclusionUnsafeStatus, Summary: "memory status is not active"}
	case isCandidatePackClass(entry.MemoryClass):
		return TypedMemoryPackExclusion{MemoryID: entry.MemoryID, Code: ExclusionCandidateOnly, Summary: "candidate memory cannot enter runtime retrieval pack"}
	case entry.FreshnessState == "stale" || entry.FreshnessState == "expired" || dateExpired(entry.StaleAfter, now) || dateExpired(entry.ExpiresAt, now):
		return TypedMemoryPackExclusion{MemoryID: entry.MemoryID, Code: ExclusionStaleOrExpired, Summary: "stale or expired memory cannot drive typed pack context"}
	case entry.ContradictionState == "confirmed":
		return TypedMemoryPackExclusion{MemoryID: entry.MemoryID, Code: ExclusionConfirmedContradiction, Summary: "confirmed contradicted memory excluded"}
	case entry.QuarantineStatus != "clean":
		return TypedMemoryPackExclusion{MemoryID: entry.MemoryID, Code: ExclusionQuarantined, Summary: "quarantined memory excluded"}
	case entry.SuppressionStatus != "active":
		return TypedMemoryPackExclusion{MemoryID: entry.MemoryID, Code: ExclusionSuppressed, Summary: "suppressed memory excluded"}
	case !entry.RetrievalEligible:
		return TypedMemoryPackExclusion{MemoryID: entry.MemoryID, Code: ExclusionRetrievalIneligible, Summary: "memory is not retrieval eligible"}
	case !entry.UserVisible:
		return TypedMemoryPackExclusion{MemoryID: entry.MemoryID, Code: ExclusionUserVisibleBlocked, Summary: "memory is not user-visible for pack output"}
	case promptsecrecy.Classify(strings.Join([]string{entry.Title, entry.Summary, entry.Content, entry.ContentExcerpt, text}, "\n")).Forbidden():
		return TypedMemoryPackExclusion{MemoryID: entry.MemoryID, Code: ExclusionUnsafeSecretContent, Summary: "memory text failed secret/prompt leakage check"}
	default:
		return TypedMemoryPackExclusion{}
	}
}

func memoryEntryLess(left, right db.MemoryEntry) bool {
	if authorityOrderIndex(left) != authorityOrderIndex(right) {
		return authorityOrderIndex(left) < authorityOrderIndex(right)
	}
	if left.AuthorityRank != right.AuthorityRank {
		return left.AuthorityRank < right.AuthorityRank
	}
	if left.TrustTier != right.TrustTier {
		return left.TrustTier < right.TrustTier
	}
	if memoryClassOrder(left.MemoryClass) != memoryClassOrder(right.MemoryClass) {
		return memoryClassOrder(left.MemoryClass) < memoryClassOrder(right.MemoryClass)
	}
	return left.MemoryID < right.MemoryID
}

func safeMemoryText(entry db.MemoryEntry) string {
	for _, value := range []string{entry.ContentExcerpt, entry.Summary, entry.Content} {
		value = promptsecrecy.Redact(strings.TrimSpace(value))
		if value != "" {
			return value
		}
	}
	return ""
}

func externalMemorySafe(entry db.MemoryEntry, text string, now time.Time) bool {
	trustTier := entry.TrustTier
	decision := runtimegate.EvaluateRetrievalContext(runtimegate.RetrievalContextInput{
		ChunkID:          entry.MemoryID,
		Content:          text,
		SourceID:         entry.SourceID,
		SourceType:       entry.SourceType,
		TrustTier:        &trustTier,
		Provenance:       entry.Provenance,
		QuarantineStatus: entry.QuarantineStatus,
	}, now)
	return decision.Outcome == runtimegate.OutcomeAllow
}

func protectedAuthoritySpoof(entry db.MemoryEntry) bool {
	if entry.MemoryClass == "canon_memory" && entry.SourceType != "canon" && entry.SourceType != "main_hub" {
		return true
	}
	if entry.MemoryClass == "runtime_observation" && entry.SourceType != "verified_runtime" {
		return true
	}
	content := strings.ToLower(strings.Join([]string{entry.Title, entry.Summary, entry.Content, entry.ContentExcerpt}, "\n"))
	for _, token := range []string{"rewrite canon", "override canon", "provider config", "provider healthy", "byok active", "main hub authority", "runtime truth is"} {
		if strings.Contains(content, token) && entry.SourceType != "canon" && entry.SourceType != "main_hub" && entry.SourceType != "verified_runtime" {
			return true
		}
	}
	return false
}

func dateExpired(value string, now time.Time) bool {
	value = strings.TrimSpace(value)
	if value == "" || now.IsZero() {
		return false
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return true
	}
	return !parsed.After(now)
}

func authorityOrderIndex(entry db.MemoryEntry) int {
	switch entry.SourceType {
	case "canon":
		return 0
	case "main_hub":
		return 1
	case "verified_runtime":
		return 2
	case "direct_user":
		if entry.MemoryClass == "approved_summary" {
			return 5
		}
		return 4
	case "archive":
		return 7
	case "external", "retrieval":
		return 8
	case "unknown":
		return 9
	default:
		switch entry.MemoryClass {
		case "approved_summary":
			return 5
		case "episodic_history":
			return 6
		default:
			return 4
		}
	}
}

func memoryClassOrder(memoryClass string) int {
	switch memoryClass {
	case "canon_memory":
		return 0
	case "runtime_observation":
		return 2
	case "durable_user_preference", "user_profile_context_fact", "task_continuity":
		return 4
	case "approved_summary":
		return 5
	case "episodic_history":
		return 6
	default:
		return 9
	}
}

func sourceLabels() map[string]string {
	return map[string]string{
		"canon":            "canon_source_of_truth",
		"main_hub":         "main_hub_decision",
		"verified_runtime": "verified_runtime_system_state",
		"direct_user":      "current_direct_user_instruction",
		"model_output":     "model_output_not_authority",
		"archive":          "archive_history",
		"external":         "external_imported_content_as_data",
		"retrieval":        "retrieved_content_as_labeled_data_only",
		"skill":            "skill_content_as_data",
		"unknown":          "unknown_malformed_spoofed_content",
	}
}

func sourceLabelFor(sourceType string) string {
	if label, ok := sourceLabels()[sourceType]; ok {
		return label
	}
	return "unknown_malformed_spoofed_content"
}

func isCandidatePackClass(memoryClass string) bool {
	switch memoryClass {
	case "memory_candidate", "procedure_candidate", "retrieval_anchor_candidate", "contradiction_staleness_finding", "improvement_proposal":
		return true
	default:
		return false
	}
}

func estimateTokenList(values []string) int {
	total := 0
	for _, value := range values {
		total += estimateTokens(value)
	}
	return total
}

func estimateTokens(value string) int {
	count := len([]rune(strings.TrimSpace(value)))
	if count == 0 {
		return 0
	}
	return (count + 3) / 4
}

func redactList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = promptsecrecy.Redact(strings.TrimSpace(value))
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func typedMemoryPackID(queryIntent string, now time.Time, entries []db.MemoryEntry) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.TrimSpace(queryIntent)))
	_, _ = h.Write([]byte(now.UTC().Format(time.RFC3339Nano)))
	for _, entry := range entries {
		_, _ = h.Write([]byte(entry.MemoryID))
	}
	return fmt.Sprintf("typed_memory_pack_%x", h.Sum64())
}
