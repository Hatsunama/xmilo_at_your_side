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

const migration005 = `
CREATE TABLE IF NOT EXISTS external_imports (
    import_id TEXT PRIMARY KEY,
    item_type TEXT NOT NULL,
    source_type TEXT NOT NULL,
    source_uri TEXT NOT NULL DEFAULT '',
    source_hash TEXT NOT NULL DEFAULT '',
    display_name TEXT NOT NULL DEFAULT '',
    version TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL,
    trust_tier INTEGER NOT NULL DEFAULT 0,
    authority_rank TEXT NOT NULL DEFAULT 'external_untrusted',
    activation_state TEXT NOT NULL DEFAULT 'disabled',
    declared_capabilities_json TEXT NOT NULL DEFAULT '[]',
    requested_tools_json TEXT NOT NULL DEFAULT '[]',
    risk_findings_json TEXT NOT NULL DEFAULT '[]',
    validation_result_json TEXT NOT NULL DEFAULT '{}',
    provenance_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (item_type IN ('skill', 'plugin', 'tool_descriptor', 'prompt_pack', 'retrieval_source', 'unknown')),
    CHECK (source_type IN ('online', 'github', 'local', 'user_built', 'bundled', 'unknown')),
    CHECK (state IN ('imported_untrusted', 'quarantined', 'validated_candidate', 'blocked', 'approved_inactive', 'active_scoped', 'disabled', 'removed')),
    CHECK (activation_state IN ('disabled', 'approved_inactive', 'active_scoped', 'removed')),
    CHECK (trust_tier >= 0)
);

CREATE TABLE IF NOT EXISTS external_import_artifacts (
    artifact_id INTEGER PRIMARY KEY AUTOINCREMENT,
    import_id TEXT NOT NULL,
    artifact_type TEXT NOT NULL,
    artifact_path TEXT NOT NULL,
    content_hash TEXT NOT NULL DEFAULT '',
    quarantine_state TEXT NOT NULL DEFAULT 'quarantined',
    created_at TEXT NOT NULL,
    FOREIGN KEY(import_id) REFERENCES external_imports(import_id) ON DELETE CASCADE,
    CHECK (quarantine_state IN ('imported_untrusted', 'quarantined', 'validated_candidate', 'blocked', 'approved_inactive', 'active_scoped', 'disabled', 'removed'))
);

CREATE TABLE IF NOT EXISTS external_activation_allowlist (
    import_id TEXT PRIMARY KEY,
    activation_state TEXT NOT NULL,
    scoped_permissions_json TEXT NOT NULL DEFAULT '[]',
    activated_by TEXT NOT NULL DEFAULT 'runtime',
    activated_at TEXT,
    expires_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(import_id) REFERENCES external_imports(import_id) ON DELETE CASCADE,
    CHECK (activation_state IN ('approved_inactive', 'active_scoped', 'disabled', 'removed'))
);
`

const migration006 = `
CREATE TABLE IF NOT EXISTS external_import_tool_descriptors (
    tool_id TEXT PRIMARY KEY,
    import_id TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    risk_classes_json TEXT NOT NULL DEFAULT '[]',
    side_effects_declared INTEGER NOT NULL DEFAULT 0,
    side_effects INTEGER NOT NULL DEFAULT 0,
    requires_confirmation INTEGER NOT NULL DEFAULT 0,
    requested_capabilities_json TEXT NOT NULL DEFAULT '[]',
    input_schema_json TEXT NOT NULL DEFAULT '{}',
    output_schema_json TEXT NOT NULL DEFAULT '{}',
    validation_state TEXT NOT NULL,
    risk_findings_json TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(import_id) REFERENCES external_imports(import_id) ON DELETE CASCADE,
    CHECK (validation_state IN ('imported_untrusted', 'quarantined', 'validated_candidate', 'blocked', 'approved_inactive', 'active_scoped', 'disabled', 'removed')),
    CHECK (side_effects_declared IN (0, 1)),
    CHECK (side_effects IN (0, 1)),
    CHECK (requires_confirmation IN (0, 1))
);

CREATE TABLE IF NOT EXISTS external_tool_activation_allowlist (
    tool_id TEXT PRIMARY KEY,
    import_id TEXT NOT NULL,
    activation_state TEXT NOT NULL,
    scoped_permissions_json TEXT NOT NULL DEFAULT '[]',
    activated_by TEXT NOT NULL DEFAULT 'runtime',
    activated_at TEXT,
    expires_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY(import_id) REFERENCES external_imports(import_id) ON DELETE CASCADE,
    FOREIGN KEY(tool_id) REFERENCES external_import_tool_descriptors(tool_id) ON DELETE CASCADE,
    CHECK (activation_state IN ('approved_inactive', 'active_scoped', 'disabled', 'removed'))
);
`

const migration007 = `
CREATE TABLE IF NOT EXISTS retrieval_records (
    chunk_id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL,
    source_type TEXT NOT NULL,
    trust_tier INTEGER NOT NULL,
    authority_rank TEXT NOT NULL,
    provenance_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    expires_at TEXT,
    freshness TEXT NOT NULL DEFAULT 'unknown',
    content_hash TEXT NOT NULL,
    quarantine_status TEXT NOT NULL,
    contains_external_instruction INTEGER NOT NULL DEFAULT 0,
    contains_secret INTEGER NOT NULL DEFAULT 0,
    embedding_model TEXT NOT NULL DEFAULT '',
    embedding_version TEXT NOT NULL DEFAULT '',
    content_summary TEXT NOT NULL DEFAULT '',
    raw_content_ref TEXT NOT NULL DEFAULT '',
    embedding_json TEXT NOT NULL DEFAULT '[]',
    CHECK (source_type IN ('canon', 'runtime_state', 'memory', 'archive', 'external', 'skill', 'plugin', 'user_file', 'unknown')),
    CHECK (trust_tier >= 0),
    CHECK (quarantine_status IN ('clean', 'quarantined', 'blocked', 'unknown')),
    CHECK (contains_external_instruction IN (0, 1)),
    CHECK (contains_secret IN (0, 1))
);

CREATE INDEX IF NOT EXISTS idx_retrieval_records_source ON retrieval_records(source_id, source_type);
CREATE INDEX IF NOT EXISTS idx_retrieval_records_rank ON retrieval_records(authority_rank, trust_tier, quarantine_status);
`

const migration008 = `
CREATE TABLE IF NOT EXISTS durable_poisoning_records (
    record_key TEXT PRIMARY KEY,
    record_kind TEXT NOT NULL,
    source_id TEXT NOT NULL,
    source_type TEXT NOT NULL,
    trust_tier INTEGER NOT NULL,
    authority_rank TEXT NOT NULL,
    source_hash TEXT NOT NULL DEFAULT '',
    expected_source_hash TEXT NOT NULL DEFAULT '',
    provenance_chain_json TEXT NOT NULL,
    poisoning_findings_json TEXT NOT NULL DEFAULT '[]',
    quarantine_status TEXT NOT NULL,
    stale INTEGER NOT NULL DEFAULT 0,
    conflict INTEGER NOT NULL DEFAULT 0,
    test_fixture INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    expires_at TEXT,
    CHECK (trust_tier >= 0),
    CHECK (quarantine_status IN ('clean', 'quarantined', 'blocked')),
    CHECK (stale IN (0, 1)),
    CHECK (conflict IN (0, 1)),
    CHECK (test_fixture IN (0, 1))
);

CREATE INDEX IF NOT EXISTS idx_durable_poisoning_records_kind ON durable_poisoning_records(record_kind, source_type, quarantine_status);
`
