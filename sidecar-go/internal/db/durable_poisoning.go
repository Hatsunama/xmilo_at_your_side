package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"xmilo/sidecar-go/internal/poisoning"
)

type DurablePoisoningRecord struct {
	RecordKey          string
	RecordKind         string
	SourceID           string
	SourceType         string
	TrustTier          int
	AuthorityRank      string
	SourceHash         string
	ExpectedSourceHash string
	ProvenanceChain    []poisoning.ProvenanceNode
	PoisoningFindings  []poisoning.Finding
	QuarantineStatus   poisoning.QuarantineStatus
	Stale              bool
	Conflict           bool
	TestFixture        bool
	CreatedAt          string
	UpdatedAt          string
	ExpiresAt          string
}

func (s *Store) UpsertDurablePoisoningRecord(record DurablePoisoningRecord) error {
	normalized, err := normalizeDurablePoisoningRecord(record)
	if err != nil {
		return err
	}
	provenanceJSON, err := json.Marshal(normalized.ProvenanceChain)
	if err != nil {
		return err
	}
	findingsJSON, err := json.Marshal(normalized.PoisoningFindings)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`
        INSERT INTO durable_poisoning_records(
            record_key, record_kind, source_id, source_type, trust_tier, authority_rank,
            source_hash, expected_source_hash, provenance_chain_json, poisoning_findings_json,
            quarantine_status, stale, conflict, test_fixture, created_at, updated_at, expires_at
        )
        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(record_key) DO UPDATE SET
            record_kind = excluded.record_kind,
            source_id = excluded.source_id,
            source_type = excluded.source_type,
            trust_tier = excluded.trust_tier,
            authority_rank = excluded.authority_rank,
            source_hash = excluded.source_hash,
            expected_source_hash = excluded.expected_source_hash,
            provenance_chain_json = excluded.provenance_chain_json,
            poisoning_findings_json = excluded.poisoning_findings_json,
            quarantine_status = excluded.quarantine_status,
            stale = excluded.stale,
            conflict = excluded.conflict,
            test_fixture = excluded.test_fixture,
            updated_at = excluded.updated_at,
            expires_at = excluded.expires_at
    `, normalized.RecordKey, normalized.RecordKind, normalized.SourceID, normalized.SourceType, normalized.TrustTier, normalized.AuthorityRank,
		normalized.SourceHash, normalized.ExpectedSourceHash, string(provenanceJSON), string(findingsJSON), normalized.QuarantineStatus,
		boolToInt(normalized.Stale), boolToInt(normalized.Conflict), boolToInt(normalized.TestFixture), normalized.CreatedAt, normalized.UpdatedAt, nullableString(normalized.ExpiresAt))
	return err
}

func (s *Store) GetDurablePoisoningRecord(recordKey string) (*DurablePoisoningRecord, error) {
	recordKey = strings.TrimSpace(recordKey)
	if recordKey == "" {
		return nil, errors.New("durable_poisoning_record_missing_key")
	}
	row := s.DB.QueryRow(`
        SELECT record_key, record_kind, source_id, source_type, trust_tier, authority_rank,
            source_hash, expected_source_hash, provenance_chain_json, poisoning_findings_json,
            quarantine_status, stale, conflict, test_fixture, created_at, updated_at, COALESCE(expires_at, '')
        FROM durable_poisoning_records WHERE record_key = ?
    `, recordKey)
	var record DurablePoisoningRecord
	var provenanceJSON, findingsJSON string
	var stale, conflict, testFixture int
	if err := row.Scan(&record.RecordKey, &record.RecordKind, &record.SourceID, &record.SourceType, &record.TrustTier, &record.AuthorityRank,
		&record.SourceHash, &record.ExpectedSourceHash, &provenanceJSON, &findingsJSON, &record.QuarantineStatus,
		&stale, &conflict, &testFixture, &record.CreatedAt, &record.UpdatedAt, &record.ExpiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(provenanceJSON), &record.ProvenanceChain); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(findingsJSON), &record.PoisoningFindings); err != nil {
		return nil, err
	}
	record.Stale = stale == 1
	record.Conflict = conflict == 1
	record.TestFixture = testFixture == 1
	return &record, nil
}

func DurablePoisoningRecordFromAssessment(candidate poisoning.Candidate, assessment poisoning.Assessment) DurablePoisoningRecord {
	createdAt := candidate.Now
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	trustTier := assessment.EffectiveTrustTier
	if candidate.TrustTier != nil {
		trustTier = *candidate.TrustTier
	}
	return DurablePoisoningRecord{
		RecordKey:          candidate.RecordKey,
		RecordKind:         candidate.RecordKind,
		SourceID:           candidate.SourceID,
		SourceType:         candidate.SourceType,
		TrustTier:          trustTier,
		AuthorityRank:      candidate.AuthorityRank,
		SourceHash:         candidate.SourceHash,
		ExpectedSourceHash: candidate.ExpectedSourceHash,
		ProvenanceChain:    candidate.ProvenanceChain,
		PoisoningFindings:  assessment.Findings,
		QuarantineStatus:   assessment.Status,
		Stale:              assessment.Stale,
		Conflict:           assessment.Conflict,
		TestFixture:        candidate.TestFixture,
		CreatedAt:          createdAt.Format(time.RFC3339),
		UpdatedAt:          createdAt.Format(time.RFC3339),
		ExpiresAt:          candidate.ExpiresAt,
	}
}

func normalizeDurablePoisoningRecord(record DurablePoisoningRecord) (DurablePoisoningRecord, error) {
	record.RecordKey = strings.TrimSpace(record.RecordKey)
	record.RecordKind = strings.TrimSpace(record.RecordKind)
	record.SourceID = strings.TrimSpace(record.SourceID)
	record.SourceType = strings.TrimSpace(record.SourceType)
	record.AuthorityRank = strings.TrimSpace(record.AuthorityRank)
	if record.RecordKey == "" {
		return record, errors.New("durable_poisoning_record_missing_key")
	}
	if record.RecordKind == "" {
		return record, errors.New("durable_poisoning_record_missing_kind")
	}
	if record.SourceID == "" || record.SourceType == "" {
		return record, errors.New("durable_poisoning_record_missing_source")
	}
	if record.TrustTier < 0 {
		return record, errors.New("durable_poisoning_record_invalid_trust_tier")
	}
	if record.AuthorityRank == "" {
		return record, errors.New("durable_poisoning_record_missing_authority_rank")
	}
	if err := poisoning.ValidateProvenanceChain(record.ProvenanceChain); err != nil {
		return record, err
	}
	switch record.QuarantineStatus {
	case poisoning.QuarantineClean, poisoning.QuarantineQuarantined, poisoning.QuarantineBlocked:
	default:
		return record, errors.New("durable_poisoning_record_invalid_quarantine_status")
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
