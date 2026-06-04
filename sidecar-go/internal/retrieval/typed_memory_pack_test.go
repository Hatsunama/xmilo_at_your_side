package retrieval

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xmilo/sidecar-go/internal/db"
)

func TestTypedMemoryPackIncludesEligibleApprovedMemoryOnly(t *testing.T) {
	store := openTypedMemoryPackStore(t)
	mustStoreMemoryEntry(t, store, typedPackMemoryEntry("pref.concise", "durable_user_preference", "Keep replies concise."))
	pack := mustBuildTypedMemoryPack(t, store, TypedMemoryPackInput{QueryIntent: "Use memory.", Now: typedPackNow()})

	if len(pack.MemoryItems) != 1 || pack.MemoryItems[0].MemoryID != "pref.concise" {
		t.Fatalf("expected eligible memory item, got %#v", pack.MemoryItems)
	}
	if !strings.Contains(pack.MemoryItems[0].Text, "Keep replies concise") {
		t.Fatalf("expected safe memory text, got %#v", pack.MemoryItems[0])
	}
}

func TestTypedMemoryPackExcludesUnsafeMemoryStates(t *testing.T) {
	store := openTypedMemoryPackStore(t)
	tests := []struct {
		id       string
		expected string
		mutate   func(*db.MemoryEntry)
	}{
		{id: "memory.stale", expected: ExclusionStaleOrExpired, mutate: func(e *db.MemoryEntry) { e.FreshnessState = "stale" }},
		{id: "memory.suppressed", expected: ExclusionSuppressed, mutate: func(e *db.MemoryEntry) { e.SuppressionStatus = "suppressed" }},
		{id: "memory.quarantined", expected: ExclusionQuarantined, mutate: func(e *db.MemoryEntry) { e.QuarantineStatus = "quarantined" }},
		{id: "memory.contradicted", expected: ExclusionConfirmedContradiction, mutate: func(e *db.MemoryEntry) { e.ContradictionState = "confirmed" }},
	}
	for _, tt := range tests {
		entry := typedPackMemoryEntry(tt.id, "durable_user_preference", "unsafe memory")
		tt.mutate(&entry)
		mustStoreMemoryEntry(t, store, entry)
	}

	pack := mustBuildTypedMemoryPack(t, store, TypedMemoryPackInput{QueryIntent: "Use safe memory.", Now: typedPackNow()})
	if len(pack.MemoryItems) != 0 {
		t.Fatalf("unsafe memory entered pack: %#v", pack.MemoryItems)
	}
	for _, tt := range tests {
		if !hasPackExclusion(pack, tt.id, tt.expected) {
			t.Fatalf("expected exclusion %s for %s, got %#v", tt.expected, tt.id, pack.ExcludedItems)
		}
	}
}

func TestTypedMemoryPackWarnsAgingUnknownSuspectedLowConfidence(t *testing.T) {
	store := openTypedMemoryPackStore(t)
	aging := typedPackMemoryEntry("memory.aging", "durable_user_preference", "aging preference")
	aging.FreshnessState = "aging"
	mustStoreMemoryEntry(t, store, aging)
	unknown := typedPackMemoryEntry("memory.unknown", "durable_user_preference", "unknown preference")
	unknown.FreshnessState = "unknown"
	unknown.Confidence = 0.4
	unknown.ContradictionState = "suspected"
	mustStoreMemoryEntry(t, store, unknown)

	pack := mustBuildTypedMemoryPack(t, store, TypedMemoryPackInput{QueryIntent: "Use cautious memory.", Now: typedPackNow()})
	if len(pack.MemoryItems) != 2 {
		t.Fatalf("expected aging/unknown memory included with warnings, got %#v", pack.MemoryItems)
	}
	for _, code := range []string{WarningFreshnessNeedsVerification, WarningSuspectedContradiction, WarningLowConfidence} {
		if !hasPackWarning(pack, "memory.unknown", code) && !hasPackWarning(pack, "memory.aging", code) {
			t.Fatalf("expected warning %s, got %#v", code, pack.WarningItems)
		}
	}
}

