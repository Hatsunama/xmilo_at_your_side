package db

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRetrievalRecordCreateReadPreservesTrustProvenanceAndRank(t *testing.T) {
	store := openRetrievalStore(t)
	record := testRetrievalRecord("canon.rules.1", RetrievalSourceCanon, "canon", 0, "rank_000_canon")
	record.Embedding = []float64{0.1, 0.2, 0.3}
	if err := store.UpsertRetrievalRecord(record); err != nil {
		t.Fatalf("upsert retrieval record: %v", err)
	}
	loaded, err := store.GetRetrievalRecord("canon.rules.1")
	if err != nil {
		t.Fatalf("get retrieval record: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected retrieval record")
	}
	if loaded.SourceType != RetrievalSourceCanon || loaded.TrustTier != 0 || loaded.AuthorityRank != "rank_000_canon" {
		t.Fatalf("trust/rank did not round-trip: %#v", loaded)
	}
	if loaded.Provenance["source_id"] != "canon" {
		t.Fatalf("provenance did not round-trip: %#v", loaded.Provenance)
	}
	if len(loaded.Embedding) != 3 {
		t.Fatalf("embedding did not round-trip: %#v", loaded.Embedding)
	}
}

func TestRetrievalRecordRejectsMissingTrustProvenanceAndSource(t *testing.T) {
	store := openRetrievalStore(t)
	missingSource := testRetrievalRecord("missing.source", RetrievalSourceType(""), "source", 0, "rank_100_external")
	if err := store.UpsertRetrievalRecord(missingSource); err == nil || err.Error() != "unsupported_retrieval_source_type:" {
		t.Fatalf("expected missing source type error, got %v", err)
	}
	missingProvenance := testRetrievalRecord("missing.provenance", RetrievalSourceExternal, "source", 5, "rank_500_external")
	missingProvenance.Provenance = nil
	if err := store.UpsertRetrievalRecord(missingProvenance); err == nil || err.Error() != "retrieval_record_missing_provenance" {
		t.Fatalf("expected missing provenance error, got %v", err)
	}
	missingTrust := testRetrievalRecord("missing.trust", RetrievalSourceExternal, "source", -1, "rank_500_external")
	if err := store.UpsertRetrievalRecord(missingTrust); err == nil || err.Error() != "retrieval_record_missing_trust_tier" {
		t.Fatalf("expected missing trust error, got %v", err)
	}
}

func TestRetrievalRecordQuarantineAndSecretEligibilityFailClosed(t *testing.T) {
	now := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	quarantined := testRetrievalRecord("external.q", RetrievalSourceExternal, "external", 5, "rank_500_external")
	quarantined.QuarantineStatus = RetrievalQuarantineQuarantined
	if RetrievalRecordEligibleForTrustedSelection(quarantined, now) {
		t.Fatal("quarantined record should not be trusted retrieval eligible")
	}
	secret := testRetrievalRecord("external.secret", RetrievalSourceExternal, "external", 5, "rank_500_external")
	secret.ContainsSecret = true
	if RetrievalRecordEligibleForTrustedSelection(secret, now) {
		t.Fatal("secret-bearing record should not be trusted retrieval eligible")
	}
	expired := testRetrievalRecord("external.expired", RetrievalSourceExternal, "external", 5, "rank_500_external")
	expired.ExpiresAt = now.Add(-time.Minute).Format(time.RFC3339)
	if RetrievalRecordEligibleForTrustedSelection(expired, now) {
		t.Fatal("expired record should not be trusted retrieval eligible")
	}
}

func TestRetrievalRecordPreservesAuthorityRankBeforeSimilarity(t *testing.T) {
	store := openRetrievalStore(t)
	canon := testRetrievalRecord("canon.rank", RetrievalSourceCanon, "canon", 0, "rank_000_canon")
	canon.Embedding = []float64{0.0}
	external := testRetrievalRecord("external.rank", RetrievalSourceExternal, "external", 5, "rank_500_external")
	external.Embedding = []float64{0.99}
	if err := store.UpsertRetrievalRecord(external); err != nil {
		t.Fatalf("upsert external: %v", err)
	}
	if err := store.UpsertRetrievalRecord(canon); err != nil {
		t.Fatalf("upsert canon: %v", err)
	}
	records, err := store.ListRetrievalRecords()
	if err != nil {
		t.Fatalf("list retrieval records: %v", err)
	}
	if len(records) < 2 || records[0].ChunkID != "canon.rank" {
		t.Fatalf("authority rank should sort before similarity/storage order, got %#v", records)
	}
}

