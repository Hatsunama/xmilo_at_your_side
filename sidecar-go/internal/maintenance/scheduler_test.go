package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xmilo/sidecar-go/internal/db"
	"xmilo/sidecar-go/internal/memorycandidate"
	"xmilo/sidecar-go/internal/memorycontrol"
	"xmilo/sidecar-go/internal/retrieval"
	"xmilo/sidecar-go/internal/runtime"
	"xmilo/sidecar-go/internal/ws"
)

func TestPhase18HLoopNightlyCandidateAPIAndRetrievalRemainInert(t *testing.T) {
	store := openMaintenanceStore(t)
	archiveDate := "2026-06-03"
	nightlyTime := time.Date(2026, 6, 4, 3, 0, 0, 0, time.Local)
	seedMaintenanceTaskHistory(t, store, "task.preference", "remember concise updates", "User prefers concise updates.", "2026-06-03T15:00:00Z")
	seedMaintenanceTaskHistory(t, store, "task.setup", "setup a local workflow", "Completed setup steps for local workflow.", "2026-06-03T16:00:00Z")
	seedMaintenanceTaskHistory(t, store, "task.secret", "api_key=sk-user-provided-value", "User pasted a key by mistake.", "2026-06-03T17:00:00Z")
	approved := maintenanceMemoryEntry("memory.approved.summary", "approved_summary")
	approved.SourceType = "archive"
	approved.SourceID = "nightly_archive_2026-06-02"
	approved.ContentExcerpt = "Safe approved setup summary."
	approved.Content = approved.ContentExcerpt
	approved.Summary = approved.ContentExcerpt
	if err := store.UpsertMemoryEntry(approved); err != nil {
		t.Fatalf("seed approved summary: %v", err)
	}
	stale := maintenanceMemoryEntry("memory.stale.preference", "durable_user_preference")
	stale.FreshnessState = "stale"
	if err := store.UpsertMemoryEntry(stale); err != nil {
		t.Fatalf("seed stale memory: %v", err)
	}
	if err := store.UpsertConsolidationRun(db.ConsolidationRun{
		RunID:        "nightly_consolidation_2026-06-02",
		ArchiveDate:  "2026-06-02",
		Trigger:      "scheduled",
		Status:       db.ConsolidationRunFailedSafe,
		ErrorCode:    "candidate_probe",
		ErrorSummary: "prior candidate path failed safely",
	}); err != nil {
		t.Fatalf("seed failed run: %v", err)
	}
	for key, value := range map[string]string{
		"phase18h_runtime_probe":  "runtime-state",
		"phase18h_canon_probe":    "canon-state",
		"phase18h_policy_probe":   "policy-state",
		"phase18h_skill_probe":    "skill-state",
		"phase18h_provider_probe": "provider-state",
	} {
		if err := store.SetRuntimeConfig(key, value); err != nil {
			t.Fatalf("seed runtime config %s: %v", key, err)
		}
	}
	beforeMemoryRows := maintenanceQueryCount(t, store, `SELECT COUNT(*) FROM memory_entries`)
	beforeRetrievalRows := maintenanceQueryCount(t, store, `SELECT COUNT(*) FROM retrieval_records`)
	scheduler := testScheduler(store)
	scheduler.releaseCheckFn = func(context.Context) releaseCheck {
		return releaseCheck{
			Status:  "ok",
			TagName: "v-update-should-not-learn",
			URL:     "https://updates.example.test/releases/v-update-should-not-learn",
		}
	}

	scheduler.runNightly(context.Background(), archiveDate, nightlyTime, "scheduled")

	run := requireConsolidationRun(t, store, consolidationRunID(archiveDate))
	if run.Status != db.ConsolidationRunCompletedSummary || run.CandidateCount != 6 || run.QuarantinedCount != 1 || run.SuppressedCount != 1 {
		t.Fatalf("expected truthful completed_summary_only candidate ledger counts, got %#v", run)
	}
	candidates, err := memorycontrol.New(store).ListCandidates()
	if err != nil {
		t.Fatalf("list candidate projection: %v", err)
	}
	if len(candidates) != 6 {
		t.Fatalf("expected six projected candidates, got %#v", candidates)
	}
	gotTypes := map[string]bool{}
	for _, candidate := range candidates {
		gotTypes[candidate.CandidateType] = true
		if !reflectOnlyReject(candidate.AllowedActions) {
			t.Fatalf("candidate projection exposed non-reject actions: %#v", candidate)
		}
		rawProjection := candidate.Title + candidate.Summary + candidate.SourceID + strings.Join(candidate.Warnings, " ") + strings.Join(candidate.AllowedActions, " ")
		assertNoMaintenanceSecret(t, rawProjection, "candidate projection")
		for _, forbidden := range []string{"approve_candidate", "promote_candidate", "rollback", "v-update-should-not-learn", "updates.example.test", "update_check_status"} {
			if strings.Contains(rawProjection, forbidden) {
				t.Fatalf("candidate projection leaked forbidden marker %q: %#v", forbidden, candidate)
			}
		}
	}
	for _, want := range []string{"memory_candidate", "procedure_candidate", "retrieval_anchor_candidate", "contradiction_staleness_finding", "improvement_proposal"} {
		if !gotTypes[want] {
			t.Fatalf("missing candidate type %s in %#v", want, gotTypes)
		}
	}
	storedCandidates, err := store.ListMemoryCandidates()
	if err != nil {
		t.Fatalf("list stored candidates: %v", err)
	}
	for _, candidate := range storedCandidates {
		raw := candidate.Title + candidate.Summary + candidate.Content + candidate.SourceID
		for _, value := range candidate.Provenance {
			if s, ok := value.(string); ok {
				raw += s
			}
		}
		for _, value := range candidate.PromotionGateResult {
			if s, ok := value.(string); ok {
				raw += s
			}
		}
		assertNoMaintenanceSecret(t, raw, "stored candidate")
		if strings.Contains(raw, "v-update-should-not-learn") || strings.Contains(raw, "updates.example.test") || strings.Contains(raw, "update_check_status") {
			t.Fatalf("GitHub update-check data became candidate content/provenance: %#v", candidate)
		}
		if allowed, _ := candidate.PromotionGateResult["promotion_allowed"].(bool); allowed {
			t.Fatalf("candidate allowed promotion: %#v", candidate)
		}
		if allowed, _ := candidate.PromotionGateResult["approval_allowed"].(bool); allowed {
			t.Fatalf("candidate allowed approval: %#v", candidate)
		}
		if effect, _ := candidate.PromotionGateResult["retrieval_effect"].(string); effect != "none" {
			t.Fatalf("candidate changed retrieval effect: %#v", candidate.PromotionGateResult)
		}
		if used, _ := candidate.Provenance["llm_reflection_used"].(bool); used {
			t.Fatalf("candidate used LLM reflection: %#v", candidate)
		}
	}
	var rawEvidence string
	if err := store.DB.QueryRow(`SELECT COALESCE(group_concat(source_ref, ' '), '') FROM memory_evidence_refs WHERE candidate_id <> ''`).Scan(&rawEvidence); err != nil {
		t.Fatalf("read candidate evidence refs: %v", err)
	}
	assertNoMaintenanceSecret(t, rawEvidence, "candidate evidence")
	if strings.Contains(rawEvidence, "v-update-should-not-learn") || strings.Contains(rawEvidence, "updates.example.test") {
		t.Fatalf("GitHub update-check data became candidate evidence: %s", rawEvidence)
	}
	if got := maintenanceQueryCount(t, store, `SELECT COUNT(*) FROM memory_entries`); got != beforeMemoryRows {
		t.Fatalf("candidate generation created active memory rows: before=%d after=%d", beforeMemoryRows, got)
	}
	if got := maintenanceQueryCount(t, store, `SELECT COUNT(*) FROM retrieval_records`); got != beforeRetrievalRows {
		t.Fatalf("candidate generation created retrieval rows: before=%d after=%d", beforeRetrievalRows, got)
	}
	pack, err := retrieval.BuildTypedMemoryRetrievalPack(store, retrieval.TypedMemoryPackInput{
		QueryIntent: "Use approved memory only.",
		Now:         nightlyTime.UTC(),
	})
	if err != nil {
		t.Fatalf("build typed memory pack: %v", err)
	}
	for _, item := range pack.MemoryItems {
		if strings.HasPrefix(item.MemoryID, "candidate.") || strings.Contains(item.Text, "Candidate generated") || strings.Contains(item.Text, "Review inert candidate") {
			t.Fatalf("candidate entered typed retrieval pack: %#v", item)
		}
	}
	resp, err := memorycontrol.New(store).RejectCandidate(candidates[0].CandidateID, memorycontrol.ActionRequest{Reason: "not useful"})
	if err != nil {
		t.Fatalf("reject candidate: %v", err)
	}
	if !resp.OK || resp.AuditID == "" || resp.Candidate == nil || resp.Candidate.Status != "rejected" {
		t.Fatalf("candidate reject response missing audit/status proof: %#v", resp)
	}
	if _, err := memorycontrol.New(store).ApproveCandidate(candidates[0].CandidateID, memorycontrol.ActionRequest{}); memoryControlErrorCode(err) != memorycontrol.CodeCandidateApprovalDeferred {
		t.Fatalf("candidate approval should remain deferred, got %v", err)
	}
	if got := maintenanceQueryCount(t, store, `SELECT COUNT(*) FROM memory_action_audit WHERE action = 'reject_candidate' AND candidate_id <> ''`); got != 1 {
		t.Fatalf("candidate reject audit missing: %d", got)
	}
	if got := maintenanceQueryCount(t, store, `SELECT COUNT(*) FROM memory_entries`); got != beforeMemoryRows {
		t.Fatalf("candidate rejection promoted or created memory rows: before=%d after=%d", beforeMemoryRows, got)
	}
	if got := maintenanceQueryCount(t, store, `SELECT COUNT(*) FROM retrieval_records`); got != beforeRetrievalRows {
		t.Fatalf("candidate rejection created retrieval rows: before=%d after=%d", beforeRetrievalRows, got)
	}
	for key, want := range map[string]string{
		"phase18h_runtime_probe":  "runtime-state",
		"phase18h_canon_probe":    "canon-state",
		"phase18h_policy_probe":   "policy-state",
		"phase18h_skill_probe":    "skill-state",
		"phase18h_provider_probe": "provider-state",
	} {
		got, err := store.GetRuntimeConfig(key)
		if err != nil {
			t.Fatalf("get runtime config %s: %v", key, err)
		}
		if got != want {
			t.Fatalf("runtime/canon/policy/skill/provider probe mutated for %s: got %q want %q", key, got, want)
		}
	}
	assertMaintenancePendingEventsSafe(t, store)
}