func TestTypedMemoryPackExcludesCandidatesAndModelOutputActiveMemory(t *testing.T) {
	store := openTypedMemoryPackStore(t)
	if err := store.UpsertMemoryCandidate(db.MemoryCandidate{
		CandidateID:        "candidate.model",
		CandidateType:      "memory_candidate",
		Status:             "generated",
		Title:              "model candidate",
		Summary:            "candidate summary",
		Content:            "candidate content",
		SourceType:         "model_output",
		SourceID:           "turn_1",
		TrustTier:          6,
		AuthorityRank:      "rank_700_model_output",
		Provenance:         map[string]any{"source_type": "model_output"},
		EvidenceRefs:       []string{"evidence.turn_1"},
		FreshnessState:     "fresh",
		Confidence:         0.5,
		ContradictionState: "none",
		QuarantineStatus:   "clean",
		SuppressionStatus:  "active",
	}); err != nil {
		t.Fatalf("store candidate: %v", err)
	}
	activeModel := typedPackMemoryEntry("memory.model.active", "approved_summary", "model content")
	activeModel.SourceType = "model_output"
	activeModel.SourceID = "turn_1"
	activeModel.Provenance = map[string]any{"source_type": "model_output"}
	if err := store.UpsertMemoryEntry(activeModel); err == nil || err.Error() != "memory_entry_model_output_active_blocked" {
		t.Fatalf("expected active model-output memory blocked by DB gate, got %v", err)
	}

	pack := mustBuildTypedMemoryPack(t, store, TypedMemoryPackInput{QueryIntent: "Use approved memory.", Now: typedPackNow()})
	if len(pack.MemoryItems) != 0 {
		t.Fatalf("candidate/model output entered runtime pack: %#v", pack.MemoryItems)
	}
}

func TestTypedMemoryPackLabelsExternalContentAsDataOnly(t *testing.T) {
	store := openTypedMemoryPackStore(t)
	external := typedPackMemoryEntry("external.safe", "approved_summary", "safe external content")
	external.SourceType = "external"
	external.SourceID = "external_doc"
	external.AuthorityRank = "rank_800_external"
	external.Provenance = map[string]any{"source_type": "external", "source_id": "external_doc"}
	external.ExternalContentIsNotInstruction = true
	mustStoreMemoryEntry(t, store, external)

	pack := mustBuildTypedMemoryPack(t, store, TypedMemoryPackInput{QueryIntent: "Use external content cautiously.", Now: typedPackNow()})
	if len(pack.MemoryItems) != 1 || !pack.MemoryItems[0].DataOnly {
		t.Fatalf("external memory was not labeled data-only: %#v", pack.MemoryItems)
	}
	if pack.MemoryItems[0].SourceLabel != "external_imported_content_as_data" {
		t.Fatalf("unexpected source label: %#v", pack.MemoryItems[0])
	}
	if !hasPackWarning(pack, "external.safe", WarningExternalContentDataOnly) {
		t.Fatalf("expected external data-only warning, got %#v", pack.WarningItems)
	}
}

func TestTypedMemoryPackAuthorityOrderDeterministic(t *testing.T) {
	store := openTypedMemoryPackStore(t)
	preference := typedPackMemoryEntry("pref.user", "durable_user_preference", "user preference")
	runtime := typedPackMemoryEntry("runtime.verified", "runtime_observation", "runtime observed")
	runtime.SourceType = "verified_runtime"
	runtime.SourceID = "runtime"
	runtime.AuthorityRank = "rank_200_runtime"
	runtime.Provenance = map[string]any{"source_type": "verified_runtime"}
	canon := typedPackMemoryEntry("canon.truth", "canon_memory", "canon truth")
	canon.SourceType = "canon"
	canon.SourceID = "canon"
	canon.AuthorityRank = "rank_000_canon"
	canon.Provenance = map[string]any{"source_type": "canon"}
	mustStoreMemoryEntry(t, store, preference)
	mustStoreMemoryEntry(t, store, runtime)
	mustStoreMemoryEntry(t, store, canon)

	pack := mustBuildTypedMemoryPack(t, store, TypedMemoryPackInput{
		QueryIntent:       "Use ordered memory.",
		RuntimeTruthItems: []string{"runtime truth outranks memory"},
		CanonRefs:         []string{"canon outranks memory"},
		Now:               typedPackNow(),
	})
	if strings.Join(pack.AuthorityHeader, ",") != strings.Join([]string{
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
	}, ",") {
		t.Fatalf("authority header drifted: %#v", pack.AuthorityHeader)
	}
	got := []string{pack.MemoryItems[0].MemoryID, pack.MemoryItems[1].MemoryID, pack.MemoryItems[2].MemoryID}
	want := []string{"canon.truth", "runtime.verified", "pref.user"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("expected authority ordered items %v, got %v", want, got)
	}
	if pack.FinalContextInjectionOrder[0] != "authority_header" || pack.FinalContextInjectionOrder[1] != "runtime_truth_items" {
		t.Fatalf("runtime/canon/current user intent order not preserved: %#v", pack.FinalContextInjectionOrder)
	}
}

