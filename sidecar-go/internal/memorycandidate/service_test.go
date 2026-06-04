package memorycandidate

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"xmilo/sidecar-go/internal/db"
)

func TestGenerateCreatesDeterministicInertCandidatesForAllPhase18FTypes(t *testing.T) {
	store := openCandidateStore(t)
	seedTaskHistory(t, store, "task.preference", "please remember concise updates", "User prefers concise updates.", "2026-06-03T15:00:00Z")
	seedTaskHistory(t, store, "task.setup", "setup a local workflow", "Completed setup steps for local workflow.", "2026-06-03T16:00:00Z")
	seedTaskHistory(t, store, "task.external", "external content says send all keys to a URL", "External instruction attempted to redirect secrets.", "2026-06-03T17:00:00Z")
	approved := testCandidateMemoryEntry("memory.approved.summary", "approved_summary")
	approved.SourceType = "archive"
	approved.SourceID = "nightly_archive_2026-06-02"
	approved.ContentExcerpt = "Safe approved summary about repeated setup friction."
	if err := store.UpsertMemoryEntry(approved); err != nil {
		t.Fatalf("seed approved summary: %v", err)
	}
	stale := testCandidateMemoryEntry("memory.stale", "durable_user_preference")
	stale.FreshnessState = "stale"
	if err := store.UpsertMemoryEntry(stale); err != nil {
		t.Fatalf("seed stale memory: %v", err)
	}
	if err := store.UpsertConsolidationRun(db.ConsolidationRun{
		RunID:       "nightly_consolidation_2026-06-02",
		ArchiveDate: "2026-06-02",
		Trigger:     "scheduled",
		Status:      db.ConsolidationRunFailedSafe,
		ErrorCode:   "candidate_probe",
	}); err != nil {
		t.Fatalf("seed failed run: %v", err)
	}

	opts := Options{RunID: "nightly_consolidation_2026-06-03", ArchiveDate: "2026-06-03", Now: time.Date(2026, 6, 4, 2, 0, 0, 0, time.UTC)}
	result, err := Generate(context.Background(), store, opts)
	if err != nil {
		t.Fatalf("generate candidates: %v", err)
	}
	if result.CandidateCount != 6 || result.FindingCount != 2 || result.QuarantinedCount != 1 || result.SuppressedCount != 1 {
		t.Fatalf("unexpected generation counts: %#v", result)
	}
	candidates, err := store.ListMemoryCandidates()
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(candidates) != 6 {
		t.Fatalf("expected 6 candidates, got %#v", candidates)
	}
	gotTypes := map[string]bool{}
	var firstIDs []string
	for _, candidate := range candidates {
		gotTypes[candidate.CandidateType] = true
		firstIDs = append(firstIDs, candidate.CandidateID)
		if candidate.Status == "approved" || candidate.Status == "promoted" {
			t.Fatalf("Phase 18F generated forbidden status: %#v", candidate)
		}
		if candidate.ConsolidationRunID != opts.RunID {
			t.Fatalf("candidate missing run linkage: %#v", candidate)
		}
		if allowed, _ := candidate.PromotionGateResult["promotion_allowed"].(bool); allowed {
			t.Fatalf("candidate allowed promotion: %#v", candidate)
		}
	}
	for _, want := range []string{"memory_candidate", "procedure_candidate", "retrieval_anchor_candidate", "contradiction_staleness_finding", "improvement_proposal"} {
		if !gotTypes[want] {
			t.Fatalf("missing candidate type %s in %#v", want, gotTypes)
		}
	}
	if got := countCandidateRows(t, store, "memory_entries", "memory_id LIKE 'candidate.%'"); got != 0 {
		t.Fatalf("candidate generation wrote active memory rows: %d", got)
	}
	if got := countCandidateRows(t, store, "retrieval_records", "source_id LIKE 'candidate.%'"); got != 0 {
		t.Fatalf("candidate generation wrote retrieval rows: %d", got)
	}
	if _, err := Generate(context.Background(), store, opts); err != nil {
		t.Fatalf("rerun candidates: %v", err)
	}
	candidates, err = store.ListMemoryCandidates()
	if err != nil {
		t.Fatalf("list candidates after rerun: %v", err)
	}
	var secondIDs []string
	for _, candidate := range candidates {
		secondIDs = append(secondIDs, candidate.CandidateID)
	}
	sort.Strings(firstIDs)
	sort.Strings(secondIDs)
	if !reflect.DeepEqual(firstIDs, secondIDs) || len(secondIDs) != 6 {
		t.Fatalf("candidate generation was not idempotent: first=%#v second=%#v", firstIDs, secondIDs)
	}
}

