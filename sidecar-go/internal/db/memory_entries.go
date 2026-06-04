package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"xmilo/sidecar-go/internal/promptsecrecy"
)

type MemoryEntry struct {
	MemoryID                        string
	MemoryClass                     string
	Status                          string
	Title                           string
	Summary                         string
	Content                         string
	ContentExcerpt                  string
	SourceType                      string
	SourceID                        string
	TrustTier                       int
	AuthorityRank                   string
	Provenance                      map[string]any
	EvidenceRefs                    []string
	FreshnessState                  string
	Confidence                      float64
	ContradictionState              string
	QuarantineStatus                string
	SuppressionStatus               string
	StaleAfter                      string
	ExpiresAt                       string
	CreatedAt                       string
	UpdatedAt                       string
	LastVerifiedAt                  string
	AllowedActions                  []string
	AuditEventIDs                   []string
	SupersedesMemoryID              string
	RollbackAvailable               bool
	ExternalContentIsNotInstruction bool
	RetrievalEligible               bool
	RetrievalReason                 string
	EmbeddingStatus                 string
	CandidateOriginRunID            string
	PromotionGateResult             map[string]any
	UserVisible                     bool
}

type MemoryCandidate struct {
	CandidateID         string
	CandidateType       string
	Status              string
	Title               string
	Summary             string
	Content             string
	SourceType          string
	SourceID            string
	TrustTier           int
	AuthorityRank       string
	Provenance          map[string]any
	EvidenceRefs        []string
	FreshnessState      string
	Confidence          float64
	ContradictionState  string
	QuarantineStatus    string
	SuppressionStatus   string
	ConsolidationRunID  string
	PromotionGateResult map[string]any
	CreatedAt           string
	UpdatedAt           string
	ExpiresAt           string
}

type MemoryEvidenceRef struct {
	EvidenceID       string
	MemoryID         string
	CandidateID      string
	SourceType       string
	SourceID         string
	SourceRef        string
	EvidenceKind     string
	TrustTier        int
	AuthorityRank    string
	Timestamp        string
	ContentHash      string
	RedactionStatus  string
	DisplayAllowed   bool
	PromotionAllowed bool
	CreatedAt        string
}

type MemoryActionAudit struct {
	AuditID                  string
	MemoryID                 string
	CandidateID              string
	Action                   string
	Actor                    string
	Reason                   string
	BeforeState              map[string]any
	AfterState               map[string]any
	Timestamp                string
	RollbackRef              string
	GateResult               map[string]any
	UserConfirmationRequired bool
	SourceRequestID          string
}

type MemoryFinding struct {
	FindingID         string
	MemoryIDs         []string
	CandidateIDs      []string
	FindingType       string
	Confidence        float64
	EvidenceRefs      []string
	RecommendedAction string
	Status            string
	Resolver          string
	AuditEventIDs     []string
	UserVisible       bool
	CreatedAt         string
	UpdatedAt         string
	ResolvedAt        string
}

func (s *Store) UpsertMemoryEntry(entry MemoryEntry) error {
	normalized, err := s.normalizeMemoryEntry(entry)
	if err != nil {
		return err
	}
	provenance, err := encodeStringMap(normalized.Provenance)
	if err != nil {
		return err
	}
	evidenceRefs, err := encodeStringList(normalized.EvidenceRefs)
	if err != nil {
		return err
	}
	allowedActions, err := encodeStringList(normalized.AllowedActions)
	if err != nil {
		return err
	}
	auditEventIDs, err := encodeStringList(normalized.AuditEventIDs)
	if err != nil {
		return err
	}
	gateResult, err := encodeSafeJSON(normalized.PromotionGateResult)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`
        INSERT INTO memory_entries(
            memory_id, memory_class, status, title, summary, content, content_excerpt, source_type, source_id,
            trust_tier, authority_rank, provenance_json, evidence_refs_json, freshness_state, confidence,
            contradiction_state, quarantine_status, suppression_status, stale_after, expires_at, created_at,
            updated_at, last_verified_at, allowed_actions_json, audit_event_ids_json, supersedes_memory_id,
            rollback_available, external_content_is_not_instruction, retrieval_eligible, retrieval_reason,
            embedding_status, candidate_origin_run_id, promotion_gate_result_json, user_visible
        )
        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(memory_id) DO UPDATE SET
            memory_class = excluded.memory_class,
            status = excluded.status,
            title = excluded.title,
            summary = excluded.summary,
            content = excluded.content,
            content_excerpt = excluded.content_excerpt,
            source_type = excluded.source_type,
            source_id = excluded.source_id,
            trust_tier = excluded.trust_tier,
            authority_rank = excluded.authority_rank,
            provenance_json = excluded.provenance_json,
            evidence_refs_json = excluded.evidence_refs_json,
            freshness_state = excluded.freshness_state,
            confidence = excluded.confidence,
            contradiction_state = excluded.contradiction_state,
            quarantine_status = excluded.quarantine_status,
            suppression_status = excluded.suppression_status,
            stale_after = excluded.stale_after,
            expires_at = excluded.expires_at,
            updated_at = excluded.updated_at,
            last_verified_at = excluded.last_verified_at,
            allowed_actions_json = excluded.allowed_actions_json,
            audit_event_ids_json = excluded.audit_event_ids_json,
            supersedes_memory_id = excluded.supersedes_memory_id,
            rollback_available = excluded.rollback_available,
            external_content_is_not_instruction = excluded.external_content_is_not_instruction,
            retrieval_eligible = excluded.retrieval_eligible,
            retrieval_reason = excluded.retrieval_reason,
            embedding_status = excluded.embedding_status,
            candidate_origin_run_id = excluded.candidate_origin_run_id,
            promotion_gate_result_json = excluded.promotion_gate_result_json,
            user_visible = excluded.user_visible
    `, normalized.MemoryID, normalized.MemoryClass, normalized.Status, normalized.Title, normalized.Summary, normalized.Content, normalized.ContentExcerpt,
		normalized.SourceType, normalized.SourceID, normalized.TrustTier, normalized.AuthorityRank, provenance, evidenceRefs, normalized.FreshnessState,
		normalized.Confidence, normalized.ContradictionState, normalized.QuarantineStatus, normalized.SuppressionStatus, nullableString(normalized.StaleAfter),
		nullableString(normalized.ExpiresAt), normalized.CreatedAt, normalized.UpdatedAt, nullableString(normalized.LastVerifiedAt), allowedActions, auditEventIDs,
		normalized.SupersedesMemoryID, boolToInt(normalized.RollbackAvailable), boolToInt(normalized.ExternalContentIsNotInstruction),
		boolToInt(normalized.RetrievalEligible), normalized.RetrievalReason, normalized.EmbeddingStatus, normalized.CandidateOriginRunID, gateResult,
		boolToInt(normalized.UserVisible))
	return err
}