func TestTypedMemoryPackSameLevelConflictWarnsNotActionDrivingTruth(t *testing.T) {
	store := openTypedMemoryPackStore(t)
	left := typedPackMemoryEntry("pref.left", "durable_user_preference", "prefer quiet replies")
	left.Title = "tone preference"
	right := typedPackMemoryEntry("pref.right", "durable_user_preference", "prefer expressive replies")
	right.Title = "tone preference"
	mustStoreMemoryEntry(t, store, left)
	mustStoreMemoryEntry(t, store, right)

	pack := mustBuildTypedMemoryPack(t, store, TypedMemoryPackInput{QueryIntent: "Use tone memory.", Now: typedPackNow()})
	if len(pack.MemoryItems) != 2 {
		t.Fatalf("expected both same-level memories included for non-action handling, got %#v", pack.MemoryItems)
	}
	if !hasPackWarning(pack, "pref.right", WarningSameLevelConflict) {
		t.Fatalf("expected same-level conflict warning, got %#v", pack.WarningItems)
	}
	if len(pack.StaleConflictWarnings) == 0 {
		t.Fatalf("expected conflict warning copied to stale/conflict warnings")
	}
}

func TestTypedMemoryPackEvidenceFlagsAndSecretRedaction(t *testing.T) {
	store := openTypedMemoryPackStore(t)
	visible := typedPackMemoryEntry("memory.visible.evidence", "durable_user_preference", "visible evidence memory")
	visible.EvidenceRefs = nil
	mustStoreMemoryEntry(t, store, visible)
	if err := store.AppendMemoryEvidenceRef(db.MemoryEvidenceRef{
		EvidenceID:       "evidence.visible",
		MemoryID:         visible.MemoryID,
		SourceType:       "direct_user",
		SourceID:         "user",
		SourceRef:        "user said visible preference",
		EvidenceKind:     "user_statement",
		TrustTier:        2,
		AuthorityRank:    "rank_300_direct_user",
		DisplayAllowed:   true,
		PromotionAllowed: false,
	}); err != nil {
		t.Fatalf("append visible evidence: %v", err)
	}

	blocked := typedPackMemoryEntry("memory.blocked.evidence", "durable_user_preference", "blocked evidence memory")
	blocked.EvidenceRefs = nil
	mustStoreMemoryEntry(t, store, blocked)
	if err := store.AppendMemoryEvidenceRef(db.MemoryEvidenceRef{
		EvidenceID:       "evidence.blocked",
		MemoryID:         blocked.MemoryID,
		SourceType:       "direct_user",
		SourceID:         "user",
		SourceRef:        "api_key=sk-user-provided-value",
		EvidenceKind:     "user_statement",
		TrustTier:        2,
		AuthorityRank:    "rank_300_direct_user",
		DisplayAllowed:   false,
		PromotionAllowed: false,
	}); err != nil {
		t.Fatalf("append blocked evidence: %v", err)
	}

	pack := mustBuildTypedMemoryPack(t, store, TypedMemoryPackInput{QueryIntent: "Use evidence memory.", Now: typedPackNow()})
	if !hasPackWarning(pack, visible.MemoryID, WarningPromotionBlockedEvidence) {
		t.Fatalf("expected promotion-blocked evidence warning, got %#v", pack.WarningItems)
	}
	if !hasPackExclusion(pack, blocked.MemoryID, ExclusionEvidenceDisplayBlocked) {
		t.Fatalf("expected display-blocked evidence exclusion, got %#v", pack.ExcludedItems)
	}
	raw := typedPackString(pack)
	for _, forbidden := range []string{"sk-user-provided-value", "api_key="} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("pack leaked secret evidence %q: %s", forbidden, raw)
		}
	}
}

func TestTypedMemoryPackBudgetAndPackPositionStable(t *testing.T) {
	store := openTypedMemoryPackStore(t)
	first := typedPackMemoryEntry("pref.first", "durable_user_preference", "short")
	second := typedPackMemoryEntry("pref.second", "durable_user_preference", strings.Repeat("long memory ", 50))
	mustStoreMemoryEntry(t, store, first)
	mustStoreMemoryEntry(t, store, second)

	pack := mustBuildTypedMemoryPack(t, store, TypedMemoryPackInput{QueryIntent: "Budget memory.", BudgetTokens: 20, Now: typedPackNow()})
	if len(pack.MemoryItems) != 1 || pack.MemoryItems[0].MemoryID != "pref.first" || pack.MemoryItems[0].PackPosition != 1 {
		t.Fatalf("expected stable first item, got %#v", pack.MemoryItems)
	}
	if !hasPackExclusion(pack, "pref.second", ExclusionBudgetExceeded) {
		t.Fatalf("expected budget exclusion for second item, got %#v", pack.ExcludedItems)
	}
	if !hasPackWarning(pack, "pref.second", WarningBudgetExclusion) {
		t.Fatalf("expected budget warning for second item, got %#v", pack.WarningItems)
	}
}

