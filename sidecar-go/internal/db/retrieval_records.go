package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	importsafety "xmilo/sidecar-go/internal/imports"
	"xmilo/sidecar-go/internal/promptsecrecy"
)

type RetrievalSourceType string

const (
	RetrievalSourceCanon        RetrievalSourceType = "canon"
	RetrievalSourceRuntimeState RetrievalSourceType = "runtime_state"
	RetrievalSourceMemory       RetrievalSourceType = "memory"
	RetrievalSourceArchive      RetrievalSourceType = "archive"
	RetrievalSourceExternal     RetrievalSourceType = "external"
	RetrievalSourceSkill        RetrievalSourceType = "skill"
	RetrievalSourcePlugin       RetrievalSourceType = "plugin"
	RetrievalSourceUserFile     RetrievalSourceType = "user_file"
	RetrievalSourceUnknown      RetrievalSourceType = "unknown"
)

type RetrievalQuarantineStatus string

const (
	RetrievalQuarantineClean       RetrievalQuarantineStatus = "clean"
	RetrievalQuarantineQuarantined RetrievalQuarantineStatus = "quarantined"
	RetrievalQuarantineBlocked     RetrievalQuarantineStatus = "blocked"
	RetrievalQuarantineUnknown     RetrievalQuarantineStatus = "unknown"
)

type RetrievalRecord struct {
	ChunkID                     string
	SourceID                    string
	SourceType                  RetrievalSourceType
	TrustTier                   int
	AuthorityRank               string
	Provenance                  map[string]any
	CreatedAt                   string
	UpdatedAt                   string
	ExpiresAt                   string
	Freshness                   string
	Hash                        string
	QuarantineStatus            RetrievalQuarantineStatus
	ContainsExternalInstruction bool
	ContainsSecret              bool
	EmbeddingModel              string
	EmbeddingVersion            string
	ContentSummary              string
	RawContentRef               string
	Embedding                   []float64
	Confidence                  float64
	ContradictionState          string
	EvidenceRefs                []string
	SuppressionStatus           string
	StaleAfter                  string
	LastVerifiedAt              string
	RetrievalReason             string
	RetrievalScore              float64
	RetrievalBackend            string
	UsedVector                  bool
	UsedLexical                 bool
	FallbackReason              string
	PackPosition                int
	TokenEstimate               int
}

func (s *Store) UpsertRetrievalRecord(record RetrievalRecord) error {
	normalized, err := normalizeRetrievalRecord(record)
	if err != nil {
		return err
	}
	provenance, err := encodeStringMap(normalized.Provenance)
	if err != nil {
		return err
	}
	embedding, err := json.Marshal(normalized.Embedding)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`
        INSERT INTO retrieval_records(
            chunk_id, source_id, source_type, trust_tier, authority_rank, provenance_json,
            created_at, updated_at, expires_at, freshness, content_hash, quarantine_status,
            contains_external_instruction, contains_secret, embedding_model, embedding_version,
            content_summary, raw_content_ref, embedding_json, confidence, contradiction_state,
            evidence_refs_json, suppression_status, stale_after, last_verified_at, retrieval_reason,
            retrieval_score, retrieval_backend, used_vector, used_lexical, fallback_reason, pack_position,
            token_estimate
        )
        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(chunk_id) DO UPDATE SET
            source_id = excluded.source_id,
            source_type = excluded.source_type,
            trust_tier = excluded.trust_tier,
            authority_rank = excluded.authority_rank,
            provenance_json = excluded.provenance_json,
            updated_at = excluded.updated_at,
            expires_at = excluded.expires_at,
            freshness = excluded.freshness,
            content_hash = excluded.content_hash,
            quarantine_status = excluded.quarantine_status,
            contains_external_instruction = excluded.contains_external_instruction,
            contains_secret = excluded.contains_secret,
            embedding_model = excluded.embedding_model,
            embedding_version = excluded.embedding_version,
            content_summary = excluded.content_summary,
            raw_content_ref = excluded.raw_content_ref,
            embedding_json = excluded.embedding_json,
            confidence = excluded.confidence,
            contradiction_state = excluded.contradiction_state,
            evidence_refs_json = excluded.evidence_refs_json,
            suppression_status = excluded.suppression_status,
            stale_after = excluded.stale_after,
            last_verified_at = excluded.last_verified_at,
            retrieval_reason = excluded.retrieval_reason,
            retrieval_score = excluded.retrieval_score,
            retrieval_backend = excluded.retrieval_backend,
            used_vector = excluded.used_vector,
            used_lexical = excluded.used_lexical,
            fallback_reason = excluded.fallback_reason,
            pack_position = excluded.pack_position,
            token_estimate = excluded.token_estimate
    `, normalized.ChunkID, normalized.SourceID, normalized.SourceType, normalized.TrustTier, normalized.AuthorityRank, provenance,
		normalized.CreatedAt, normalized.UpdatedAt, nullableString(normalized.ExpiresAt), normalized.Freshness, normalized.Hash,
		normalized.QuarantineStatus, boolToInt(normalized.ContainsExternalInstruction), boolToInt(normalized.ContainsSecret),
		normalized.EmbeddingModel, normalized.EmbeddingVersion, normalized.ContentSummary, normalized.RawContentRef, string(embedding),
		normalized.Confidence, normalized.ContradictionState, mustEncodeStringList(normalized.EvidenceRefs), normalized.SuppressionStatus,
		nullableString(normalized.StaleAfter), nullableString(normalized.LastVerifiedAt), normalized.RetrievalReason, normalized.RetrievalScore,
		normalized.RetrievalBackend, boolToInt(normalized.UsedVector), boolToInt(normalized.UsedLexical), normalized.FallbackReason,
		normalized.PackPosition, normalized.TokenEstimate)
	return err
}

