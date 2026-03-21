package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
	if _, err := s.DB.Exec(migration001); err != nil {
		return err
	}
	_, err := s.DB.Exec(`INSERT OR IGNORE INTO schema_version(version, applied_at) VALUES(1, ?)`, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) SetRuntimeConfig(key, value string) error {
	_, err := s.DB.Exec(`INSERT INTO runtime_config(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *Store) GetRuntimeConfig(key string) (string, error) {
	var value string
	err := s.DB.QueryRow(`SELECT value FROM runtime_config WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
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
	_, err := s.DB.Exec(`
        INSERT INTO task_slots(slot, task_id, prompt, intent, room_id, anchor_id, status, started_at, updated_at, retry_count, max_retries, failure_type, stuck_reason)
        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
            stuck_reason = excluded.stuck_reason
    `, slot, t.TaskID, t.Prompt, t.Intent, t.RoomID, t.AnchorID, t.Status, t.StartedAt, t.UpdatedAt, t.RetryCount, t.MaxRetries, t.FailureType, t.StuckReason)
	return err
}

func (s *Store) GetTask(slot string) (*runtime.TaskSnapshot, error) {
	row := s.DB.QueryRow(`
        SELECT task_id, prompt, intent, room_id, anchor_id, status, started_at, updated_at, retry_count, max_retries, failure_type, stuck_reason
        FROM task_slots WHERE slot = ? AND task_id IS NOT NULL
    `, slot)

	var t runtime.TaskSnapshot
	if err := row.Scan(&t.TaskID, &t.Prompt, &t.Intent, &t.RoomID, &t.AnchorID, &t.Status, &t.StartedAt, &t.UpdatedAt, &t.RetryCount, &t.MaxRetries, &t.FailureType, &t.StuckReason); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	t.Slot = slot
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
	return version, err
}