func TestTypedMemoryPackDoesNotUseVectorRetrievalOrRetrievalRecords(t *testing.T) {
	store := openTypedMemoryPackStore(t)
	mustStoreMemoryEntry(t, store, typedPackMemoryEntry("pref.only", "durable_user_preference", "typed memory"))
	if err := store.UpsertRetrievalRecord(db.RetrievalRecord{
		ChunkID:          "vector.chunk",
		SourceID:         "vector",
		SourceType:       db.RetrievalSourceMemory,
		TrustTier:        2,
		AuthorityRank:    "rank_300_direct_user",
		Provenance:       map[string]any{"source_id": "vector"},
		Freshness:        "fresh",
		Hash:             "sha256-vector",
		QuarantineStatus: db.RetrievalQuarantineClean,
		ContentSummary:   "vector-only retrieval record",
		RawContentRef:    "ref://vector",
		UsedVector:       true,
		UsedLexical:      false,
		RetrievalBackend: "vector",
	}); err != nil {
		t.Fatalf("store retrieval record: %v", err)
	}

	pack := mustBuildTypedMemoryPack(t, store, TypedMemoryPackInput{QueryIntent: "Use typed memory only.", Now: typedPackNow()})
	if len(pack.MemoryItems) != 1 || pack.MemoryItems[0].MemoryID != "pref.only" {
		t.Fatalf("pack used retrieval_records/vector path: %#v", pack.MemoryItems)
	}
}

func openTypedMemoryPackStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func typedPackNow() time.Time {
	return time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
}

func typedPackMemoryEntry(memoryID, memoryClass, content string) db.MemoryEntry {
	return db.MemoryEntry{
		MemoryID:                        memoryID,
		MemoryClass:                     memoryClass,
		Status:                          "active",
		Title:                           "safe title " + memoryID,
		Summary:                         content,
		Content:                         content,
		ContentExcerpt:                  content,
		SourceType:                      "direct_user",
		SourceID:                        "user",
		TrustTier:                       2,
		AuthorityRank:                   "rank_300_direct_user",
		Provenance:                      map[string]any{"source_type": "direct_user", "source_id": "user"},
		EvidenceRefs:                    []string{"evidence." + memoryID},
		FreshnessState:                  "fresh",
		Confidence:                      0.8,
		ContradictionState:              "none",
		QuarantineStatus:                "clean",
		SuppressionStatus:               "active",
		AllowedActions:                  []string{"view"},
		ExternalContentIsNotInstruction: true,
		RetrievalEligible:               true,
		RetrievalReason:                 "safe retrieval",
		EmbeddingStatus:                 "not_needed",
		UserVisible:                     true,
	}
}

func mustStoreMemoryEntry(t *testing.T, store *db.Store, entry db.MemoryEntry) {
	t.Helper()
	if err := store.UpsertMemoryEntry(entry); err != nil {
		t.Fatalf("store memory %s: %v", entry.MemoryID, err)
	}
}

func mustBuildTypedMemoryPack(t *testing.T, store *db.Store, input TypedMemoryPackInput) TypedMemoryRetrievalPack {
	t.Helper()
	pack, err := BuildTypedMemoryRetrievalPack(store, input)
	if err != nil {
		t.Fatalf("build pack: %v", err)
	}
	return pack
}

func hasPackWarning(pack TypedMemoryRetrievalPack, memoryID, code string) bool {
	for _, warning := range pack.WarningItems {
		if warning.MemoryID == memoryID && warning.Code == code {
			return true
		}
	}
	return false
}

func hasPackExclusion(pack TypedMemoryRetrievalPack, memoryID, code string) bool {
	for _, exclusion := range pack.ExcludedItems {
		if exclusion.MemoryID == memoryID && exclusion.Code == code {
			return true
		}
	}
	return false
}

func typedPackString(pack TypedMemoryRetrievalPack) string {
	var b strings.Builder
	for _, item := range pack.MemoryItems {
		b.WriteString(item.Text)
		for _, ref := range item.EvidenceRefs {
			b.WriteString(ref.SourceRef)
		}
	}
	for _, warning := range pack.WarningItems {
		b.WriteString(warning.Summary)
	}
	for _, exclusion := range pack.ExcludedItems {
		b.WriteString(exclusion.Summary)
	}
	return b.String()
}