func (s *Store) GetMemoryEntry(memoryID string) (*MemoryEntry, error) {
	memoryID = strings.TrimSpace(memoryID)
	if memoryID == "" {
		return nil, errors.New("memory_entry_missing_id")
	}
	row := s.DB.QueryRow(`
        SELECT memory_id, memory_class, status, title, summary, content, content_excerpt, source_type, source_id,
            trust_tier, authority_rank, provenance_json, evidence_refs_json, freshness_state, confidence,
            contradiction_state, quarantine_status, suppression_status, COALESCE(stale_after, ''), COALESCE(expires_at, ''),
            created_at, updated_at, COALESCE(last_verified_at, ''), allowed_actions_json, audit_event_ids_json,
            supersedes_memory_id, rollback_available, external_content_is_not_instruction, retrieval_eligible,
            retrieval_reason, embedding_status, candidate_origin_run_id, promotion_gate_result_json, user_visible
        FROM memory_entries WHERE memory_id = ?
    `, memoryID)
	entry, err := scanMemoryEntry(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return entry, err
}

func (s *Store) ListMemoryEntriesForRetrievalPack() ([]MemoryEntry, error) {
	rows, err := s.DB.Query(`
        SELECT memory_id, memory_class, status, title, summary, content, content_excerpt, source_type, source_id,
            trust_tier, authority_rank, provenance_json, evidence_refs_json, freshness_state, confidence,
            contradiction_state, quarantine_status, suppression_status, COALESCE(stale_after, ''), COALESCE(expires_at, ''),
            created_at, updated_at, COALESCE(last_verified_at, ''), allowed_actions_json, audit_event_ids_json,
            supersedes_memory_id, rollback_available, external_content_is_not_instruction, retrieval_eligible,
            retrieval_reason, embedding_status, candidate_origin_run_id, promotion_gate_result_json, user_visible
        FROM memory_entries
        ORDER BY authority_rank ASC, trust_tier ASC, memory_class ASC, memory_id ASC
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []MemoryEntry
	for rows.Next() {
		entry, err := scanMemoryEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, *entry)
	}
	return entries, rows.Err()
}

func (s *Store) ListMemoryCandidates() ([]MemoryCandidate, error) {
	rows, err := s.DB.Query(`
        SELECT candidate_id, candidate_type, status, title, summary, content, source_type, source_id, trust_tier,
            authority_rank, provenance_json, evidence_refs_json, freshness_state, confidence, contradiction_state,
            quarantine_status, suppression_status, consolidation_run_id, promotion_gate_result_json, created_at,
            updated_at, COALESCE(expires_at, '')
        FROM memory_candidates
        ORDER BY authority_rank ASC, trust_tier ASC, candidate_type ASC, candidate_id ASC
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []MemoryCandidate
	for rows.Next() {
		candidate, err := scanMemoryCandidate(rows)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, *candidate)
	}
	return candidates, rows.Err()
}

func (s *Store) ListMemoryEvidenceRefsForMemoryIDs(memoryIDs []string) ([]MemoryEvidenceRef, error) {
	seen := map[string]bool{}
	var ids []string
	for _, memoryID := range memoryIDs {
		memoryID = strings.TrimSpace(memoryID)
		if memoryID == "" || seen[memoryID] {
			continue
		}
		seen[memoryID] = true
		ids = append(ids, memoryID)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := s.DB.Query(`
        SELECT evidence_id, memory_id, candidate_id, source_type, source_id, source_ref, evidence_kind,
            trust_tier, authority_rank, timestamp, content_hash, redaction_status, display_allowed,
            promotion_allowed, created_at
        FROM memory_evidence_refs
        WHERE memory_id IN (`+placeholders+`)
        ORDER BY memory_id ASC, authority_rank ASC, trust_tier ASC, evidence_id ASC
    `, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var refs []MemoryEvidenceRef
	for rows.Next() {
		ref, err := scanMemoryEvidenceRef(rows)
		if err != nil {
			return nil, err
		}
		refs = append(refs, *ref)
	}
	return refs, rows.Err()
}

func (s *Store) UpsertMemoryCandidate(candidate MemoryCandidate) error {
	normalized, err := normalizeMemoryCandidate(candidate)
	if err != nil {
		return err
	}
	provenance, err := encodeStringMap(normalized.Provenance)
	if err != nil {
		return err
	}
	evidenceRefs, err := encodeStringList(normalized.EvidenceRefs)
	if err != nil {
		return err
	}
	gateResult, err := encodeSafeJSON(normalized.PromotionGateResult)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`
        INSERT INTO memory_candidates(
            candidate_id, candidate_type, status, title, summary, content, source_type, source_id, trust_tier,
            authority_rank, provenance_json, evidence_refs_json, freshness_state, confidence, contradiction_state,
            quarantine_status, suppression_status, consolidation_run_id, promotion_gate_result_json, created_at,
            updated_at, expires_at
        )
        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(candidate_id) DO UPDATE SET
            candidate_type = excluded.candidate_type,
            status = excluded.status,
            title = excluded.title,
            summary = excluded.summary,
            content = excluded.content,
            source_type = excluded.source_type,
            source_id = excluded.source_id,
            trust_tier = excluded.trust_tier,
            authority_rank = excluded.authority_rank,
            provenance_json = excluded.provenance_json,
            evidence_refs_json = excluded.evidence_refs_json,
            freshness_state = excluded.freshness_state,
            confidence = excluded.confidence,
            contradiction_state = excluded.contradiction_state,
            quarantine_status = excluded.quarantine_status,
            suppression_status = excluded.suppression_status,
            consolidation_run_id = excluded.consolidation_run_id,
            promotion_gate_result_json = excluded.promotion_gate_result_json,
            updated_at = excluded.updated_at,
            expires_at = excluded.expires_at
    `, normalized.CandidateID, normalized.CandidateType, normalized.Status, normalized.Title, normalized.Summary, normalized.Content,
		normalized.SourceType, normalized.SourceID, normalized.TrustTier, normalized.AuthorityRank, provenance, evidenceRefs,
		normalized.FreshnessState, normalized.Confidence, normalized.ContradictionState, normalized.QuarantineStatus,
		normalized.SuppressionStatus, normalized.ConsolidationRunID, gateResult, normalized.CreatedAt, normalized.UpdatedAt,
		nullableString(normalized.ExpiresAt))
	return err
}

func (s *Store) GetMemoryCandidate(candidateID string) (*MemoryCandidate, error) {
	candidateID = strings.TrimSpace(candidateID)
	if candidateID == "" {
		return nil, errors.New("memory_candidate_missing_id")
	}
	row := s.DB.QueryRow(`
        SELECT candidate_id, candidate_type, status, title, summary, content, source_type, source_id, trust_tier,
            authority_rank, provenance_json, evidence_refs_json, freshness_state, confidence, contradiction_state,
            quarantine_status, suppression_status, consolidation_run_id, promotion_gate_result_json, created_at,
            updated_at, COALESCE(expires_at, '')
        FROM memory_candidates WHERE candidate_id = ?
    `, candidateID)
	candidate, err := scanMemoryCandidate(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return candidate, err
}

func (s *Store) AppendMemoryEvidenceRef(ref MemoryEvidenceRef) error {
	normalized, err := normalizeMemoryEvidenceRef(ref)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`
        INSERT INTO memory_evidence_refs(
            evidence_id, memory_id, candidate_id, source_type, source_id, source_ref, evidence_kind, trust_tier,
            authority_rank, timestamp, content_hash, redaction_status, display_allowed, promotion_allowed, created_at
        )
        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `, normalized.EvidenceID, normalized.MemoryID, normalized.CandidateID, normalized.SourceType, normalized.SourceID,
		normalized.SourceRef, normalized.EvidenceKind, normalized.TrustTier, normalized.AuthorityRank, normalized.Timestamp,
		normalized.ContentHash, normalized.RedactionStatus, boolToInt(normalized.DisplayAllowed), boolToInt(normalized.PromotionAllowed),
		normalized.CreatedAt)
	return err
}

func (s *Store) AppendMemoryActionAudit(audit MemoryActionAudit) error {
	normalized, err := normalizeMemoryActionAudit(audit)
	if err != nil {
		return err
	}
	beforeState, err := encodeSafeJSON(normalized.BeforeState)
	if err != nil {
		return err
	}
	afterState, err := encodeSafeJSON(normalized.AfterState)
	if err != nil {
		return err
	}
	gateResult, err := encodeSafeJSON(normalized.GateResult)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`
        INSERT INTO memory_action_audit(
            audit_id, memory_id, candidate_id, action, actor, reason, before_state_json, after_state_json,
            timestamp, rollback_ref, gate_result_json, user_confirmation_required, source_request_id
        )
        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `, normalized.AuditID, normalized.MemoryID, normalized.CandidateID, normalized.Action, normalized.Actor,
		normalized.Reason, beforeState, afterState, normalized.Timestamp, normalized.RollbackRef, gateResult,
		boolToInt(normalized.UserConfirmationRequired), normalized.SourceRequestID)
	return err
}

func (s *Store) ListMemoryActionAudit() ([]MemoryActionAudit, error) {
	rows, err := s.DB.Query(`
        SELECT audit_id, memory_id, candidate_id, action, actor, reason, before_state_json, after_state_json,
            timestamp, rollback_ref, gate_result_json, user_confirmation_required, source_request_id
        FROM memory_action_audit
        ORDER BY timestamp DESC, audit_id DESC
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var audits []MemoryActionAudit
	for rows.Next() {
		audit, err := scanMemoryActionAudit(rows)
		if err != nil {
			return nil, err
		}
		audits = append(audits, *audit)
	}
	return audits, rows.Err()
}

func (s *Store) UpsertMemoryFinding(finding MemoryFinding) error {
	normalized, err := normalizeMemoryFinding(finding)
	if err != nil {
		return err
	}
	memoryIDs, err := encodeStringList(normalized.MemoryIDs)
	if err != nil {
		return err
	}
	candidateIDs, err := encodeStringList(normalized.CandidateIDs)
	if err != nil {
		return err
	}
	evidenceRefs, err := encodeStringList(normalized.EvidenceRefs)
	if err != nil {
		return err
	}
	auditEventIDs, err := encodeStringList(normalized.AuditEventIDs)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`
        INSERT INTO memory_findings(
            finding_id, memory_ids_json, candidate_ids_json, finding_type, confidence, evidence_refs_json,
            recommended_action, status, resolver, audit_event_ids_json, user_visible, created_at, updated_at, resolved_at
        )
        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(finding_id) DO UPDATE SET
            memory_ids_json = excluded.memory_ids_json,
            candidate_ids_json = excluded.candidate_ids_json,
            finding_type = excluded.finding_type,
            confidence = excluded.confidence,
            evidence_refs_json = excluded.evidence_refs_json,
            recommended_action = excluded.recommended_action,
            status = excluded.status,
            resolver = excluded.resolver,
            audit_event_ids_json = excluded.audit_event_ids_json,
            user_visible = excluded.user_visible,
            updated_at = excluded.updated_at,
            resolved_at = excluded.resolved_at
    `, normalized.FindingID, memoryIDs, candidateIDs, normalized.FindingType, normalized.Confidence, evidenceRefs,
		normalized.RecommendedAction, normalized.Status, normalized.Resolver, auditEventIDs, boolToInt(normalized.UserVisible),
		normalized.CreatedAt, normalized.UpdatedAt, nullableString(normalized.ResolvedAt))
	return err
}

func (s *Store) normalizeMemoryEntry(entry MemoryEntry) (MemoryEntry, error) {
	entry.MemoryID = strings.TrimSpace(entry.MemoryID)
	entry.MemoryClass = strings.TrimSpace(entry.MemoryClass)
	entry.Status = strings.TrimSpace(entry.Status)
	entry.SourceType = strings.TrimSpace(entry.SourceType)
	entry.SourceID = strings.TrimSpace(entry.SourceID)
	entry.AuthorityRank = strings.TrimSpace(entry.AuthorityRank)
	entry.SupersedesMemoryID = strings.TrimSpace(entry.SupersedesMemoryID)
	entry.CandidateOriginRunID = strings.TrimSpace(entry.CandidateOriginRunID)
	if entry.MemoryID == "" {
		return entry, errors.New("memory_entry_missing_id")
	}
	if !isAllowedMemoryClass(entry.MemoryClass) {
		return entry, errors.New("memory_entry_invalid_class")
	}
	if isCandidateMemoryClass(entry.MemoryClass) {
		return entry, errors.New("memory_entry_candidate_class_requires_memory_candidates")
	}
	if !isAllowedMemoryEntryStatus(entry.Status) {
		return entry, errors.New("memory_entry_invalid_status")
	}
	if entry.Status == "candidate" {
		return entry, errors.New("memory_entry_candidate_status_requires_memory_candidates")
	}
	if !isAllowedMemorySourceType(entry.SourceType) {
		return entry, errors.New("memory_entry_invalid_source_type")
	}
	if entry.TrustTier < 0 {
		return entry, errors.New("memory_entry_invalid_trust_tier")
	}
	if entry.AuthorityRank == "" {
		return entry, errors.New("memory_entry_missing_authority_rank")
	}
	if len(entry.Provenance) == 0 {
		return entry, errors.New("memory_entry_missing_provenance")
	}
	if !isAllowedFreshnessState(entry.FreshnessState) {
		return entry, errors.New("memory_entry_invalid_freshness_state")
	}
	if entry.Confidence < 0 || entry.Confidence > 1 {
		return entry, errors.New("memory_entry_invalid_confidence")
	}
	if !isAllowedContradictionState(entry.ContradictionState) {
		return entry, errors.New("memory_entry_invalid_contradiction_state")
	}
	if !isAllowedMemoryQuarantineStatus(entry.QuarantineStatus) {
		return entry, errors.New("memory_entry_invalid_quarantine_status")
	}
	if !isAllowedSuppressionStatus(entry.SuppressionStatus) {
		return entry, errors.New("memory_entry_invalid_suppression_status")
	}
	if !isAllowedEmbeddingStatus(entry.EmbeddingStatus) {
		return entry, errors.New("memory_entry_invalid_embedding_status")
	}
	if entry.Status == "active" && entry.SourceType == "model_output" {
		return entry, errors.New("memory_entry_model_output_active_blocked")
	}
	if entry.MemoryClass == "canon_memory" && entry.SourceType != "canon" && entry.SourceType != "main_hub" {
		return entry, errors.New("memory_entry_protected_authority_rewrite")
	}
	if entry.MemoryClass == "runtime_observation" && entry.SourceType != "verified_runtime" {
		return entry, errors.New("memory_entry_runtime_truth_requires_verified_runtime")
	}
	if protectedTruthMutation(entry.Title, entry.Summary, entry.Content) && entry.SourceType != "canon" && entry.SourceType != "main_hub" && entry.SourceType != "verified_runtime" {
		return entry, errors.New("memory_entry_protected_truth_rewrite")
	}
	if entry.SourceType == "external" && !entry.ExternalContentIsNotInstruction {
		return entry, errors.New("memory_entry_external_content_instruction")
	}
	if promptsecrecy.Classify(strings.Join([]string{entry.Title, entry.Summary, entry.Content, entry.ContentExcerpt}, "\n")).Forbidden() {
		return entry, errors.New("memory_entry_secret_content")
	}
	if entry.SupersedesMemoryID != "" {
		superseded, err := s.GetMemoryEntry(entry.SupersedesMemoryID)
		if err != nil {
			return entry, err
		}
		if superseded == nil {
			return entry, errors.New("memory_entry_supersedes_missing")
		}
		if entry.SourceType != "direct_user" || !userCorrectionCanSupersede(superseded.MemoryClass) {
			return entry, errors.New("memory_entry_supersedes_protected_memory")
		}
	}
	if !memoryCanDriveRetrieval(entry) {
		entry.RetrievalEligible = false
	}
	entry.Title = promptsecrecy.Redact(strings.TrimSpace(entry.Title))
	entry.Summary = promptsecrecy.Redact(strings.TrimSpace(entry.Summary))
	entry.Content = promptsecrecy.Redact(strings.TrimSpace(entry.Content))
	entry.ContentExcerpt = promptsecrecy.Redact(strings.TrimSpace(entry.ContentExcerpt))
	entry.RetrievalReason = promptsecrecy.Redact(strings.TrimSpace(entry.RetrievalReason))
	now := time.Now().UTC().Format(time.RFC3339)
	if entry.CreatedAt == "" {
		entry.CreatedAt = now
	}
	if entry.UpdatedAt == "" {
		entry.UpdatedAt = now
	}
	return entry, nil
}

func normalizeMemoryCandidate(candidate MemoryCandidate) (MemoryCandidate, error) {
	candidate.CandidateID = strings.TrimSpace(candidate.CandidateID)
	candidate.CandidateType = strings.TrimSpace(candidate.CandidateType)
	candidate.Status = strings.TrimSpace(candidate.Status)
	candidate.SourceType = strings.TrimSpace(candidate.SourceType)
	candidate.SourceID = strings.TrimSpace(candidate.SourceID)
	candidate.AuthorityRank = strings.TrimSpace(candidate.AuthorityRank)
	candidate.ConsolidationRunID = strings.TrimSpace(candidate.ConsolidationRunID)
	if candidate.CandidateID == "" {
		return candidate, errors.New("memory_candidate_missing_id")
	}
	if !isAllowedCandidateType(candidate.CandidateType) {
		return candidate, errors.New("memory_candidate_invalid_type")
	}
	if !isAllowedCandidateStatus(candidate.Status) {
		return candidate, errors.New("memory_candidate_invalid_status")
	}
	if !isAllowedMemorySourceType(candidate.SourceType) {
		return candidate, errors.New("memory_candidate_invalid_source_type")
	}
	if candidate.TrustTier < 0 {
		return candidate, errors.New("memory_candidate_invalid_trust_tier")
	}
	if candidate.AuthorityRank == "" {
		return candidate, errors.New("memory_candidate_missing_authority_rank")
	}
	if len(candidate.Provenance) == 0 {
		return candidate, errors.New("memory_candidate_missing_provenance")
	}
	if !isAllowedFreshnessState(candidate.FreshnessState) {
		return candidate, errors.New("memory_candidate_invalid_freshness_state")
	}
	if candidate.Confidence < 0 || candidate.Confidence > 1 {
		return candidate, errors.New("memory_candidate_invalid_confidence")
	}
	if !isAllowedContradictionState(candidate.ContradictionState) {
		return candidate, errors.New("memory_candidate_invalid_contradiction_state")
	}
	if !isAllowedMemoryQuarantineStatus(candidate.QuarantineStatus) {
		return candidate, errors.New("memory_candidate_invalid_quarantine_status")
	}
	if !isAllowedSuppressionStatus(candidate.SuppressionStatus) {
		return candidate, errors.New("memory_candidate_invalid_suppression_status")
	}
	candidate.Title = promptsecrecy.Redact(strings.TrimSpace(candidate.Title))
	candidate.Summary = promptsecrecy.Redact(strings.TrimSpace(candidate.Summary))
	candidate.Content = promptsecrecy.Redact(strings.TrimSpace(candidate.Content))
	now := time.Now().UTC().Format(time.RFC3339)
	if candidate.CreatedAt == "" {
		candidate.CreatedAt = now
	}
	if candidate.UpdatedAt == "" {
		candidate.UpdatedAt = now
	}
	return candidate, nil
}

func normalizeMemoryEvidenceRef(ref MemoryEvidenceRef) (MemoryEvidenceRef, error) {
	ref.EvidenceID = strings.TrimSpace(ref.EvidenceID)
	ref.MemoryID = strings.TrimSpace(ref.MemoryID)
	ref.CandidateID = strings.TrimSpace(ref.CandidateID)
	ref.SourceType = strings.TrimSpace(ref.SourceType)
	ref.SourceID = strings.TrimSpace(ref.SourceID)
	ref.SourceRef = strings.TrimSpace(ref.SourceRef)
	ref.EvidenceKind = strings.TrimSpace(ref.EvidenceKind)
	ref.AuthorityRank = strings.TrimSpace(ref.AuthorityRank)
	if ref.EvidenceID == "" {
		return ref, errors.New("memory_evidence_missing_id")
	}
	if ref.MemoryID == "" && ref.CandidateID == "" {
		return ref, errors.New("memory_evidence_missing_subject")
	}
	if !isAllowedMemorySourceType(ref.SourceType) {
		return ref, errors.New("memory_evidence_invalid_source_type")
	}
	if !isAllowedEvidenceKind(ref.EvidenceKind) {
		return ref, errors.New("memory_evidence_invalid_kind")
	}
	if ref.TrustTier < 0 {
		return ref, errors.New("memory_evidence_invalid_trust_tier")
	}
	if ref.AuthorityRank == "" {
		return ref, errors.New("memory_evidence_missing_authority_rank")
	}
	if promptsecrecy.Classify(ref.SourceRef).Forbidden() {
		if ref.DisplayAllowed || ref.PromotionAllowed {
			return ref, errors.New("memory_evidence_secret_requires_blocked_flags")
		}
		ref.SourceRef = promptsecrecy.Redact(ref.SourceRef)
		if ref.RedactionStatus == "" {
			ref.RedactionStatus = "redacted"
		}
	}
	if ref.Timestamp == "" {
		ref.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if ref.CreatedAt == "" {
		ref.CreatedAt = ref.Timestamp
	}
	ref.SourceRef = promptsecrecy.Redact(ref.SourceRef)
	return ref, nil
}

func normalizeMemoryActionAudit(audit MemoryActionAudit) (MemoryActionAudit, error) {
	audit.AuditID = strings.TrimSpace(audit.AuditID)
	audit.MemoryID = strings.TrimSpace(audit.MemoryID)
	audit.CandidateID = strings.TrimSpace(audit.CandidateID)
	audit.Action = strings.TrimSpace(audit.Action)
	audit.Actor = strings.TrimSpace(audit.Actor)
	audit.Reason = promptsecrecy.Redact(strings.TrimSpace(audit.Reason))
	audit.RollbackRef = strings.TrimSpace(audit.RollbackRef)
	audit.SourceRequestID = strings.TrimSpace(audit.SourceRequestID)
	if audit.AuditID == "" {
		return audit, errors.New("memory_audit_missing_id")
	}
	if audit.MemoryID == "" && audit.CandidateID == "" {
		return audit, errors.New("memory_audit_missing_subject")
	}
	if !isAllowedMemoryAuditAction(audit.Action) {
		return audit, errors.New("memory_audit_invalid_action")
	}
	if audit.Actor == "" {
		return audit, errors.New("memory_audit_missing_actor")
	}
	if audit.Timestamp == "" {
		audit.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	return audit, nil
}

func normalizeMemoryFinding(finding MemoryFinding) (MemoryFinding, error) {
	finding.FindingID = strings.TrimSpace(finding.FindingID)
	finding.FindingType = strings.TrimSpace(finding.FindingType)
	finding.Status = strings.TrimSpace(finding.Status)
	finding.Resolver = strings.TrimSpace(finding.Resolver)
	finding.RecommendedAction = promptsecrecy.Redact(strings.TrimSpace(finding.RecommendedAction))
	if finding.FindingID == "" {
		return finding, errors.New("memory_finding_missing_id")
	}
	if !isAllowedFindingType(finding.FindingType) {
		return finding, errors.New("memory_finding_invalid_type")
	}
	if finding.Confidence < 0 || finding.Confidence > 1 {
		return finding, errors.New("memory_finding_invalid_confidence")
	}
	if !isAllowedFindingStatus(finding.Status) {
		return finding, errors.New("memory_finding_invalid_status")
	}
	if (finding.Status == "resolved" || finding.Status == "dismissed") && (finding.Resolver == "" || len(finding.AuditEventIDs) == 0) {
		return finding, errors.New("memory_finding_resolution_requires_resolver_and_audit")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if finding.CreatedAt == "" {
		finding.CreatedAt = now
	}
	if finding.UpdatedAt == "" {
		finding.UpdatedAt = now
	}
	if (finding.Status == "resolved" || finding.Status == "dismissed") && finding.ResolvedAt == "" {
		finding.ResolvedAt = finding.UpdatedAt
	}
	return finding, nil
}

func scanMemoryEntry(scanner retrievalScanner) (*MemoryEntry, error) {
	var entry MemoryEntry
	var provenanceJSON, evidenceRefsJSON, allowedActionsJSON, auditEventIDsJSON, gateResultJSON string
	var rollbackAvailable, externalContentIsNotInstruction, retrievalEligible, userVisible int
	if err := scanner.Scan(&entry.MemoryID, &entry.MemoryClass, &entry.Status, &entry.Title, &entry.Summary, &entry.Content, &entry.ContentExcerpt,
		&entry.SourceType, &entry.SourceID, &entry.TrustTier, &entry.AuthorityRank, &provenanceJSON, &evidenceRefsJSON, &entry.FreshnessState,
		&entry.Confidence, &entry.ContradictionState, &entry.QuarantineStatus, &entry.SuppressionStatus, &entry.StaleAfter, &entry.ExpiresAt,
		&entry.CreatedAt, &entry.UpdatedAt, &entry.LastVerifiedAt, &allowedActionsJSON, &auditEventIDsJSON, &entry.SupersedesMemoryID,
		&rollbackAvailable, &externalContentIsNotInstruction, &retrievalEligible, &entry.RetrievalReason, &entry.EmbeddingStatus,
		&entry.CandidateOriginRunID, &gateResultJSON, &userVisible); err != nil {
		return nil, err
	}
	var err error
	if entry.Provenance, err = decodeStringMap(provenanceJSON); err != nil {
		return nil, err
	}
	if entry.PromotionGateResult, err = decodeStringMap(gateResultJSON); err != nil {
		return nil, err
	}
	if entry.EvidenceRefs, err = decodeStringList(evidenceRefsJSON); err != nil {
		return nil, err
	}
	if entry.AllowedActions, err = decodeStringList(allowedActionsJSON); err != nil {
		return nil, err
	}
	if entry.AuditEventIDs, err = decodeStringList(auditEventIDsJSON); err != nil {
		return nil, err
	}
	entry.RollbackAvailable = rollbackAvailable == 1
	entry.ExternalContentIsNotInstruction = externalContentIsNotInstruction == 1
	entry.RetrievalEligible = retrievalEligible == 1
	entry.UserVisible = userVisible == 1
	return &entry, nil
}

func scanMemoryCandidate(scanner retrievalScanner) (*MemoryCandidate, error) {
	var candidate MemoryCandidate
	var provenanceJSON, evidenceRefsJSON, gateResultJSON string
	if err := scanner.Scan(&candidate.CandidateID, &candidate.CandidateType, &candidate.Status, &candidate.Title, &candidate.Summary,
		&candidate.Content, &candidate.SourceType, &candidate.SourceID, &candidate.TrustTier, &candidate.AuthorityRank, &provenanceJSON,
		&evidenceRefsJSON, &candidate.FreshnessState, &candidate.Confidence, &candidate.ContradictionState, &candidate.QuarantineStatus,
		&candidate.SuppressionStatus, &candidate.ConsolidationRunID, &gateResultJSON, &candidate.CreatedAt, &candidate.UpdatedAt,
		&candidate.ExpiresAt); err != nil {
		return nil, err
	}
	var err error
	if candidate.Provenance, err = decodeStringMap(provenanceJSON); err != nil {
		return nil, err
	}
	if candidate.PromotionGateResult, err = decodeStringMap(gateResultJSON); err != nil {
		return nil, err
	}
	if candidate.EvidenceRefs, err = decodeStringList(evidenceRefsJSON); err != nil {
		return nil, err
	}
	return &candidate, nil
}

func scanMemoryEvidenceRef(scanner retrievalScanner) (*MemoryEvidenceRef, error) {
	var ref MemoryEvidenceRef
	var displayAllowed, promotionAllowed int
	if err := scanner.Scan(&ref.EvidenceID, &ref.MemoryID, &ref.CandidateID, &ref.SourceType, &ref.SourceID, &ref.SourceRef,
		&ref.EvidenceKind, &ref.TrustTier, &ref.AuthorityRank, &ref.Timestamp, &ref.ContentHash, &ref.RedactionStatus,
		&displayAllowed, &promotionAllowed, &ref.CreatedAt); err != nil {
		return nil, err
	}
	ref.DisplayAllowed = displayAllowed == 1
	ref.PromotionAllowed = promotionAllowed == 1
	return &ref, nil
}

func scanMemoryActionAudit(scanner retrievalScanner) (*MemoryActionAudit, error) {
	var audit MemoryActionAudit
	var beforeStateJSON, afterStateJSON, gateResultJSON string
	var userConfirmationRequired int
	if err := scanner.Scan(&audit.AuditID, &audit.MemoryID, &audit.CandidateID, &audit.Action, &audit.Actor,
		&audit.Reason, &beforeStateJSON, &afterStateJSON, &audit.Timestamp, &audit.RollbackRef, &gateResultJSON,
		&userConfirmationRequired, &audit.SourceRequestID); err != nil {
		return nil, err
	}
	var err error
	if audit.BeforeState, err = decodeStringMap(beforeStateJSON); err != nil {
		return nil, err
	}
	if audit.AfterState, err = decodeStringMap(afterStateJSON); err != nil {
		return nil, err
	}
	if audit.GateResult, err = decodeStringMap(gateResultJSON); err != nil {
		return nil, err
	}
	audit.UserConfirmationRequired = userConfirmationRequired == 1
	return &audit, nil
}

func encodeSafeJSON(value map[string]any) (string, error) {
	if value == nil {
		value = map[string]any{}
	}
	raw, err := json.Marshal(sanitizeRuntimeValue(value))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func memoryCanDriveRetrieval(entry MemoryEntry) bool {
	if entry.Status != "active" {
		return false
	}
	if entry.FreshnessState == "stale" || entry.FreshnessState == "expired" {
		return false
	}
	if entry.ContradictionState == "confirmed" {
		return false
	}
	if entry.QuarantineStatus != "clean" {
		return false
	}
	if entry.SuppressionStatus != "active" {
		return false
	}
	return true
}

func userCorrectionCanSupersede(memoryClass string) bool {
	switch memoryClass {
	case "durable_user_preference", "user_profile_context_fact", "task_continuity":
		return true
	default:
		return false
	}
}

func protectedTruthMutation(parts ...string) bool {
	content := strings.ToLower(strings.Join(parts, "\n"))
	return containsAny(content, []string{
		"rewrite canon",
		"change canon",
		"override canon",
		"runtime truth is",
		"provider config",
		"provider healthy",
		"byok active",
		"hosted active",
		"policy is now",
		"safety rule",
		"main hub authority",
	})
}

func containsAny(content string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(content, needle) {
			return true
		}
	}
	return false
}

func isCandidateMemoryClass(value string) bool {
	switch value {
	case "memory_candidate", "procedure_candidate", "retrieval_anchor_candidate", "contradiction_staleness_finding", "improvement_proposal":
		return true
	default:
		return false
	}
}

func isAllowedMemoryClass(value string) bool {
	switch value {
	case "canon_memory", "durable_user_preference", "user_profile_context_fact", "task_continuity", "approved_summary", "runtime_observation", "episodic_history", "scratch_transient", "quarantined_suppressed", "memory_candidate", "procedure_candidate", "retrieval_anchor_candidate", "contradiction_staleness_finding", "improvement_proposal":
		return true
	default:
		return false
	}
}

func isAllowedMemoryEntryStatus(value string) bool {
	switch value {
	case "candidate", "active", "needs_confirmation", "quarantined", "suppressed", "stale", "superseded", "deleted_by_user", "rejected":
		return true
	default:
		return false
	}
}

func isAllowedMemorySourceType(value string) bool {
	switch value {
	case "canon", "main_hub", "verified_runtime", "direct_user", "model_output", "archive", "external", "retrieval", "skill", "unknown":
		return true
	default:
		return false
	}
}

func isAllowedFreshnessState(value string) bool {
	switch value {
	case "fresh", "aging", "stale", "expired", "unknown":
		return true
	default:
		return false
	}
}

func isAllowedContradictionState(value string) bool {
	switch value {
	case "none", "suspected", "confirmed", "resolved":
		return true
	default:
		return false
	}
}

func isAllowedMemoryQuarantineStatus(value string) bool {
	switch value {
	case "clean", "quarantined", "blocked", "unknown":
		return true
	default:
		return false
	}
}

func isAllowedSuppressionStatus(value string) bool {
	switch value {
	case "active", "suppressed", "demoted", "rolled_back":
		return true
	default:
		return false
	}
}

func isAllowedEmbeddingStatus(value string) bool {
	switch value {
	case "not_needed", "pending", "ready", "failed", "blocked":
		return true
	default:
		return false
	}
}

func isAllowedCandidateType(value string) bool {
	switch value {
	case "memory_candidate", "procedure_candidate", "retrieval_anchor_candidate", "contradiction_staleness_finding", "improvement_proposal":
		return true
	default:
		return false
	}
}

func isAllowedCandidateStatus(value string) bool {
	switch value {
	case "generated", "needs_review", "approved", "rejected", "quarantined", "suppressed", "promoted", "expired":
		return true
	default:
		return false
	}
}

func isAllowedEvidenceKind(value string) bool {
	switch value {
	case "user_statement", "runtime_state", "tool_result", "app_bridge_evidence", "task_completion_evidence", "canon_ref", "archive_ref", "external_ref":
		return true
	default:
		return false
	}
}

func isAllowedMemoryAuditAction(value string) bool {
	switch value {
	case "view", "suppress", "restore_suppression", "delete_user_remove", "correct_supersede", "mark_stale", "view_provenance", "rollback", "approve_candidate", "reject_candidate", "quarantine", "unquarantine":
		return true
	default:
		return false
	}
}

func isAllowedFindingType(value string) bool {
	switch value {
	case "contradiction", "stale", "unsupported", "poisoning", "missing_evidence", "authority_conflict":
		return true
	default:
		return false
	}
}

func isAllowedFindingStatus(value string) bool {
	switch value {
	case "open", "needs_review", "resolved", "dismissed", "superseded":
		return true
	default:
		return false
	}
}