func (s *Store) GetRetrievalRecord(chunkID string) (*RetrievalRecord, error) {
	chunkID = strings.TrimSpace(chunkID)
	if chunkID == "" {
		return nil, errors.New("retrieval_record_missing_chunk_id")
	}
	row := s.DB.QueryRow(`
        SELECT chunk_id, source_id, source_type, trust_tier, authority_rank, provenance_json,
            created_at, updated_at, COALESCE(expires_at, ''), freshness, content_hash, quarantine_status,
            contains_external_instruction, contains_secret, embedding_model, embedding_version,
            content_summary, raw_content_ref, embedding_json, confidence, contradiction_state,
            evidence_refs_json, suppression_status, COALESCE(stale_after, ''), COALESCE(last_verified_at, ''),
            retrieval_reason, retrieval_score, retrieval_backend, used_vector, used_lexical, fallback_reason,
            pack_position, token_estimate
        FROM retrieval_records WHERE chunk_id = ?
    `, chunkID)
	record, err := scanRetrievalRecord(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return record, err
}

func (s *Store) ListRetrievalRecords() ([]RetrievalRecord, error) {
	rows, err := s.DB.Query(`
        SELECT chunk_id, source_id, source_type, trust_tier, authority_rank, provenance_json,
            created_at, updated_at, COALESCE(expires_at, ''), freshness, content_hash, quarantine_status,
            contains_external_instruction, contains_secret, embedding_model, embedding_version,
            content_summary, raw_content_ref, embedding_json, confidence, contradiction_state,
            evidence_refs_json, suppression_status, COALESCE(stale_after, ''), COALESCE(last_verified_at, ''),
            retrieval_reason, retrieval_score, retrieval_backend, used_vector, used_lexical, fallback_reason,
            pack_position, token_estimate
        FROM retrieval_records ORDER BY authority_rank ASC, trust_tier ASC, chunk_id ASC
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []RetrievalRecord
	for rows.Next() {
		record, err := scanRetrievalRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, *record)
	}
	return records, rows.Err()
}

func (s *Store) InvalidateRetrievalRecordsBySource(sourceID string) error {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		return errors.New("retrieval_record_missing_source_id")
	}
	_, err := s.DB.Exec(`
        UPDATE retrieval_records
        SET quarantine_status = 'blocked', freshness = 'invalidated', updated_at = ?
        WHERE source_id = ?
    `, time.Now().UTC().Format(time.RFC3339), sourceID)
	return err
}

func RetrievalRecordEligibleForTrustedSelection(record RetrievalRecord, now time.Time) bool {
	if record.QuarantineStatus != RetrievalQuarantineClean {
		return false
	}
	if record.ContainsSecret || record.ContainsExternalInstruction {
		return false
	}
	if strings.TrimSpace(record.ProvenanceString()) == "" {
		return false
	}
	if record.ExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339, record.ExpiresAt)
		if err != nil || !now.IsZero() && !expiresAt.After(now) {
			return false
		}
	}
	return true
}

func (r RetrievalRecord) ProvenanceString() string {
	if len(r.Provenance) == 0 {
		return ""
	}
	raw, err := json.Marshal(r.Provenance)
	if err != nil {
		return ""
	}
	return string(raw)
}

type retrievalScanner interface {
	Scan(dest ...any) error
}

func scanRetrievalRecord(scanner retrievalScanner) (*RetrievalRecord, error) {
	var record RetrievalRecord
	var provenance, embedding, evidenceRefs string
	var containsExternalInstruction, containsSecret, usedVector, usedLexical int
	if err := scanner.Scan(&record.ChunkID, &record.SourceID, &record.SourceType, &record.TrustTier, &record.AuthorityRank, &provenance,
		&record.CreatedAt, &record.UpdatedAt, &record.ExpiresAt, &record.Freshness, &record.Hash, &record.QuarantineStatus,
		&containsExternalInstruction, &containsSecret, &record.EmbeddingModel, &record.EmbeddingVersion,
		&record.ContentSummary, &record.RawContentRef, &embedding, &record.Confidence, &record.ContradictionState,
		&evidenceRefs, &record.SuppressionStatus, &record.StaleAfter, &record.LastVerifiedAt, &record.RetrievalReason,
		&record.RetrievalScore, &record.RetrievalBackend, &usedVector, &usedLexical, &record.FallbackReason,
		&record.PackPosition, &record.TokenEstimate); err != nil {
		return nil, err
	}
	var err error
	if record.Provenance, err = decodeStringMap(provenance); err != nil {
		return nil, err
	}
	if strings.TrimSpace(embedding) != "" {
		if err := json.Unmarshal([]byte(embedding), &record.Embedding); err != nil {
			return nil, err
		}
	}
	if record.EvidenceRefs, err = decodeStringList(evidenceRefs); err != nil {
		return nil, err
	}
	record.ContainsExternalInstruction = containsExternalInstruction == 1
	record.ContainsSecret = containsSecret == 1
	record.UsedVector = usedVector == 1
	record.UsedLexical = usedLexical == 1
	return &record, nil
}

func normalizeRetrievalRecord(record RetrievalRecord) (RetrievalRecord, error) {
	record.ChunkID = strings.TrimSpace(record.ChunkID)
	record.SourceID = strings.TrimSpace(record.SourceID)
	if record.ChunkID == "" {
		return record, errors.New("retrieval_record_missing_chunk_id")
	}
	if record.SourceID == "" {
		return record, errors.New("retrieval_record_missing_source_id")
	}
	if !isAllowedRetrievalSourceType(record.SourceType) {
		return record, fmt.Errorf("unsupported_retrieval_source_type:%s", record.SourceType)
	}
	if record.TrustTier < 0 {
		return record, errors.New("retrieval_record_missing_trust_tier")
	}
	if strings.TrimSpace(record.AuthorityRank) == "" {
		return record, errors.New("retrieval_record_missing_authority_rank")
	}
	if len(record.Provenance) == 0 {
		return record, errors.New("retrieval_record_missing_provenance")
	}
	if !isAllowedRetrievalQuarantineStatus(record.QuarantineStatus) {
		return record, fmt.Errorf("unsupported_retrieval_quarantine_status:%s", record.QuarantineStatus)
	}
	if strings.TrimSpace(record.Hash) == "" {
		return record, errors.New("retrieval_record_missing_hash")
	}
	if importsafety.ContainsSecretRisk(record.ContentSummary, record.RawContentRef, record.EmbeddingModel, record.EmbeddingVersion) {
		return record, errors.New("retrieval_record_secret_metadata")
	}
	if promptsecrecy.Classify(record.ContentSummary).Forbidden() {
		return record, errors.New("retrieval_record_prompt_secrecy_metadata")
	}
	if record.Freshness == "" {
		record.Freshness = "unknown"
	}
	if record.ContradictionState == "" {
		record.ContradictionState = "none"
	}
	if !isAllowedContradictionState(record.ContradictionState) {
		return record, errors.New("retrieval_record_invalid_contradiction_state")
	}
	if record.SuppressionStatus == "" {
		record.SuppressionStatus = "active"
	}
	if !isAllowedSuppressionStatus(record.SuppressionStatus) {
		return record, errors.New("retrieval_record_invalid_suppression_status")
	}
	if record.Confidence < 0 || record.Confidence > 1 {
		return record, errors.New("retrieval_record_invalid_confidence")
	}
	if record.RetrievalBackend == "" {
		record.RetrievalBackend = "lexical"
	}
	if !record.UsedVector && !record.UsedLexical {
		record.UsedLexical = true
	}
	if record.PackPosition < 0 || record.TokenEstimate < 0 {
		return record, errors.New("retrieval_record_invalid_metadata_count")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if record.CreatedAt == "" {
		record.CreatedAt = now
	}
	if record.UpdatedAt == "" {
		record.UpdatedAt = record.CreatedAt
	}
	return record, nil
}

func mustEncodeStringList(values []string) string {
	raw, err := encodeStringList(values)
	if err != nil {
		return "[]"
	}
	return raw
}

func isAllowedRetrievalSourceType(sourceType RetrievalSourceType) bool {
	switch sourceType {
	case RetrievalSourceCanon, RetrievalSourceRuntimeState, RetrievalSourceMemory, RetrievalSourceArchive, RetrievalSourceExternal, RetrievalSourceSkill, RetrievalSourcePlugin, RetrievalSourceUserFile, RetrievalSourceUnknown:
		return true
	default:
		return false
	}
}

func isAllowedRetrievalQuarantineStatus(status RetrievalQuarantineStatus) bool {
	switch status {
	case RetrievalQuarantineClean, RetrievalQuarantineQuarantined, RetrievalQuarantineBlocked, RetrievalQuarantineUnknown:
		return true
	default:
		return false
	}
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
