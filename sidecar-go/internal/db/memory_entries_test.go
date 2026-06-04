package db

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestMemorySchemaMigrationCreatesPhase18DTables(t *testing.T) {
	store := openMemoryEntryStore(t)
	version, err := store.SchemaVersion()
	if err != nil {
		t.Fatalf("schema version: %v", err)
	}
	if version != 10 {
		t.Fatalf("expected schema version 10, got %d", version)
	}
	for _, table := range []string{"memory_entries", "memory_evidence_refs", "memory_action_audit", "memory_findings", "memory_candidates"} {
		if got := memoryTableCount(t, store, table); got != 1 {
			t.Fatalf("expected %s table, got count %d", table, got)
		}
	}
	for _, column := range []string{"confidence", "contradiction_state", "evidence_refs_json", "suppression_status", "used_vector", "used_lexical"} {
		exists, err := store.columnExists("retrieval_records", column)
		if err != nil {
			t.Fatalf("check retrieval column %s: %v", column, err)
		}
		if !exists {
			t.Fatalf("expected retrieval metadata column %s", column)
		}
	}
}

func TestMemorySchemaMigrationIsIdempotentAndPreservesExistingRows(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.sqlite")
	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := store.AddTaskHistory("task_existing", "prompt", "completed", "summary"); err != nil {
		t.Fatalf("add task history: %v", err)
	}
	if err := store.UpsertRetrievalRecord(testRetrievalRecord("retrieval.existing", RetrievalSourceMemory, "memory", 2, "rank_200_memory")); err != nil {
		t.Fatalf("upsert retrieval: %v", err)
	}
	if err := store.UpsertConsolidationRun(ConsolidationRun{
		RunID:       "nightly_consolidation_2026-06-03",
		ArchiveDate: "2026-06-03",
		Trigger:     "scheduled",
		Status:      ConsolidationRunCompletedSummary,
	}); err != nil {
		t.Fatalf("upsert consolidation: %v", err)
	}
	_ = store.Close()

	reopened, err := Open(dbPath)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer reopened.Close()
	version, err := reopened.SchemaVersion()
	if err != nil {
		t.Fatalf("schema version: %v", err)
	}
	if version != 10 {
		t.Fatalf("expected schema version 10 after reopen, got %d", version)
	}
	if got := memoryQueryCount(t, reopened, `SELECT COUNT(*) FROM task_history WHERE task_id = 'task_existing'`); got != 1 {
		t.Fatalf("task_history row not preserved: %d", got)
	}
	if got := memoryQueryCount(t, reopened, `SELECT COUNT(*) FROM retrieval_records WHERE chunk_id = 'retrieval.existing'`); got != 1 {
		t.Fatalf("retrieval row not preserved: %d", got)
	}
	if got := memoryQueryCount(t, reopened, `SELECT COUNT(*) FROM consolidation_runs WHERE run_id = 'nightly_consolidation_2026-06-03'`); got != 1 {
		t.Fatalf("consolidation row not preserved: %d", got)
	}
}

func TestMemoryEntryRejectsInvalidClassAndStatus(t *testing.T) {
	store := openMemoryEntryStore(t)
	entry := testMemoryEntry("memory.invalid.class", "durable_user_preference")
	entry.MemoryClass = "freeform_memory"
	if err := store.UpsertMemoryEntry(entry); err == nil || err.Error() != "memory_entry_invalid_class" {
		t.Fatalf("expected invalid class error, got %v", err)
	}
	entry = testMemoryEntry("memory.invalid.status", "durable_user_preference")
	entry.Status = "done"
	if err := store.UpsertMemoryEntry(entry); err == nil || err.Error() != "memory_entry_invalid_status" {
		t.Fatalf("expected invalid status error, got %v", err)
	}
	entry = testMemoryEntry("memory.candidate.class", "memory_candidate")
	if err := store.UpsertMemoryEntry(entry); err == nil || err.Error() != "memory_entry_candidate_class_requires_memory_candidates" {
		t.Fatalf("expected candidate class rejection, got %v", err)
	}
}

func TestMemoryEntryBlocksDirectModelOutputActiveWrite(t *testing.T) {
	store := openMemoryEntryStore(t)
	entry := testMemoryEntry("memory.model.active", "approved_summary")
	entry.SourceType = "model_output"
	entry.SourceID = "turn_1"
	entry.Provenance = map[string]any{"source_type": "model_output", "turn_id": "turn_1"}
	if err := store.UpsertMemoryEntry(entry); err == nil || err.Error() != "memory_entry_model_output_active_blocked" {
		t.Fatalf("expected model output active block, got %v", err)
	}
}

