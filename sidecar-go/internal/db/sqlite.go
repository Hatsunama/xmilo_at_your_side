package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"xmilo/sidecar-go/internal/runtime"
)

type Store struct {
	DB *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)

	s := &Store{DB: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	migrations := []struct {
		version int
		script  string
	}{
		{1, migration001},
		{2, migration002},
		{3, migration003},
		{4, migration004},
	}

	current := 0
	row := s.DB.QueryRow(`SELECT MAX(version) FROM schema_version`)
	if err := row.Scan(&current); err != nil {
		if !strings.Contains(err.Error(), "no such table") {
			return err
		}
		current = 0
	}

	for _, migration := range migrations {
		if migration.version <= current {
			continue
		}
		if _, err := s.DB.Exec(migration.script); err != nil {
			if migration.version == 2 && strings.Contains(err.Error(), "duplicate column name") && strings.Contains(err.Error(), "intake_assessment") {
				// column already exists, nothing to do
			} else if migration.version == 3 && strings.Contains(err.Error(), "duplicate column name") && strings.Contains(err.Error(), "evidence_boundary") {
				// column already exists
			} else if migration.version == 4 && strings.Contains(err.Error(), "already exists") && strings.Contains(err.Error(), "memory_entries") {
				// table already exists (older build already migrated)
			} else {
				return err
			}
		}
		if _, err := s.DB.Exec(`INSERT INTO schema_version(version, applied_at) VALUES(?, ?)`, migration.version, time.Now().UTC().Format(time.RFC3339)); err != nil {
			return err
		}
		current = migration.version
	}
	return nil
}

func (s *Store) SetRuntimeConfig(key, value string) error {
	_, err := s.DB.Exec(`INSERT INTO runtime_config(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *Store) SetRuntimeConfigJSON(key string, value any) error {
	if value == nil {
		_, err := s.DB.Exec(`DELETE FROM runtime_config WHERE key = ?`, key)
		return err
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.SetRuntimeConfig(key, string(raw))
}

func (s *Store) GetRuntimeConfig(key string) (string, error) {
	var value string
	err := s.DB.QueryRow(`SELECT value FROM runtime_config WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func (s *Store) GetRuntimeConfigJSON(key string, out any) error {
	raw, err := s.GetRuntimeConfig(key)
	if err != nil {
		return err
	}
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), out)
}