func TestSchedulerDefersNightlyWhenActiveTaskExists(t *testing.T) {
	store := openMaintenanceStore(t)
	now := time.Date(2026, 6, 4, 3, 0, 0, 0, time.Local)
	active := runtime.TaskSnapshot{
		TaskID:    "task_active",
		AttemptID: "attempt_active",
		Prompt:    "finish active task",
		Status:    "running",
		StartedAt: now.Add(-time.Hour).UTC().Format(time.RFC3339),
		UpdatedAt: now.Add(-time.Minute).UTC().Format(time.RFC3339),
	}
	if err := store.UpsertTask("active", active); err != nil {
		t.Fatalf("seed active task: %v", err)
	}
	scheduler := testScheduler(store)

	scheduler.tick(context.Background(), now)

	archiveDate := now.AddDate(0, 0, -1).Format("2006-01-02")
	run := requireConsolidationRun(t, store, consolidationRunID(archiveDate))
	if run.Status != db.ConsolidationRunDeferredActiveTask || run.ActiveTaskID != active.TaskID || run.DeferredReason != "active_task" {
		t.Fatalf("expected active-task deferral ledger, got %#v", run)
	}
	assertMaintenanceEventCount(t, store, "maintenance.nightly_deferred", 1)
	assertMaintenanceEventCount(t, store, "archive.record_created", 0)
	assertMaintenanceEventCount(t, store, "maintenance.nightly_completed", 0)
	if got := maintenanceQueryCount(t, store, `SELECT COUNT(*) FROM memory_candidates`); got != 0 {
		t.Fatalf("active-task deferral generated candidates: %d", got)
	}
	candidates, err := memorycontrol.New(store).ListCandidates()
	if err != nil {
		t.Fatalf("list candidates after deferral: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("active-task deferral changed candidate projection: %#v", candidates)
	}
	loaded, err := store.GetTask("active")
	if err != nil {
		t.Fatalf("get active task: %v", err)
	}
	if loaded == nil || loaded.TaskID != active.TaskID || loaded.Status != active.Status {
		t.Fatalf("active task mutated or missing: %#v", loaded)
	}
}

func TestSchedulerNightlyCreatesSummaryOnlyLedger(t *testing.T) {
	store := openMaintenanceStore(t)
	scheduler := testScheduler(store)
	archiveDate := "2026-06-03"

	scheduler.runNightly(context.Background(), archiveDate, time.Date(2026, 6, 4, 3, 0, 0, 0, time.Local), "scheduled")

	run := requireConsolidationRun(t, store, consolidationRunID(archiveDate))
	if run.Status != db.ConsolidationRunCompletedSummary {
		t.Fatalf("expected completed summary-only run, got %#v", run)
	}
	if run.ArchiveRecordID != "nightly_archive_"+archiveDate || run.SummaryRecordCount != 1 {
		t.Fatalf("expected one archive summary record, got %#v", run)
	}
	if run.CandidateCount != 0 || run.QuarantinedCount != 0 || run.SuppressedCount != 0 {
		t.Fatalf("summary-only path created candidate/quarantine/suppression counts: %#v", run)
	}
	assertMaintenanceEventCount(t, store, "maintenance.nightly_started", 1)
	assertMaintenanceEventCount(t, store, "archive.record_created", 1)
	assertMaintenanceEventCount(t, store, "maintenance.nightly_completed", 1)
}

func TestSchedulerNightlyGeneratesInertCandidatesAfterActiveTaskDeferralPasses(t *testing.T) {
	store := openMaintenanceStore(t)
	archiveDate := "2026-06-03"
	if _, err := store.DB.Exec(`INSERT INTO task_history(task_id, prompt, status, summary, created_at) VALUES(?, ?, 'completed', ?, ?)`,
		"task.preference", "remember quiet summaries", "User prefers quiet summaries.", "2026-06-03T15:00:00Z"); err != nil {
		t.Fatalf("seed task history: %v", err)
	}
	scheduler := testScheduler(store)

	scheduler.runNightly(context.Background(), archiveDate, time.Date(2026, 6, 4, 3, 0, 0, 0, time.Local), "scheduled")

	run := requireConsolidationRun(t, store, consolidationRunID(archiveDate))
	if run.Status != db.ConsolidationRunCompletedSummary || run.CandidateCount != 1 || run.QuarantinedCount != 0 || run.SuppressedCount != 0 {
		t.Fatalf("expected completed run with one inert candidate, got %#v", run)
	}
	if got := maintenanceQueryCount(t, store, `SELECT COUNT(*) FROM memory_candidates WHERE consolidation_run_id = 'nightly_consolidation_2026-06-03'`); got != 1 {
		t.Fatalf("expected one memory candidate, got %d", got)
	}
	if got := maintenanceQueryCount(t, store, `SELECT COUNT(*) FROM memory_entries WHERE memory_id LIKE 'candidate.%'`); got != 0 {
		t.Fatalf("candidate generation wrote active memory: %d", got)
	}
	if got := maintenanceQueryCount(t, store, `SELECT COUNT(*) FROM retrieval_records WHERE source_id LIKE 'candidate.%'`); got != 0 {
		t.Fatalf("candidate generation wrote retrieval rows: %d", got)
	}
}

func TestSchedulerDoesNotDeleteOrMutateActiveTaskOrRuntimeTruth(t *testing.T) {
	store := openMaintenanceStore(t)
	now := time.Date(2026, 6, 4, 3, 0, 0, 0, time.Local)
	task := runtime.TaskSnapshot{
		TaskID:    "task_running",
		AttemptID: "attempt_running",
		Prompt:    "current work",
		Status:    "running",
		StartedAt: now.Add(-time.Hour).UTC().Format(time.RFC3339),
		UpdatedAt: now.Add(-time.Minute).UTC().Format(time.RFC3339),
	}
	if err := store.UpsertTask("active", task); err != nil {
		t.Fatalf("seed active task: %v", err)
	}
	if err := store.SetRuntimeConfig("runtime_truth_probe", "verified_runtime_state"); err != nil {
		t.Fatalf("seed runtime config: %v", err)
	}
	scheduler := testScheduler(store)

	scheduler.tick(context.Background(), now)

	loaded, err := store.GetTask("active")
	if err != nil {
		t.Fatalf("get active task: %v", err)
	}
	if loaded == nil || loaded.TaskID != task.TaskID || loaded.AttemptID != task.AttemptID || loaded.Status != task.Status {
		t.Fatalf("active task mutated: %#v", loaded)
	}
	value, err := store.GetRuntimeConfig("runtime_truth_probe")
	if err != nil {
		t.Fatalf("get runtime config: %v", err)
	}
	if value != "verified_runtime_state" {
		t.Fatalf("runtime truth config mutated: %q", value)
	}
	assertMaintenanceEventCount(t, store, "archive.record_created", 0)
}

func TestSchedulerFailureRecordsFailedSafeWithoutFakeCompletion(t *testing.T) {
	store := openMaintenanceStore(t)
	if _, err := store.DB.Exec(`DROP TABLE task_history`); err != nil {
		t.Fatalf("drop task history: %v", err)
	}
	scheduler := testScheduler(store)
	archiveDate := "2026-06-03"

	scheduler.runNightly(context.Background(), archiveDate, time.Date(2026, 6, 4, 3, 0, 0, 0, time.Local), "scheduled")

	run := requireConsolidationRun(t, store, consolidationRunID(archiveDate))
	if run.Status != db.ConsolidationRunFailedSafe || run.ErrorCode != "archive_record_failed" {
		t.Fatalf("expected failed_safe archive failure, got %#v", run)
	}
	if run.SummaryRecordCount != 0 || run.CandidateCount != 0 || run.ArchiveRecordID != "" {
		t.Fatalf("failure path must not record completed archive truth: %#v", run)
	}
	assertMaintenanceEventCount(t, store, "maintenance.nightly_started", 1)
	assertMaintenanceEventCount(t, store, "archive.record_created", 0)
	assertMaintenanceEventCount(t, store, "maintenance.nightly_completed", 0)
	assertMaintenanceEventCount(t, store, "runtime.error", 1)
}

func TestSchedulerCandidateGenerationFailureRecordsFailedSafeWithoutFakeCompletion(t *testing.T) {
	store := openMaintenanceStore(t)
	scheduler := testScheduler(store)
	scheduler.candidateFn = func(context.Context, *db.Store, memorycandidate.Options) (memorycandidate.Result, error) {
		return memorycandidate.Result{}, errors.New("candidate generator unavailable")
	}
	archiveDate := "2026-06-03"

	scheduler.runNightly(context.Background(), archiveDate, time.Date(2026, 6, 4, 3, 0, 0, 0, time.Local), "scheduled")

	run := requireConsolidationRun(t, store, consolidationRunID(archiveDate))
	if run.Status != db.ConsolidationRunFailedSafe || run.ErrorCode != "candidate_generation_failed" {
		t.Fatalf("expected failed_safe candidate generation failure, got %#v", run)
	}
	if run.CandidateCount != 0 || run.QuarantinedCount != 0 || run.SuppressedCount != 0 {
		t.Fatalf("failed candidate generation recorded candidate counts: %#v", run)
	}
	candidates, err := memorycontrol.New(store).ListCandidates()
	if err != nil {
		t.Fatalf("list candidates after failed generation: %v", err)
	}
	if len(candidates) != 0 {
		t.Fatalf("failed candidate generation exposed candidate projection: %#v", candidates)
	}
	assertMaintenanceEventCount(t, store, "maintenance.nightly_completed", 0)
	assertMaintenanceEventCount(t, store, "runtime.error", 1)
}

func testScheduler(store *db.Store) *Scheduler {
	return &Scheduler{
		store: store,
		hub:   ws.NewHub(),
		releaseCheckFn: func(context.Context) releaseCheck {
			return releaseCheck{Status: "ok", TagName: "v-test", URL: "https://example.test/release"}
		},
	}
}

func maintenanceQueryCount(t *testing.T, store *db.Store, query string) int {
	t.Helper()
	var count int
	if err := store.DB.QueryRow(query).Scan(&count); err != nil {
		t.Fatalf("count query: %v", err)
	}
	return count
}

func seedMaintenanceTaskHistory(t *testing.T, store *db.Store, taskID, prompt, summary, createdAt string) {
	t.Helper()
	if _, err := store.DB.Exec(`INSERT INTO task_history(task_id, prompt, status, summary, created_at) VALUES(?, ?, 'completed', ?, ?)`, taskID, prompt, summary, createdAt); err != nil {
		t.Fatalf("seed task history: %v", err)
	}
}

func maintenanceMemoryEntry(memoryID, memoryClass string) db.MemoryEntry {
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
		ExternalContentIsNotInstruction: true,
		RetrievalEligible:               true,
		RetrievalReason:                 "safe memory",
		EmbeddingStatus:                 "not_needed",
		UserVisible:                     true,
	}
}