func TestMemoryCandidateStoresModelOutputInertly(t *testing.T) {
	store := openMemoryEntryStore(t)
	candidate := testMemoryCandidate("candidate.model")
	candidate.SourceType = "model_output"
	candidate.Content = "candidate summary from model output"
	if err := store.UpsertMemoryCandidate(candidate); err != nil {
		t.Fatalf("upsert candidate: %v", err)
	}
	loaded, err := store.GetMemoryCandidate(candidate.CandidateID)
	if err != nil {
		t.Fatalf("get candidate: %v", err)
	}
	if loaded == nil || loaded.CandidateType != "memory_candidate" || loaded.Status != "generated" {
		t.Fatalf("candidate did not round-trip inertly: %#v", loaded)
	}
	if got := memoryQueryCount(t, store, `SELECT COUNT(*) FROM memory_entries WHERE memory_id = 'candidate.model'`); got != 0 {
		t.Fatalf("candidate created runtime memory entry: %d", got)
	}
	if got := memoryQueryCount(t, store, `SELECT COUNT(*) FROM retrieval_records WHERE source_id = 'candidate.model'`); got != 0 {
		t.Fatalf("candidate created retrieval record: %d", got)
	}
}

func TestUserCorrectionSupersedeBoundaries(t *testing.T) {
	store := openMemoryEntryStore(t)
	preference := testMemoryEntry("pref.original", "durable_user_preference")
	if err := store.UpsertMemoryEntry(preference); err != nil {
		t.Fatalf("upsert preference: %v", err)
	}
	correction := testMemoryEntry("pref.corrected", "durable_user_preference")
	correction.SupersedesMemoryID = preference.MemoryID
	if err := store.UpsertMemoryEntry(correction); err != nil {
		t.Fatalf("expected user correction to supersede preference: %v", err)
	}

	canon := testMemoryEntry("canon.original", "canon_memory")
	canon.SourceType = "canon"
	canon.AuthorityRank = "rank_000_canon"
	canon.Provenance = map[string]any{"source_type": "canon"}
	if err := store.UpsertMemoryEntry(canon); err != nil {
		t.Fatalf("upsert canon: %v", err)
	}
	canonRewrite := testMemoryEntry("canon.rewrite", "durable_user_preference")
	canonRewrite.SupersedesMemoryID = canon.MemoryID
	if err := store.UpsertMemoryEntry(canonRewrite); err == nil || err.Error() != "memory_entry_supersedes_protected_memory" {
		t.Fatalf("expected protected supersede block, got %v", err)
	}
	runtimeRewrite := testMemoryEntry("runtime.bad", "runtime_observation")
	if err := store.UpsertMemoryEntry(runtimeRewrite); err == nil || err.Error() != "memory_entry_runtime_truth_requires_verified_runtime" {
		t.Fatalf("expected runtime truth source block, got %v", err)
	}
	providerRewrite := testMemoryEntry("provider.bad", "durable_user_preference")
	providerRewrite.Content = "provider healthy and BYOK active"
	if err := store.UpsertMemoryEntry(providerRewrite); err == nil || err.Error() != "memory_entry_protected_truth_rewrite" {
		t.Fatalf("expected provider truth rewrite block, got %v", err)
	}
}

func TestMemoryActionsAppendAuditRows(t *testing.T) {
	store := openMemoryEntryStore(t)
	entry := testMemoryEntry("memory.audit", "durable_user_preference")
	if err := store.UpsertMemoryEntry(entry); err != nil {
		t.Fatalf("upsert memory: %v", err)
	}
	for _, action := range []string{"suppress", "delete_user_remove", "correct_supersede"} {
		if err := store.AppendMemoryActionAudit(MemoryActionAudit{
			AuditID:     "audit." + action,
			MemoryID:    entry.MemoryID,
			Action:      action,
			Actor:       "runtime",
			Reason:      "user requested " + action,
			BeforeState: map[string]any{"content": "Authorization: Bearer secret"},
			AfterState:  map[string]any{"status": action},
		}); err != nil {
			t.Fatalf("append audit %s: %v", action, err)
		}
	}
	if got := memoryQueryCount(t, store, `SELECT COUNT(*) FROM memory_action_audit WHERE memory_id = 'memory.audit'`); got != 3 {
		t.Fatalf("expected three audit rows, got %d", got)
	}
	var beforeJSON string
	if err := store.DB.QueryRow(`SELECT before_state_json FROM memory_action_audit WHERE audit_id = 'audit.suppress'`).Scan(&beforeJSON); err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if strings.Contains(beforeJSON, "Bearer") || strings.Contains(beforeJSON, "secret") {
		t.Fatalf("audit before state leaked secret: %s", beforeJSON)
	}
	audits, err := store.ListMemoryActionAudit()
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(audits) != 3 || audits[0].Action == "" || len(audits[0].BeforeState) == 0 {
		t.Fatalf("audit list did not round-trip rows: %#v", audits)
	}
}