func TestRetrievalRecordRejectsSecretMetadata(t *testing.T) {
	store := openRetrievalStore(t)
	record := testRetrievalRecord("secret.metadata", RetrievalSourceExternal, "external", 5, "rank_500_external")
	record.ContentSummary = "contains Authorization: Bearer token"
	if err := store.UpsertRetrievalRecord(record); err == nil || err.Error() != "retrieval_record_secret_metadata" {
		t.Fatalf("expected secret metadata rejection, got %v", err)
	}
}

func TestRetrievalRecordInvalidationBySource(t *testing.T) {
	store := openRetrievalStore(t)
	record := testRetrievalRecord("source.invalidate.1", RetrievalSourceMemory, "memory-source", 2, "rank_200_memory")
	if err := store.UpsertRetrievalRecord(record); err != nil {
		t.Fatalf("upsert retrieval record: %v", err)
	}
	if err := store.InvalidateRetrievalRecordsBySource("memory-source"); err != nil {
		t.Fatalf("invalidate source: %v", err)
	}
	loaded, err := store.GetRetrievalRecord("source.invalidate.1")
	if err != nil {
		t.Fatalf("get retrieval record: %v", err)
	}
	if loaded.QuarantineStatus != RetrievalQuarantineBlocked || loaded.Freshness != "invalidated" {
		t.Fatalf("expected invalidated blocked record, got %#v", loaded)
	}
}

func TestRetrievalRecordMetadataExpansionPreservesExistingFields(t *testing.T) {
	store := openRetrievalStore(t)
	record := testRetrievalRecord("metadata.expanded", RetrievalSourceMemory, "memory-source", 2, "rank_200_memory")
	record.Confidence = 0.72
	record.ContradictionState = "suspected"
	record.EvidenceRefs = []string{"evidence.memory"}
	record.SuppressionStatus = "demoted"
	record.StaleAfter = "2026-07-01T00:00:00Z"
	record.LastVerifiedAt = "2026-06-04T12:00:00Z"
	record.RetrievalReason = "memory recall"
	record.RetrievalScore = 0.44
	record.RetrievalBackend = "lexical"
	record.UsedVector = false
	record.UsedLexical = true
	record.FallbackReason = "vector_unavailable"
	record.PackPosition = 3
	record.TokenEstimate = 42
	if err := store.UpsertRetrievalRecord(record); err != nil {
		t.Fatalf("upsert retrieval record: %v", err)
	}
	loaded, err := store.GetRetrievalRecord(record.ChunkID)
	if err != nil {
		t.Fatalf("get retrieval record: %v", err)
	}
	if loaded.SourceType != RetrievalSourceMemory || loaded.TrustTier != 2 || loaded.AuthorityRank != "rank_200_memory" || loaded.Hash != record.Hash {
		t.Fatalf("existing retrieval fields drifted: %#v", loaded)
	}
	if loaded.Confidence != 0.72 || loaded.ContradictionState != "suspected" || loaded.SuppressionStatus != "demoted" ||
		loaded.RetrievalBackend != "lexical" || !loaded.UsedLexical || loaded.UsedVector || loaded.PackPosition != 3 || loaded.TokenEstimate != 42 {
		t.Fatalf("metadata fields did not round-trip: %#v", loaded)
	}
	if len(loaded.EvidenceRefs) != 1 || loaded.EvidenceRefs[0] != "evidence.memory" {
		t.Fatalf("evidence refs did not round-trip: %#v", loaded.EvidenceRefs)
	}
}

func testRetrievalRecord(chunkID string, sourceType RetrievalSourceType, sourceID string, trustTier int, authorityRank string) RetrievalRecord {
	return RetrievalRecord{
		ChunkID:          chunkID,
		SourceID:         sourceID,
		SourceType:       sourceType,
		TrustTier:        trustTier,
		AuthorityRank:    authorityRank,
		Provenance:       map[string]any{"source_id": sourceID, "source_type": string(sourceType)},
		Freshness:        "fresh",
		Hash:             "sha256-test-" + chunkID,
		QuarantineStatus: RetrievalQuarantineClean,
		EmbeddingModel:   "mock",
		EmbeddingVersion: "1",
		ContentSummary:   "safe summary",
		RawContentRef:    "ref://" + chunkID,
	}
}

func openRetrievalStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
