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

const migration009 = `
CREATE TABLE IF NOT EXISTS consolidation_runs (
    run_id TEXT PRIMARY KEY,
    archive_date TEXT NOT NULL,
    trigger TEXT NOT NULL,
    status TEXT NOT NULL,
    started_at TEXT,
    completed_at TEXT,
    input_task_history_count INTEGER NOT NULL DEFAULT 0,
    archive_record_id TEXT NOT NULL DEFAULT '',
    summary_record_count INTEGER NOT NULL DEFAULT 0,
    candidate_count INTEGER NOT NULL DEFAULT 0,
    quarantined_count INTEGER NOT NULL DEFAULT 0,
    suppressed_count INTEGER NOT NULL DEFAULT 0,
    active_task_id TEXT NOT NULL DEFAULT '',
    deferred_reason TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    error_summary TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (status IN ('deferred_active_task', 'running', 'completed_summary_only', 'failed_safe')),
    CHECK (input_task_history_count >= 0),
    CHECK (summary_record_count >= 0),
    CHECK (candidate_count >= 0),
    CHECK (quarantined_count >= 0),
    CHECK (suppressed_count >= 0)
);

CREATE INDEX IF NOT EXISTS idx_consolidation_runs_archive_date ON consolidation_runs(archive_date, status);
`