func TestListMemoryCandidatesForVisibleControl(t *testing.T) {
	store := openMemoryEntryStore(t)
	candidate := testMemoryCandidate("candidate.visible.control")
	if err := store.UpsertMemoryCandidate(candidate); err != nil {
		t.Fatalf("upsert candidate: %v", err)
	}
	candidates, err := store.ListMemoryCandidates()
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(candidates) != 1 || candidates[0].CandidateID != candidate.CandidateID || candidates[0].Content != "candidate content" {
		t.Fatalf("candidate list did not round-trip: %#v", candidates)
	}
}

func TestMemoryFindingResolutionRequiresResolverAndAudit(t *testing.T) {
	store := openMemoryEntryStore(t)
	err := store.UpsertMemoryFinding(MemoryFinding{
		FindingID:   "finding.resolved.bad",
		FindingType: "contradiction",
		Confidence:  0.9,
		Status:      "resolved",
	})
	if err == nil || err.Error() != "memory_finding_resolution_requires_resolver_and_audit" {
		t.Fatalf("expected resolver/audit requirement, got %v", err)
	}
	if err := store.UpsertMemoryFinding(MemoryFinding{
		FindingID:     "finding.resolved.good",
		FindingType:   "contradiction",
		Confidence:    0.9,
		Status:        "resolved",
		Resolver:      "main_hub",
		AuditEventIDs: []string{"audit.resolve"},
	}); err != nil {
		t.Fatalf("expected resolved finding with resolver/audit: %v", err)
	}
}

func TestUnsafeMemoryCannotDriveRetrievalEligibility(t *testing.T) {
	store := openMemoryEntryStore(t)
	tests := []struct {
		name   string
		mutate func(*MemoryEntry)
	}{
		{name: "stale", mutate: func(e *MemoryEntry) { e.FreshnessState = "stale" }},
		{name: "quarantined", mutate: func(e *MemoryEntry) { e.QuarantineStatus = "quarantined" }},
		{name: "suppressed", mutate: func(e *MemoryEntry) { e.SuppressionStatus = "suppressed" }},
		{name: "contradicted", mutate: func(e *MemoryEntry) { e.ContradictionState = "confirmed" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := testMemoryEntry("memory."+tt.name, "durable_user_preference")
			entry.RetrievalEligible = true
			tt.mutate(&entry)
			if err := store.UpsertMemoryEntry(entry); err != nil {
				t.Fatalf("upsert memory: %v", err)
			}
			loaded, err := store.GetMemoryEntry(entry.MemoryID)
			if err != nil {
				t.Fatalf("get memory: %v", err)
			}
			if loaded.RetrievalEligible {
				t.Fatalf("%s memory remained retrieval eligible: %#v", tt.name, loaded)
			}
		})
	}
}

func TestConfirmedContradictedMemoryRequiresSafeState(t *testing.T) {
	store := openMemoryEntryStore(t)
	entry := testMemoryEntry("memory.contradicted", "durable_user_preference")
	entry.ContradictionState = "confirmed"
	entry.RetrievalEligible = true
	if err := store.UpsertMemoryEntry(entry); err != nil {
		t.Fatalf("upsert contradicted memory: %v", err)
	}
	if err := store.UpsertMemoryFinding(MemoryFinding{
		FindingID:    "finding.contradicted",
		MemoryIDs:    []string{entry.MemoryID},
		FindingType:  "contradiction",
		Confidence:   1,
		EvidenceRefs: []string{"evidence.user"},
		Status:       "needs_review",
	}); err != nil {
		t.Fatalf("upsert finding: %v", err)
	}
	loaded, err := store.GetMemoryEntry(entry.MemoryID)
	if err != nil {
		t.Fatalf("get memory: %v", err)
	}
	if loaded.RetrievalEligible {
		t.Fatalf("confirmed contradiction remained retrieval eligible: %#v", loaded)
	}
}

