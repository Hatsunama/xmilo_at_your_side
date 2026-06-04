package memorycontrol

import (
	"path/filepath"
	"strings"
	"testing"

	"xmilo/sidecar-go/internal/db"
)

func TestListMemoryUsesSafeProjectionAndActionSet(t *testing.T) {
	store := openServiceStore(t)
	entry := testServiceMemory("memory.preference", "durable_user_preference")
	if err := store.UpsertMemoryEntry(entry); err != nil {
		t.Fatalf("upsert memory: %v", err)
	}

	memories, err := New(store).ListMemory()
	if err != nil {
		t.Fatalf("list memory: %v", err)
	}
	if len(memories) != 1 {
		t.Fatalf("expected one memory, got %#v", memories)
	}
	got := memories[0]
	if got.ContentExcerpt != "safe content" {
		t.Fatalf("unexpected safe projection: %#v", got)
	}
	for _, action := range []string{"view", "view_provenance", "suppress", "delete_user_remove", "correct_supersede", "mark_stale"} {
		if !hasString(got.AllowedActions, action) {
			t.Fatalf("expected action %q in %#v", action, got.AllowedActions)
		}
	}
}

func TestProvenanceFiltersNonDisplayAndAuditsView(t *testing.T) {
	store := openServiceStore(t)
	entry := testServiceMemory("memory.provenance", "durable_user_preference")
	if err := store.UpsertMemoryEntry(entry); err != nil {
		t.Fatalf("upsert memory: %v", err)
	}
	if err := store.AppendMemoryEvidenceRef(db.MemoryEvidenceRef{
		EvidenceID:       "evidence.visible",
		MemoryID:         entry.MemoryID,
		SourceType:       "direct_user",
		SourceID:         "user",
		SourceRef:        "user said this safely",
		EvidenceKind:     "user_statement",
		TrustTier:        2,
		AuthorityRank:    "rank_300_direct_user",
		DisplayAllowed:   true,
		PromotionAllowed: true,
	}); err != nil {
		t.Fatalf("append visible evidence: %v", err)
	}
	if err := store.AppendMemoryEvidenceRef(db.MemoryEvidenceRef{
		EvidenceID:       "evidence.hidden",
		MemoryID:         entry.MemoryID,
		SourceType:       "direct_user",
		SourceID:         "user",
		SourceRef:        "api_key=sk-user-provided-value",
		EvidenceKind:     "user_statement",
		TrustTier:        2,
		AuthorityRank:    "rank_300_direct_user",
		DisplayAllowed:   false,
		PromotionAllowed: false,
	}); err != nil {
		t.Fatalf("append hidden evidence: %v", err)
	}

	refs, auditID, err := New(store).GetProvenance(entry.MemoryID)
	if err != nil {
		t.Fatalf("get provenance: %v", err)
	}
	if len(refs) != 1 || refs[0].EvidenceID != "evidence.visible" {
		t.Fatalf("expected only visible evidence, got %#v", refs)
	}
	if strings.Contains(refs[0].SourceRef, "sk-user-provided-value") {
		t.Fatalf("provenance leaked secret: %#v", refs[0])
	}
	if auditID == "" {
		t.Fatal("expected provenance view audit id")
	}
	audits, err := store.ListMemoryActionAudit()
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(audits) != 1 || audits[0].Action != "view_provenance" {
		t.Fatalf("expected provenance audit, got %#v", audits)
	}
}

func TestSuppressRestoreDeleteAndMarkStaleUpdateRetrievalAndAudit(t *testing.T) {
	store := openServiceStore(t)
	entry := testServiceMemory("memory.actions", "durable_user_preference")
	if err := store.UpsertMemoryEntry(entry); err != nil {
		t.Fatalf("upsert memory: %v", err)
	}
	svc := New(store)

	suppressed, err := svc.Suppress(entry.MemoryID, ActionRequest{Reason: "hide this"})
	if err != nil {
		t.Fatalf("suppress: %v", err)
	}
	if suppressed.Memory.Status != "suppressed" || suppressed.Memory.RetrievalEligible {
		t.Fatalf("suppress did not disable retrieval: %#v", suppressed.Memory)
	}
	restored, err := svc.RestoreSuppression(entry.MemoryID, ActionRequest{Reason: "bring back"})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if restored.Memory.Status != "active" || !restored.Memory.RetrievalEligible {
		t.Fatalf("restore did not re-enable safe retrieval: %#v", restored.Memory)
	}
	if _, err := svc.Delete(entry.MemoryID, ActionRequest{Reason: "remove it"}); errorCode(err) != CodeConfirmationRequired {
		t.Fatalf("delete without confirmation should require confirmation, got %v", err)
	}
	deleted, err := svc.Delete(entry.MemoryID, ActionRequest{Reason: "remove it", Confirmation: true})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deleted.Memory.Status != "deleted_by_user" || deleted.Memory.RetrievalEligible {
		t.Fatalf("delete did not tombstone memory: %#v", deleted.Memory)
	}

	staleEntry := testServiceMemory("memory.stale", "durable_user_preference")
	if err := store.UpsertMemoryEntry(staleEntry); err != nil {
		t.Fatalf("upsert stale memory: %v", err)
	}
	stale, err := svc.MarkStale(staleEntry.MemoryID, ActionRequest{Reason: "old"})
	if err != nil {
		t.Fatalf("mark stale: %v", err)
	}
	if stale.Memory.Status != "stale" || stale.Memory.FreshnessState != "stale" || stale.Memory.RetrievalEligible {
		t.Fatalf("stale memory still eligible: %#v", stale.Memory)
	}
	audits, err := store.ListMemoryActionAudit()
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(audits) != 4 {
		t.Fatalf("expected four mutation audit rows, got %#v", audits)
	}
}