func reflectOnlyReject(actions []string) bool {
	if len(actions) != 1 {
		return false
	}
	return actions[0] == "reject_candidate"
}

func memoryControlErrorCode(err error) string {
	serviceErr, ok := memorycontrol.AsError(err)
	if !ok {
		if err == nil {
			return ""
		}
		return err.Error()
	}
	return serviceErr.Code
}

func assertNoMaintenanceSecret(t *testing.T, raw, label string) {
	t.Helper()
	for _, forbidden := range []string{"sk-user-provided-value", "api_key=sk-user-provided-value", "api_key=", "Authorization: Bearer", "user-provided-token"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("%s leaked secret marker %q: %s", label, forbidden, raw)
		}
	}
}

func assertMaintenancePendingEventsSafe(t *testing.T, store *db.Store) {
	t.Helper()
	var raw string
	if err := store.DB.QueryRow(`SELECT COALESCE(group_concat(payload_json, ' '), '') FROM pending_events`).Scan(&raw); err != nil {
		t.Fatalf("read pending events: %v", err)
	}
	assertNoMaintenanceSecret(t, raw, "pending events")
}

func openMaintenanceStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func requireConsolidationRun(t *testing.T, store *db.Store, runID string) *db.ConsolidationRun {
	t.Helper()
	run, err := store.GetConsolidationRun(runID)
	if err != nil {
		t.Fatalf("get consolidation run: %v", err)
	}
	if run == nil {
		t.Fatalf("expected consolidation run %q", runID)
	}
	return run
}

func assertMaintenanceEventCount(t *testing.T, store *db.Store, eventType string, want int) {
	t.Helper()
	var count int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM pending_events WHERE event_type = ?`, eventType).Scan(&count); err != nil {
		t.Fatalf("count pending events: %v", err)
	}
	if count != want {
		t.Fatalf("expected %d %s events, got %d; latest=%#v", want, eventType, count, latestMaintenanceEventPayload(t, store, eventType))
	}
}

func latestMaintenanceEventPayload(t *testing.T, store *db.Store, eventType string) map[string]any {
	t.Helper()
	var raw string
	if err := store.DB.QueryRow(`SELECT payload_json FROM pending_events WHERE event_type = ? ORDER BY id DESC LIMIT 1`, eventType).Scan(&raw); err != nil {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		t.Fatalf("decode event payload: %v", err)
	}
	return payload
}
