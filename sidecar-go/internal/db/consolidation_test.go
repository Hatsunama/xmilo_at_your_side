package db

import (
	"path/filepath"
	"testing"
)

func TestConsolidationRunLedgerPersistsStatusAndCounts(t *testing.T) {
	store := openConsolidationStore(t)
	run := ConsolidationRun{
		RunID:                 "nightly_consolidation_2026-06-03",
		ArchiveDate:           "2026-06-03",
		Trigger:               "scheduled",
		Status:                ConsolidationRunCompletedSummary,
		StartedAt:             "2026-06-04T02:00:00Z",
		CompletedAt:           "2026-06-04T02:00:01Z",
		InputTaskHistoryCount: 4,
		ArchiveRecordID:       "nightly_archive_2026-06-03",
		SummaryRecordCount:    1,
		CandidateCount:        0,
		QuarantinedCount:      0,
		SuppressedCount:       0,
	}
	if err := store.UpsertConsolidationRun(run); err != nil {
		t.Fatalf("upsert consolidation run: %v", err)
	}
	loaded, err := store.GetConsolidationRun(run.RunID)
	if err != nil {
		t.Fatalf("get consolidation run: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected consolidation run")
	}
	if loaded.Status != ConsolidationRunCompletedSummary || loaded.InputTaskHistoryCount != 4 || loaded.ArchiveRecordID != run.ArchiveRecordID {
		t.Fatalf("ledger fields did not persist: %#v", loaded)
	}
	if loaded.SummaryRecordCount != 1 || loaded.CandidateCount != 0 || loaded.QuarantinedCount != 0 || loaded.SuppressedCount != 0 {
		t.Fatalf("summary-only counts drifted: %#v", loaded)
	}
}

func TestConsolidationRunLedgerRejectsUnsupportedStatusAndKeepsMigrationNonDestructive(t *testing.T) {
	store := openConsolidationStore(t)
	version, err := store.SchemaVersion()
	if err != nil {
		t.Fatalf("schema version: %v", err)
	}
	if version < 9 {
		t.Fatalf("expected migration 9 or later, got %d", version)
	}
	if _, err := store.DB.Exec(`SELECT run_id, archive_date, trigger, status, created_at, updated_at FROM consolidation_runs LIMIT 1`); err != nil {
		t.Fatalf("consolidation_runs table missing required fields: %v", err)
	}
	err = store.UpsertConsolidationRun(ConsolidationRun{
		RunID:       "bad_status",
		ArchiveDate: "2026-06-03",
		Trigger:     "scheduled",
		Status:      "completed_with_candidates",
	})
	if err == nil || err.Error() != "consolidation_run_invalid_status" {
		t.Fatalf("expected invalid status error, got %v", err)
	}
}

func openConsolidationStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
