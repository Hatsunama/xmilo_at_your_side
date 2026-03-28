package db

var migration001 = `
CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS runtime_config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS task_slots (
    slot TEXT PRIMARY KEY,
    task_id TEXT,
    prompt TEXT,
    intent TEXT,
    room_id TEXT,
    anchor_id TEXT,
    status TEXT,
    started_at TEXT,
    updated_at TEXT,
    queued_at TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retries INTEGER NOT NULL DEFAULT 3,
    failure_type TEXT,
    stuck_reason TEXT,
    intake_assessment TEXT,
    evidence_boundary TEXT
);

CREATE TABLE IF NOT EXISTS task_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT NOT NULL,
    prompt TEXT NOT NULL,
    status TEXT NOT NULL,
    summary TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS conversation_tail (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS pending_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    event_type TEXT NOT NULL,
    payload_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    replayed_at TEXT
);

CREATE TABLE IF NOT EXISTS capability_queue (
    name TEXT PRIMARY KEY,
    needs_revalidation INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS flags (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`

const migration002 = `
ALTER TABLE task_slots ADD COLUMN intake_assessment TEXT;
`

const migration003 = `
ALTER TABLE task_slots ADD COLUMN evidence_boundary TEXT;
`

const migration004 = `
CREATE TABLE IF NOT EXISTS memory_entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    class TEXT NOT NULL,
    mkey TEXT NOT NULL,
    value TEXT NOT NULL,
    status TEXT NOT NULL,
    source TEXT NOT NULL,
    effect TEXT NOT NULL,
    trust_tier INTEGER NOT NULL DEFAULT 5,
    quarantine_reason TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_memory_entries_class_status_updated
    ON memory_entries(class, status, updated_at);
CREATE INDEX IF NOT EXISTS idx_memory_entries_class_key_status
    ON memory_entries(class, mkey, status);
`
