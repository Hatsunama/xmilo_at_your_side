package maintenance

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"xmilo/sidecar-go/internal/db"
	"xmilo/sidecar-go/internal/runtime"
	"xmilo/sidecar-go/internal/ws"
)

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

func testScheduler(store *db.Store) *Scheduler {
	return &Scheduler{
		store: store,
		hub:   ws.NewHub(),
		releaseCheckFn: func(context.Context) releaseCheck {
			return releaseCheck{Status: "ok", TagName: "v-test", URL: "https://example.test/release"}
		},
	}
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