func (s *Store) SetFlag(key, value string) error {
	_, err := s.DB.Exec(`INSERT INTO flags(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *Store) GetFlag(key string) (string, error) {
	var value string
	err := s.DB.QueryRow(`SELECT value FROM flags WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func (s *Store) UpsertTask(slot string, t runtime.TaskSnapshot) error {
	intakeJSON, err := encodeIntakeAssessment(t.IntakeAssessment)
	if err != nil {
		return err
	}
	evidenceJSON, err := encodeEvidenceBoundary(t.EvidenceBoundary)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`
        INSERT INTO task_slots(slot, task_id, prompt, intent, room_id, anchor_id, status, started_at, updated_at, retry_count, max_retries, failure_type, stuck_reason, intake_assessment, evidence_boundary)
        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(slot) DO UPDATE SET
            task_id = excluded.task_id,
            prompt = excluded.prompt,
            intent = excluded.intent,
            room_id = excluded.room_id,
            anchor_id = excluded.anchor_id,
            status = excluded.status,
            started_at = excluded.started_at,
            updated_at = excluded.updated_at,
            retry_count = excluded.retry_count,
            max_retries = excluded.max_retries,
            failure_type = excluded.failure_type,
            stuck_reason = excluded.stuck_reason,
            intake_assessment = excluded.intake_assessment,
            evidence_boundary = excluded.evidence_boundary
    `, slot, t.TaskID, t.Prompt, t.Intent, t.RoomID, t.AnchorID, t.Status, t.StartedAt, t.UpdatedAt, t.RetryCount, t.MaxRetries, t.FailureType, t.StuckReason, intakeJSON, evidenceJSON)
	return err
}

func (s *Store) GetTask(slot string) (*runtime.TaskSnapshot, error) {
	row := s.DB.QueryRow(`
        SELECT task_id, prompt, intent, room_id, anchor_id, status, started_at, updated_at, retry_count, max_retries, failure_type, stuck_reason
 , intake_assessment, evidence_boundary
        FROM task_slots WHERE slot = ? AND task_id IS NOT NULL
    `, slot)

	var t runtime.TaskSnapshot
	var intakeJSON string
	var evidenceJSON string
	if err := row.Scan(&t.TaskID, &t.Prompt, &t.Intent, &t.RoomID, &t.AnchorID, &t.Status, &t.StartedAt, &t.UpdatedAt, &t.RetryCount, &t.MaxRetries, &t.FailureType, &t.StuckReason, &intakeJSON, &evidenceJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	t.Slot = slot
	if intake, err := decodeIntakeAssessment(intakeJSON); err == nil {
		t.IntakeAssessment = intake
	} else if intakeJSON != "" {
		return nil, err
	}
	if evidence, err := decodeEvidenceBoundary(evidenceJSON); err == nil {
		t.EvidenceBoundary = evidence
	} else if evidenceJSON != "" {
		return nil, err
	}
	return &t, nil
}

func (s *Store) ClearTask(slot string) error {
	_, err := s.DB.Exec(`DELETE FROM task_slots WHERE slot = ?`, slot)
	return err
}

func (s *Store) AddTaskHistory(taskID, prompt, status, summary string) error {
	_, err := s.DB.Exec(`INSERT INTO task_history(task_id, prompt, status, summary, created_at) VALUES(?, ?, ?, ?, ?)`,
		taskID, prompt, status, summary, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) AppendConversation(role, content string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT INTO conversation_tail(role, content, created_at) VALUES(?, ?, ?)`, role, content, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return err
	}

	if _, err := tx.Exec(`
        DELETE FROM conversation_tail
        WHERE id NOT IN (
            SELECT id FROM conversation_tail ORDER BY id DESC LIMIT 12
        )
    `); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) GetConversationTail() ([]map[string]string, error) {
	rows, err := s.DB.Query(`SELECT role, content FROM conversation_tail ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []map[string]string
	for rows.Next() {
		var role, content string
		if err := rows.Scan(&role, &content); err != nil {
			return nil, err
		}
		out = append(out, map[string]string{"role": role, "content": content})
	}
	return out, nil
}

func (s *Store) AppendPendingEvent(eventType string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`INSERT INTO pending_events(event_type, payload_json, created_at) VALUES(?, ?, ?)`,
		eventType, string(raw), time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) StorageStats(dbPath string) (map[string]any, error) {
	stat, err := os.Stat(dbPath)
	if err != nil {
		return nil, err
	}
	var taskRows, eventRows, convRows int
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM task_slots`).Scan(&taskRows)
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM pending_events`).Scan(&eventRows)
	_ = s.DB.QueryRow(`SELECT COUNT(*) FROM conversation_tail`).Scan(&convRows)

	return map[string]any{
		"pico_sqlite_bytes":      stat.Size(),
		"runtime_task_rows":      taskRows,
		"pending_event_rows":     eventRows,
		"conversation_tail_rows": convRows,
	}, nil
}

func (s *Store) QueueCapability(name string) error {
	_, err := s.DB.Exec(`INSERT INTO capability_queue(name, needs_revalidation, created_at) VALUES(?, 1, ?) ON CONFLICT(name) DO UPDATE SET needs_revalidation = 1`,
		name, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) Close() error {
	return s.DB.Close()
}

func (s *Store) SchemaVersion() (int, error) {
	var version int
	err := s.DB.QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&version)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return 0, nil
		}
		return 0, err
	}
	return version, err
}