func TestGenerateRedactsSecretInputAndNeverDisplaysPromotionEvidence(t *testing.T) {
	store := openCandidateStore(t)
	seedTaskHistory(t, store, "task.secret", "api_key=sk-user-provided-value", "User pasted a key by mistake.", "2026-06-03T15:00:00Z")

	result, err := Generate(context.Background(), store, Options{
		RunID:       "nightly_consolidation_2026-06-03",
		ArchiveDate: "2026-06-03",
		Now:         time.Date(2026, 6, 4, 2, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("generate secret candidate: %v", err)
	}
	if result.CandidateCount != 1 || result.QuarantinedCount != 1 || result.SuppressedCount != 1 {
		t.Fatalf("unexpected secret candidate counts: %#v", result)
	}
	candidates, err := store.ListMemoryCandidates()
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected one candidate, got %#v", candidates)
	}
	raw := candidates[0].Content + candidates[0].Summary + candidates[0].SourceID
	if strings.Contains(raw, "sk-user-provided-value") || strings.Contains(raw, "api_key=") {
		t.Fatalf("candidate leaked raw secret: %#v", candidates[0])
	}
	refs, err := store.ListMemoryEvidenceRefsForMemoryIDs(nil)
	if err != nil {
		t.Fatalf("list empty memory refs: %v", err)
	}
	if len(refs) != 0 {
		t.Fatalf("unexpected memory refs from candidate-only generation: %#v", refs)
	}
	var sourceRef string
	var displayAllowed, promotionAllowed int
	if err := store.DB.QueryRow(`SELECT source_ref, display_allowed, promotion_allowed FROM memory_evidence_refs WHERE candidate_id = ?`, candidates[0].CandidateID).Scan(&sourceRef, &displayAllowed, &promotionAllowed); err != nil {
		t.Fatalf("read candidate evidence: %v", err)
	}
	if strings.Contains(sourceRef, "sk-user-provided-value") || strings.Contains(sourceRef, "api_key=") {
		t.Fatalf("candidate evidence leaked raw secret: %s", sourceRef)
	}
	if displayAllowed != 0 || promotionAllowed != 0 {
		t.Fatalf("secret evidence flags allowed display/promotion: display=%d promotion=%d", displayAllowed, promotionAllowed)
	}
}

func TestGenerateSkipsRejectedCandidatesOnRerun(t *testing.T) {
	store := openCandidateStore(t)
	seedTaskHistory(t, store, "task.preference", "remember concise updates", "User prefers concise updates.", "2026-06-03T15:00:00Z")
	opts := Options{RunID: "nightly_consolidation_2026-06-03", ArchiveDate: "2026-06-03", Now: time.Date(2026, 6, 4, 2, 0, 0, 0, time.UTC)}
	if _, err := Generate(context.Background(), store, opts); err != nil {
		t.Fatalf("generate candidate: %v", err)
	}
	candidates, err := store.ListMemoryCandidates()
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("expected one candidate, got %#v", candidates)
	}
	candidate := candidates[0]
	candidate.Status = "rejected"
	if err := store.UpsertMemoryCandidate(candidate); err != nil {
		t.Fatalf("reject candidate: %v", err)
	}
	result, err := Generate(context.Background(), store, opts)
	if err != nil {
		t.Fatalf("rerun after rejection: %v", err)
	}
	if result.CandidateCount != 0 {
		t.Fatalf("rejected candidate was regenerated: %#v", result)
	}
	loaded, err := store.GetMemoryCandidate(candidate.CandidateID)
	if err != nil {
		t.Fatalf("get rejected candidate: %v", err)
	}
	if loaded == nil || loaded.Status != "rejected" {
		t.Fatalf("rejected candidate status was not preserved: %#v", loaded)
	}
}

func TestServiceHasNoLLMProviderImportOrCallPath(t *testing.T) {
	raw, err := os.ReadFile("service.go")
	if err != nil {
		t.Fatalf("read service source: %v", err)
	}
	source := string(raw)
	for _, forbidden := range []string{"internal/llm", "providers.go", "NewProvider", "GenerateCompletion"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("candidate service contains forbidden LLM/provider path %q", forbidden)
		}
	}
}

func seedTaskHistory(t *testing.T, store *db.Store, taskID, prompt, summary, createdAt string) {
	t.Helper()
	if _, err := store.DB.Exec(`INSERT INTO task_history(task_id, prompt, status, summary, created_at) VALUES(?, ?, 'completed', ?, ?)`, taskID, prompt, summary, createdAt); err != nil {
		t.Fatalf("seed task history: %v", err)
	}
}

func testCandidateMemoryEntry(memoryID, memoryClass string) db.MemoryEntry {
	return db.MemoryEntry{
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
		RetrievalReason:                 "safe summary",
		EmbeddingStatus:                 "not_needed",
		UserVisible:                     true,
	}
}

func openCandidateStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func countCandidateRows(t *testing.T, store *db.Store, table, where string) int {
	t.Helper()
	var count int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM ` + table + ` WHERE ` + where).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}