func TestMemoryEvidenceDisplayAndPromotionFlagsEnforced(t *testing.T) {
	store := openMemoryEntryStore(t)
	err := store.AppendMemoryEvidenceRef(MemoryEvidenceRef{
		EvidenceID:       "evidence.secret.bad",
		MemoryID:         "memory.secret",
		SourceType:       "direct_user",
		EvidenceKind:     "user_statement",
		TrustTier:        2,
		AuthorityRank:    "rank_300_direct_user",
		SourceRef:        "api_key=sk-user-provided-value",
		DisplayAllowed:   true,
		PromotionAllowed: true,
	})
	if err == nil || err.Error() != "memory_evidence_secret_requires_blocked_flags" {
		t.Fatalf("expected evidence flag block, got %v", err)
	}
	if err := store.AppendMemoryEvidenceRef(MemoryEvidenceRef{
		EvidenceID:       "evidence.secret.good",
		MemoryID:         "memory.secret",
		SourceType:       "direct_user",
		EvidenceKind:     "user_statement",
		TrustTier:        2,
		AuthorityRank:    "rank_300_direct_user",
		SourceRef:        "api_key=sk-user-provided-value",
		DisplayAllowed:   false,
		PromotionAllowed: false,
	}); err != nil {
		t.Fatalf("expected redacted evidence storage: %v", err)
	}
	var ref string
	if err := store.DB.QueryRow(`SELECT source_ref FROM memory_evidence_refs WHERE evidence_id = 'evidence.secret.good'`).Scan(&ref); err != nil {
		t.Fatalf("read evidence: %v", err)
	}
	if strings.Contains(ref, "sk-user-provided-value") || strings.Contains(ref, "api_key=") {
		t.Fatalf("stored evidence leaked raw secret: %s", ref)
	}
}

func TestMemoryEntryRejectsSecretBearingActiveMemory(t *testing.T) {
	store := openMemoryEntryStore(t)
	entry := testMemoryEntry("memory.secret.active", "durable_user_preference")
	entry.Content = "api_key=sk-user-provided-value"
	if err := store.UpsertMemoryEntry(entry); err == nil || err.Error() != "memory_entry_secret_content" {
		t.Fatalf("expected secret content rejection, got %v", err)
	}
}

func TestMemoryEntryRejectsExternalContentAsInstruction(t *testing.T) {
	store := openMemoryEntryStore(t)
	entry := testMemoryEntry("memory.external.instruction", "approved_summary")
	entry.SourceType = "external"
	entry.SourceID = "doc"
	entry.Provenance = map[string]any{"source_type": "external", "source_id": "doc"}
	entry.ExternalContentIsNotInstruction = false
	if err := store.UpsertMemoryEntry(entry); err == nil || err.Error() != "memory_entry_external_content_instruction" {
		t.Fatalf("expected external instruction block, got %v", err)
	}
}

func TestBoundedConsolidationSchemaBehaviorRemainsUnchanged(t *testing.T) {
	store := openMemoryEntryStore(t)
	err := store.UpsertConsolidationRun(ConsolidationRun{
		RunID:       "bad_status",
		ArchiveDate: "2026-06-03",
		Trigger:     "scheduled",
		Status:      "completed_with_candidates",
	})
	if err == nil || err.Error() != "consolidation_run_invalid_status" {
		t.Fatalf("expected bounded consolidation status unchanged, got %v", err)
	}
}

func TestListMemoryEntriesForRetrievalPackOrdersDeterministically(t *testing.T) {
	store := openMemoryEntryStore(t)
	lower := testMemoryEntry("memory.lower", "durable_user_preference")
	lower.AuthorityRank = "rank_300_direct_user"
	lower.TrustTier = 3
	higher := testMemoryEntry("memory.higher", "runtime_observation")
	higher.SourceType = "verified_runtime"
	higher.AuthorityRank = "rank_200_runtime"
	higher.TrustTier = 1
	higher.Provenance = map[string]any{"source_type": "verified_runtime"}
	if err := store.UpsertMemoryEntry(lower); err != nil {
		t.Fatalf("upsert lower memory: %v", err)
	}
	if err := store.UpsertMemoryEntry(higher); err != nil {
		t.Fatalf("upsert higher memory: %v", err)
	}
	entries, err := store.ListMemoryEntriesForRetrievalPack()
	if err != nil {
		t.Fatalf("list memory entries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected two memory entries, got %#v", entries)
	}
	if entries[0].MemoryID != "memory.higher" || entries[1].MemoryID != "memory.lower" {
		t.Fatalf("entries not deterministically ordered: %#v", entries)
	}
}

