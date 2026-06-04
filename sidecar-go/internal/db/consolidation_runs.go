package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"xmilo/sidecar-go/internal/promptsecrecy"
)

type ConsolidationRunStatus string

const (
	ConsolidationRunDeferredActiveTask ConsolidationRunStatus = "deferred_active_task"
	ConsolidationRunRunning            ConsolidationRunStatus = "running"
	ConsolidationRunCompletedSummary   ConsolidationRunStatus = "completed_summary_only"
	ConsolidationRunFailedSafe         ConsolidationRunStatus = "failed_safe"
)

type ConsolidationRun struct {
	RunID                 string
	ArchiveDate           string
	Trigger               string
	Status                ConsolidationRunStatus
	StartedAt             string
	CompletedAt           string
	InputTaskHistoryCount int
	ArchiveRecordID       string
	SummaryRecordCount    int
	CandidateCount        int
	QuarantinedCount      int
	SuppressedCount       int
	ActiveTaskID          string
	DeferredReason        string
	ErrorCode             string
	ErrorSummary          string
	CreatedAt             string
	UpdatedAt             string
}

func (s *Store) UpsertConsolidationRun(run ConsolidationRun) error {
	normalized, err := normalizeConsolidationRun(run)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`
        INSERT INTO consolidation_runs(
            run_id, archive_date, trigger, status, started_at, completed_at, input_task_history_count,
            archive_record_id, summary_record_count, candidate_count, quarantined_count, suppressed_count,
            active_task_id, deferred_reason, error_code, error_summary, created_at, updated_at
        )
        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(run_id) DO UPDATE SET
            archive_date = excluded.archive_date,
            trigger = excluded.trigger,
            status = excluded.status,
            started_at = excluded.started_at,
            completed_at = excluded.completed_at,
            input_task_history_count = excluded.input_task_history_count,
            archive_record_id = excluded.archive_record_id,
            summary_record_count = excluded.summary_record_count,
            candidate_count = excluded.candidate_count,
            quarantined_count = excluded.quarantined_count,
            suppressed_count = excluded.suppressed_count,
            active_task_id = excluded.active_task_id,
            deferred_reason = excluded.deferred_reason,
            error_code = excluded.error_code,
            error_summary = excluded.error_summary,
            updated_at = excluded.updated_at
    `, normalized.RunID, normalized.ArchiveDate, normalized.Trigger, normalized.Status, nullableString(normalized.StartedAt),
		nullableString(normalized.CompletedAt), normalized.InputTaskHistoryCount, normalized.ArchiveRecordID,
		normalized.SummaryRecordCount, normalized.CandidateCount, normalized.QuarantinedCount, normalized.SuppressedCount,
		normalized.ActiveTaskID, normalized.DeferredReason, normalized.ErrorCode, normalized.ErrorSummary,
		normalized.CreatedAt, normalized.UpdatedAt)
	return err
}

func (s *Store) GetConsolidationRun(runID string) (*ConsolidationRun, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return nil, errors.New("consolidation_run_missing_run_id")
	}
	row := s.DB.QueryRow(`
        SELECT run_id, archive_date, trigger, status, COALESCE(started_at, ''), COALESCE(completed_at, ''),
            input_task_history_count, archive_record_id, summary_record_count, candidate_count, quarantined_count,
            suppressed_count, active_task_id, deferred_reason, error_code, error_summary, created_at, updated_at
        FROM consolidation_runs WHERE run_id = ?
    `, runID)
	run, err := scanConsolidationRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return run, err
}

func (s *Store) ListConsolidationRuns() ([]ConsolidationRun, error) {
	rows, err := s.DB.Query(`
        SELECT run_id, archive_date, trigger, status, COALESCE(started_at, ''), COALESCE(completed_at, ''),
            input_task_history_count, archive_record_id, summary_record_count, candidate_count, quarantined_count,
            suppressed_count, active_task_id, deferred_reason, error_code, error_summary, created_at, updated_at
        FROM consolidation_runs ORDER BY archive_date ASC, run_id ASC
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []ConsolidationRun
	for rows.Next() {
		run, err := scanConsolidationRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, *run)
	}
	return runs, rows.Err()
}

type consolidationRunScanner interface {
	Scan(dest ...any) error
}

func scanConsolidationRun(scanner consolidationRunScanner) (*ConsolidationRun, error) {
	var run ConsolidationRun
	if err := scanner.Scan(&run.RunID, &run.ArchiveDate, &run.Trigger, &run.Status, &run.StartedAt, &run.CompletedAt,
		&run.InputTaskHistoryCount, &run.ArchiveRecordID, &run.SummaryRecordCount, &run.CandidateCount,
		&run.QuarantinedCount, &run.SuppressedCount, &run.ActiveTaskID, &run.DeferredReason, &run.ErrorCode,
		&run.ErrorSummary, &run.CreatedAt, &run.UpdatedAt); err != nil {
		return nil, err
	}
	return &run, nil
}

func normalizeConsolidationRun(run ConsolidationRun) (ConsolidationRun, error) {
	run.RunID = strings.TrimSpace(run.RunID)
	run.ArchiveDate = strings.TrimSpace(run.ArchiveDate)
	run.Trigger = strings.TrimSpace(run.Trigger)
	run.ArchiveRecordID = strings.TrimSpace(run.ArchiveRecordID)
	run.ActiveTaskID = strings.TrimSpace(run.ActiveTaskID)
	run.DeferredReason = strings.TrimSpace(run.DeferredReason)
	run.ErrorCode = strings.TrimSpace(run.ErrorCode)
	run.ErrorSummary = promptsecrecy.Redact(strings.TrimSpace(run.ErrorSummary))
	if run.RunID == "" {
		return run, errors.New("consolidation_run_missing_run_id")
	}
	if run.ArchiveDate == "" {
		return run, errors.New("consolidation_run_missing_archive_date")
	}
	if run.Trigger == "" {
		return run, errors.New("consolidation_run_missing_trigger")
	}
	if !isAllowedConsolidationRunStatus(run.Status) {
		return run, errors.New("consolidation_run_invalid_status")
	}
	if run.InputTaskHistoryCount < 0 || run.SummaryRecordCount < 0 || run.CandidateCount < 0 || run.QuarantinedCount < 0 || run.SuppressedCount < 0 {
		return run, errors.New("consolidation_run_negative_count")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if run.CreatedAt == "" {
		run.CreatedAt = now
	}
	if run.UpdatedAt == "" {
		run.UpdatedAt = now
	}
	return run, nil
}

func isAllowedConsolidationRunStatus(status ConsolidationRunStatus) bool {
	switch status {
	case ConsolidationRunDeferredActiveTask, ConsolidationRunRunning, ConsolidationRunCompletedSummary, ConsolidationRunFailedSafe:
		return true
	default:
		return false
	}
}