const migration010 = `
CREATE TABLE IF NOT EXISTS memory_entries (
    memory_id TEXT PRIMARY KEY,
    memory_class TEXT NOT NULL,
    status TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '',
    content_excerpt TEXT NOT NULL DEFAULT '',
    source_type TEXT NOT NULL,
    source_id TEXT NOT NULL DEFAULT '',
    trust_tier INTEGER NOT NULL,
    authority_rank TEXT NOT NULL,
    provenance_json TEXT NOT NULL DEFAULT '{}',
    evidence_refs_json TEXT NOT NULL DEFAULT '[]',
    freshness_state TEXT NOT NULL,
    confidence REAL NOT NULL DEFAULT 0,
    contradiction_state TEXT NOT NULL,
    quarantine_status TEXT NOT NULL,
    suppression_status TEXT NOT NULL,
    stale_after TEXT,
    expires_at TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    last_verified_at TEXT,
    allowed_actions_json TEXT NOT NULL DEFAULT '[]',
    audit_event_ids_json TEXT NOT NULL DEFAULT '[]',
    supersedes_memory_id TEXT NOT NULL DEFAULT '',
    rollback_available INTEGER NOT NULL DEFAULT 0,
    external_content_is_not_instruction INTEGER NOT NULL DEFAULT 1,
    retrieval_eligible INTEGER NOT NULL DEFAULT 0,
    retrieval_reason TEXT NOT NULL DEFAULT '',
    embedding_status TEXT NOT NULL,
    candidate_origin_run_id TEXT NOT NULL DEFAULT '',
    promotion_gate_result_json TEXT NOT NULL DEFAULT '{}',
    user_visible INTEGER NOT NULL DEFAULT 0,
    CHECK (memory_class IN ('canon_memory', 'durable_user_preference', 'user_profile_context_fact', 'task_continuity', 'approved_summary', 'runtime_observation', 'episodic_history', 'scratch_transient', 'quarantined_suppressed', 'memory_candidate', 'procedure_candidate', 'retrieval_anchor_candidate', 'contradiction_staleness_finding', 'improvement_proposal')),
    CHECK (status IN ('candidate', 'active', 'needs_confirmation', 'quarantined', 'suppressed', 'stale', 'superseded', 'deleted_by_user', 'rejected')),
    CHECK (source_type IN ('canon', 'main_hub', 'verified_runtime', 'direct_user', 'model_output', 'archive', 'external', 'retrieval', 'skill', 'unknown')),
    CHECK (trust_tier >= 0),
    CHECK (freshness_state IN ('fresh', 'aging', 'stale', 'expired', 'unknown')),
    CHECK (confidence >= 0 AND confidence <= 1),
    CHECK (contradiction_state IN ('none', 'suspected', 'confirmed', 'resolved')),
    CHECK (quarantine_status IN ('clean', 'quarantined', 'blocked', 'unknown')),
    CHECK (suppression_status IN ('active', 'suppressed', 'demoted', 'rolled_back')),
    CHECK (embedding_status IN ('not_needed', 'pending', 'ready', 'failed', 'blocked')),
    CHECK (rollback_available IN (0, 1)),
    CHECK (external_content_is_not_instruction IN (0, 1)),
    CHECK (retrieval_eligible IN (0, 1)),
    CHECK (user_visible IN (0, 1))
);

CREATE TABLE IF NOT EXISTS memory_evidence_refs (
    evidence_id TEXT PRIMARY KEY,
    memory_id TEXT NOT NULL DEFAULT '',
    candidate_id TEXT NOT NULL DEFAULT '',
    source_type TEXT NOT NULL,
    source_id TEXT NOT NULL DEFAULT '',
    source_ref TEXT NOT NULL DEFAULT '',
    evidence_kind TEXT NOT NULL,
    trust_tier INTEGER NOT NULL,
    authority_rank TEXT NOT NULL,
    timestamp TEXT NOT NULL,
    content_hash TEXT NOT NULL DEFAULT '',
    redaction_status TEXT NOT NULL DEFAULT '',
    display_allowed INTEGER NOT NULL DEFAULT 0,
    promotion_allowed INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    CHECK (source_type IN ('canon', 'main_hub', 'verified_runtime', 'direct_user', 'model_output', 'archive', 'external', 'retrieval', 'skill', 'unknown')),
    CHECK (evidence_kind IN ('user_statement', 'runtime_state', 'tool_result', 'app_bridge_evidence', 'task_completion_evidence', 'canon_ref', 'archive_ref', 'external_ref')),
    CHECK (trust_tier >= 0),
    CHECK (display_allowed IN (0, 1)),
    CHECK (promotion_allowed IN (0, 1))
);

CREATE TABLE IF NOT EXISTS memory_action_audit (
    audit_id TEXT PRIMARY KEY,
    memory_id TEXT NOT NULL DEFAULT '',
    candidate_id TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    actor TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL DEFAULT '',
    before_state_json TEXT NOT NULL DEFAULT '{}',
    after_state_json TEXT NOT NULL DEFAULT '{}',
    timestamp TEXT NOT NULL,
    rollback_ref TEXT NOT NULL DEFAULT '',
    gate_result_json TEXT NOT NULL DEFAULT '{}',
    user_confirmation_required INTEGER NOT NULL DEFAULT 0,
    source_request_id TEXT NOT NULL DEFAULT '',
    CHECK (action IN ('view', 'suppress', 'restore_suppression', 'delete_user_remove', 'correct_supersede', 'mark_stale', 'view_provenance', 'rollback', 'approve_candidate', 'reject_candidate', 'quarantine', 'unquarantine')),
    CHECK (user_confirmation_required IN (0, 1))
);

CREATE TABLE IF NOT EXISTS memory_findings (
    finding_id TEXT PRIMARY KEY,
    memory_ids_json TEXT NOT NULL DEFAULT '[]',
    candidate_ids_json TEXT NOT NULL DEFAULT '[]',
    finding_type TEXT NOT NULL,
    confidence REAL NOT NULL DEFAULT 0,
    evidence_refs_json TEXT NOT NULL DEFAULT '[]',
    recommended_action TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL,
    resolver TEXT NOT NULL DEFAULT '',
    audit_event_ids_json TEXT NOT NULL DEFAULT '[]',
    user_visible INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    resolved_at TEXT,
    CHECK (finding_type IN ('contradiction', 'stale', 'unsupported', 'poisoning', 'missing_evidence', 'authority_conflict')),
    CHECK (confidence >= 0 AND confidence <= 1),
    CHECK (status IN ('open', 'needs_review', 'resolved', 'dismissed', 'superseded')),
    CHECK (user_visible IN (0, 1))
);

CREATE TABLE IF NOT EXISTS memory_candidates (
    candidate_id TEXT PRIMARY KEY,
    candidate_type TEXT NOT NULL,
    status TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    content TEXT NOT NULL DEFAULT '',
    source_type TEXT NOT NULL,
    source_id TEXT NOT NULL DEFAULT '',
    trust_tier INTEGER NOT NULL,
    authority_rank TEXT NOT NULL,
    provenance_json TEXT NOT NULL DEFAULT '{}',
    evidence_refs_json TEXT NOT NULL DEFAULT '[]',
    freshness_state TEXT NOT NULL,
    confidence REAL NOT NULL DEFAULT 0,
    contradiction_state TEXT NOT NULL,
    quarantine_status TEXT NOT NULL,
    suppression_status TEXT NOT NULL,
    consolidation_run_id TEXT NOT NULL DEFAULT '',
    promotion_gate_result_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    expires_at TEXT,
    CHECK (candidate_type IN ('memory_candidate', 'procedure_candidate', 'retrieval_anchor_candidate', 'contradiction_staleness_finding', 'improvement_proposal')),
    CHECK (status IN ('generated', 'needs_review', 'approved', 'rejected', 'quarantined', 'suppressed', 'promoted', 'expired')),
    CHECK (source_type IN ('canon', 'main_hub', 'verified_runtime', 'direct_user', 'model_output', 'archive', 'external', 'retrieval', 'skill', 'unknown')),
    CHECK (trust_tier >= 0),
    CHECK (freshness_state IN ('fresh', 'aging', 'stale', 'expired', 'unknown')),
    CHECK (confidence >= 0 AND confidence <= 1),
    CHECK (contradiction_state IN ('none', 'suspected', 'confirmed', 'resolved')),
    CHECK (quarantine_status IN ('clean', 'quarantined', 'blocked', 'unknown')),
    CHECK (suppression_status IN ('active', 'suppressed', 'demoted', 'rolled_back'))
);

CREATE INDEX IF NOT EXISTS idx_memory_entries_class_status ON memory_entries(memory_class, status);
CREATE INDEX IF NOT EXISTS idx_memory_entries_retrieval ON memory_entries(retrieval_eligible, freshness_state, contradiction_state, quarantine_status, suppression_status);
CREATE INDEX IF NOT EXISTS idx_memory_evidence_refs_memory ON memory_evidence_refs(memory_id, candidate_id, evidence_kind);
CREATE INDEX IF NOT EXISTS idx_memory_action_audit_memory ON memory_action_audit(memory_id, candidate_id, action);
CREATE INDEX IF NOT EXISTS idx_memory_findings_status ON memory_findings(status, finding_type);
CREATE INDEX IF NOT EXISTS idx_memory_candidates_status ON memory_candidates(candidate_type, status);
`