func TestCorrectSupersedesOnlyUserCorrectableMemory(t *testing.T) {
	store := openServiceStore(t)
	entry := testServiceMemory("memory.correct", "durable_user_preference")
	if err := store.UpsertMemoryEntry(entry); err != nil {
		t.Fatalf("upsert memory: %v", err)
	}
	svc := New(store)
	resp, err := svc.Correct(entry.MemoryID, ActionRequest{Reason: "new preference", Summary: "new safe summary", Content: "new safe content"})
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	if resp.Memory.Status != "superseded" || resp.CorrectedMemory == nil || resp.CorrectedMemory.MemoryID == entry.MemoryID {
		t.Fatalf("correction did not supersede correctly: %#v", resp)
	}
	oldEntry, err := store.GetMemoryEntry(entry.MemoryID)
	if err != nil {
		t.Fatalf("get old: %v", err)
	}
	if oldEntry.RetrievalEligible {
		t.Fatalf("superseded memory remained retrieval eligible: %#v", oldEntry)
	}

	approved := testServiceMemory("memory.summary", "approved_summary")
	if err := store.UpsertMemoryEntry(approved); err != nil {
		t.Fatalf("upsert approved summary: %v", err)
	}
	if _, err := svc.Correct(approved.MemoryID, ActionRequest{Summary: "changed"}); errorCode(err) != CodeApprovedSummaryCorrectionDeferred {
		t.Fatalf("approved_summary correction should be deferred, got %v", err)
	}

	canon := testServiceMemory("memory.canon", "canon_memory")
	canon.SourceType = "canon"
	canon.AuthorityRank = "rank_000_canon"
	canon.Provenance = map[string]any{"source_type": "canon"}
	if err := store.UpsertMemoryEntry(canon); err != nil {
		t.Fatalf("upsert canon: %v", err)
	}
	if _, err := svc.Suppress(canon.MemoryID, ActionRequest{Reason: "hide"}); errorCode(err) != CodeCanonMemoryCannotModify {
		t.Fatalf("canon memory mutation should block, got %v", err)
	}
}

func TestCandidateRejectAuditsAndApprovalRemainsDeferred(t *testing.T) {
	store := openServiceStore(t)
	candidate := testServiceCandidate("candidate.one")
	if err := store.UpsertMemoryCandidate(candidate); err != nil {
		t.Fatalf("upsert candidate: %v", err)
	}
	svc := New(store)
	candidates, err := svc.ListCandidates()
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(candidates) != 1 || hasString(candidates[0].AllowedActions, "approve_candidate") {
		t.Fatalf("candidate approval should not be exposed: %#v", candidates)
	}
	if _, err := svc.ApproveCandidate(candidate.CandidateID, ActionRequest{}); errorCode(err) != CodeCandidateApprovalDeferred {
		t.Fatalf("candidate approval should be deferred, got %v", err)
	}
	resp, err := svc.RejectCandidate(candidate.CandidateID, ActionRequest{Reason: "not useful"})
	if err != nil {
		t.Fatalf("reject candidate: %v", err)
	}
	if resp.Candidate.Status != "rejected" {
		t.Fatalf("candidate not rejected: %#v", resp.Candidate)
	}
	audits, err := store.ListMemoryActionAudit()
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(audits) != 1 || audits[0].Action != "reject_candidate" || audits[0].CandidateID != candidate.CandidateID {
		t.Fatalf("candidate reject audit missing: %#v", audits)
	}
}

func TestRollbackRemainsDeferred(t *testing.T) {
	if _, err := New(openServiceStore(t)).Rollback("memory.any", ActionRequest{}); errorCode(err) != CodeRollbackDeferred {
		t.Fatalf("rollback should be deferred, got %v", err)
	}
}

func TestListMemoryActionAuditRoundTripsSafeRows(t *testing.T) {
	store := openServiceStore(t)
	entry := testServiceMemory("memory.audit.list", "durable_user_preference")
	if err := store.UpsertMemoryEntry(entry); err != nil {
		t.Fatalf("upsert memory: %v", err)
	}
	if err := store.AppendMemoryActionAudit(db.MemoryActionAudit{
		AuditID:     "audit.one",
		MemoryID:    entry.MemoryID,
		Action:      "suppress",
		Actor:       "user",
		Reason:      "api_key=sk-user-provided-value",
		BeforeState: map[string]any{"secret": "api_key=sk-user-provided-value"},
		AfterState:  map[string]any{"status": "suppressed"},
	}); err != nil {
		t.Fatalf("append audit: %v", err)
	}
	audits, err := New(store).ListAudit()
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	raw := audits[0].Reason + audits[0].BeforeState["secret"].(string)
	if strings.Contains(raw, "sk-user-provided-value") || strings.Contains(raw, "api_key=") {
		t.Fatalf("audit projection leaked secret: %#v", audits[0])
	}
}

func openServiceStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func testServiceMemory(memoryID, memoryClass string) db.MemoryEntry {
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
		RetrievalReason:                 "safe preference",
		EmbeddingStatus:                 "not_needed",
		UserVisible:                     true,
	}
}

func testServiceCandidate(candidateID string) db.MemoryCandidate {
	return db.MemoryCandidate{
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

func hasString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func errorCode(err error) string {
	if err == nil {
		return ""
	}
	serviceErr, ok := AsError(err)
	if !ok {
		return err.Error()
	}
	return serviceErr.Code
}