func encodeIntakeAssessment(assessment *runtime.IntakeAssessment) (string, error) {
	if assessment == nil {
		return "", nil
	}
	raw, err := json.Marshal(assessment)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func decodeIntakeAssessment(raw string) (*runtime.IntakeAssessment, error) {
	if raw == "" {
		return nil, nil
	}
	var assessment runtime.IntakeAssessment
	if err := json.Unmarshal([]byte(raw), &assessment); err != nil {
		return nil, err
	}
	return &assessment, nil
}

func encodeEvidenceBoundary(boundary *runtime.EvidenceBoundary) (string, error) {
	if boundary == nil {
		return "", nil
	}
	raw, err := json.Marshal(boundary)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func decodeEvidenceBoundary(raw string) (*runtime.EvidenceBoundary, error) {
	if raw == "" {
		return nil, nil
	}
	var boundary runtime.EvidenceBoundary
	if err := json.Unmarshal([]byte(raw), &boundary); err != nil {
		return nil, err
	}
	return &boundary, nil
}

const (
	approvalStateConfigKey    = "pending_approval_state"
	resumeCheckpointConfigKey = "resume_checkpoint_state"
)

func (s *Store) SetApprovalState(state *runtime.ApprovalState) error {
	return s.SetRuntimeConfigJSON(approvalStateConfigKey, state)
}

func (s *Store) GetApprovalState() (*runtime.ApprovalState, error) {
	var state runtime.ApprovalState
	if err := s.GetRuntimeConfigJSON(approvalStateConfigKey, &state); err != nil {
		return nil, err
	}
	if state.TaskID == "" {
		return nil, nil
	}
	return &state, nil
}

func (s *Store) ClearApprovalState() error {
	return s.SetApprovalState(nil)
}

func (s *Store) SetResumeCheckpoint(state *runtime.ResumeCheckpoint) error {
	return s.SetRuntimeConfigJSON(resumeCheckpointConfigKey, state)
}

func (s *Store) GetResumeCheckpoint() (*runtime.ResumeCheckpoint, error) {
	var state runtime.ResumeCheckpoint
	if err := s.GetRuntimeConfigJSON(resumeCheckpointConfigKey, &state); err != nil {
		return nil, err
	}
	if state.TaskID == "" {
		return nil, nil
	}
	return &state, nil
}

func (s *Store) ClearResumeCheckpoint() error {
	return s.SetResumeCheckpoint(nil)
}

func (s *Store) SetActiveMemoryEntry(entry runtime.MemoryEntry) error {
	if entry.Key == "" || entry.Value == "" {
		return errors.New("memory entry requires key and value")
	}
	if entry.Source == "" {
		entry.Source = "unknown"
	}
	if entry.Effect == "" {
		entry.Effect = "unspecified"
	}
	if entry.TrustTier == 0 {
		entry.TrustTier = 5
	}
	now := time.Now().UTC().Format(time.RFC3339)
	class := string(entry.Class)
	status := string(runtime.MemoryEntryStatusActive)

	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Supersede any existing active entry for the same class/key so rollback remains possible.
	if _, err := tx.Exec(`UPDATE memory_entries SET status = ?, updated_at = ? WHERE class = ? AND mkey = ? AND status = ?`,
		string(runtime.MemoryEntryStatusSuperseded), now, class, entry.Key, status); err != nil {
		return err
	}

	if _, err := tx.Exec(`INSERT INTO memory_entries(class, mkey, value, status, source, effect, trust_tier, quarantine_reason, created_at, updated_at)
        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		class, entry.Key, entry.Value, status, entry.Source, entry.Effect, entry.TrustTier, entry.QuarantineReason, now, now); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) GetActiveMemoryValue(class runtime.MemoryClass, key string) (string, error) {
	if key == "" {
		return "", nil
	}
	var value string
	err := s.DB.QueryRow(`SELECT value FROM memory_entries WHERE class = ? AND mkey = ? AND status = ? ORDER BY id DESC LIMIT 1`,
		string(class), key, string(runtime.MemoryEntryStatusActive)).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func (s *Store) ListMemoryEntries(class runtime.MemoryClass, status runtime.MemoryEntryStatus, limit int) ([]runtime.MemoryEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.DB.Query(`SELECT id, class, mkey, value, status, source, effect, trust_tier, quarantine_reason, created_at, updated_at
        FROM memory_entries WHERE class = ? AND status = ? ORDER BY id DESC LIMIT ?`,
		string(class), string(status), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []runtime.MemoryEntry
	for rows.Next() {
		var e runtime.MemoryEntry
		var classRaw, statusRaw string
		if err := rows.Scan(&e.ID, &classRaw, &e.Key, &e.Value, &statusRaw, &e.Source, &e.Effect, &e.TrustTier, &e.QuarantineReason, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		e.Class = runtime.MemoryClass(classRaw)
		e.Status = runtime.MemoryEntryStatus(statusRaw)
		out = append(out, e)
	}
	return out, nil
}

func (s *Store) QuarantineActiveMemoryEntry(class runtime.MemoryClass, key, reason string) error {
	if key == "" {
		return errors.New("memory quarantine requires key")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.DB.Exec(`UPDATE memory_entries SET status = ?, quarantine_reason = ?, updated_at = ?
        WHERE class = ? AND mkey = ? AND status = ?`,
		string(runtime.MemoryEntryStatusQuarantined), reason, now, string(class), key, string(runtime.MemoryEntryStatusActive))
	return err
}

func (s *Store) RestoreMostRecentSupersededMemoryEntry(class runtime.MemoryClass, key string) error {
	if key == "" {
		return errors.New("memory restore requires key")
	}
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var restoreID int64
	err = tx.QueryRow(`SELECT id FROM memory_entries WHERE class = ? AND mkey = ? AND status = ? ORDER BY id DESC LIMIT 1`,
		string(class), key, string(runtime.MemoryEntryStatusSuperseded)).Scan(&restoreID)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("no superseded memory entry to restore")
	}
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`UPDATE memory_entries SET status = ?, updated_at = ? WHERE class = ? AND mkey = ? AND status = ?`,
		string(runtime.MemoryEntryStatusSuperseded), now, string(class), key, string(runtime.MemoryEntryStatusActive)); err != nil {
		return err
	}

	if _, err := tx.Exec(`UPDATE memory_entries SET status = ?, updated_at = ? WHERE id = ?`,
		string(runtime.MemoryEntryStatusActive), now, restoreID); err != nil {
		return err
	}
	return tx.Commit()
}

// ConsolidateMemoryBounded prunes only non-active memory rows (superseded/quarantined),
// keeping a bounded tail for rollback/debugging. It must never delete active rows.
// Non-automatic: callers must invoke this explicitly (no background loop wiring here).
func (s *Store) ConsolidateMemoryBounded(class runtime.MemoryClass, key string, maxSuperseded, maxQuarantined, maxAgeDays int) (int, error) {
	if key == "" {
		return 0, errors.New("memory consolidate requires key")
	}
	if maxSuperseded < 0 {
		maxSuperseded = 0
	}
	if maxQuarantined < 0 {
		maxQuarantined = 0
	}
	if maxAgeDays < 0 {
		maxAgeDays = 0
	}
	cutoff := time.Now().UTC().Add(-time.Duration(maxAgeDays) * 24 * time.Hour).Format(time.RFC3339)

	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Defensive: never delete active entries.
	var deleted int64
	for _, status := range []runtime.MemoryEntryStatus{runtime.MemoryEntryStatusSuperseded, runtime.MemoryEntryStatusQuarantined} {
		limit := maxSuperseded
		if status == runtime.MemoryEntryStatusQuarantined {
			limit = maxQuarantined
		}

		_, err := tx.Exec(`DELETE FROM memory_entries
            WHERE class = ? AND mkey = ? AND status = ? AND updated_at < ? AND id NOT IN (
                SELECT id FROM memory_entries
                WHERE class = ? AND mkey = ? AND status = ?
                ORDER BY id DESC LIMIT ?
            )`,
			string(class), key, string(status), cutoff,
			string(class), key, string(status), limit)
		if err != nil {
			return 0, err
		}
		res := tx.QueryRow(`SELECT changes()`)
		var changed int64
		_ = res.Scan(&changed)
		deleted += changed
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int(deleted), nil
}