func TestListMemoryEvidenceRefsForRetrievalPackRoundTripsDisplayFlags(t *testing.T) {
	store := openMemoryEntryStore(t)
	entry := testMemoryEntry("memory.evidence.list", "durable_user_preference")
	if err := store.UpsertMemoryEntry(entry); err != nil {
		t.Fatalf("upsert memory: %v", err)
	}
	refs := []MemoryEvidenceRef{
		{
			EvidenceID:       "evidence.display",
			MemoryID:         entry.MemoryID,
			SourceType:       "direct_user",
			SourceID:         "user",
			SourceRef:        "user visible preference",
			EvidenceKind:     "user_statement",
			TrustTier:        2,
			AuthorityRank:    "rank_300_direct_user",
			DisplayAllowed:   true,
			PromotionAllowed: true,
		},
		{
			EvidenceID:       "evidence.secret",
			MemoryID:         entry.MemoryID,
			SourceType:       "direct_user",
			SourceID:         "user",
			SourceRef:        "api_key=sk-user-provided-value",
			EvidenceKind:     "user_statement",
			TrustTier:        2,
			AuthorityRank:    "rank_300_direct_user",
			DisplayAllowed:   false,
			PromotionAllowed: false,
		},
	}
	for _, ref := range refs {
		if err := store.AppendMemoryEvidenceRef(ref); err != nil {
			t.Fatalf("append evidence %s: %v", ref.EvidenceID, err)
		}
	}
	loaded, err := store.ListMemoryEvidenceRefsForMemoryIDs([]string{entry.MemoryID, entry.MemoryID, ""})
	if err != nil {
		t.Fatalf("list evidence refs: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected two evidence refs, got %#v", loaded)
	}
	if !loaded[0].DisplayAllowed || !loaded[0].PromotionAllowed {
		t.Fatalf("display evidence flags drifted: %#v", loaded[0])
	}
	if loaded[1].DisplayAllowed || loaded[1].PromotionAllowed {
		t.Fatalf("secret evidence flags drifted: %#v", loaded[1])
	}
	if strings.Contains(loaded[1].SourceRef, "sk-user-provided-value") || strings.Contains(loaded[1].SourceRef, "api_key=") {
		t.Fatalf("secret evidence ref leaked raw value: %s", loaded[1].SourceRef)
	}
}

func testMemoryEntry(memoryID, memoryClass string) MemoryEntry {
	return MemoryEntry{
		MemoryID:                        memoryID,
		MemoryClass:                     memoryClass,
		Status:                          "active",
		Title:                           "safe title",
		Summary:                         "safe summary",
		Content:                         "safe content",
		ContentExcerpt:                  "safe content",
		SourceType:                      "direct_user",
		SourceID:                        "user",
		TrustTier:                       2,
		AuthorityRank:                   "rank_300_direct_user",
		Provenance:                      map[string]any{"source_type": "direct_user", "source_id": "user"},
		EvidenceRefs:                    []string{"evidence.user"},
		FreshnessState:                  "fresh",
		Confidence:                      0.8,
		ContradictionState:              "none",
		QuarantineStatus:                "clean",
		SuppressionStatus:               "active",
		AllowedActions:                  []string{"view"},
		RollbackAvailable:               true,
		ExternalContentIsNotInstruction: true,
		RetrievalEligible:               true,
		RetrievalReason:                 "safe preference",
		EmbeddingStatus:                 "not_needed",
		UserVisible:                     true,
	}
}

func testMemoryCandidate(candidateID string) MemoryCandidate {
	return MemoryCandidate{
		CandidateID:        candidateID,
		CandidateType:      "memory_candidate",
		Status:             "generated",
		Title:              "candidate",
		Summary:            "candidate summary",
		Content:            "candidate content",
		SourceType:         "model_output",
		SourceID:           "turn_1",
		TrustTier:          6,
		AuthorityRank:      "rank_700_model_output",
		Provenance:         map[string]any{"source_type": "model_output", "turn_id": "turn_1"},
		EvidenceRefs:       []string{"evidence.turn_1"},
		FreshnessState:     "fresh",
		Confidence:         0.5,
		ContradictionState: "none",
		QuarantineStatus:   "clean",
		SuppressionStatus:  "active",
	}
}

func openMemoryEntryStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func memoryTableCount(t *testing.T, store *Store, table string) int {
	t.Helper()
	var count int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
		t.Fatalf("count table %s: %v", table, err)
	}
	return count
}

func memoryQueryCount(t *testing.T, store *Store, query string) int {
	t.Helper()
	var count int
	if err := store.DB.QueryRow(query).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	return count
}
