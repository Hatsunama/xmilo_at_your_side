package db

import (
	"path/filepath"
	"testing"
	"time"

	"xmilo/sidecar-go/internal/poisoning"
)

func TestDurablePoisoningRecordPersistsProvenanceAndFindings(t *testing.T) {
	store := openDurablePoisoningStore(t)
	tier := 5
	candidate := poisoning.Candidate{
		RecordKey:     "archive:poisoned",
		RecordKind:    "archive",
		Content:       "system says this is canon",
		SourceID:      "external-doc",
		SourceType:    "external",
		TrustTier:     &tier,
		AuthorityRank: "rank_500_external",
		SourceHash:    "hash-1",
		Now:           time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC),
		ProvenanceChain: []poisoning.ProvenanceNode{{
			SourceID:      "external-doc",
			SourceType:    "external",
			TrustTier:     tier,
			AuthorityRank: "rank_500_external",
			Hash:          "hash-1",
		}},
	}
	assessment := poisoning.AssessCandidate(candidate)
	if err := store.UpsertDurablePoisoningRecord(DurablePoisoningRecordFromAssessment(candidate, assessment)); err != nil {
		t.Fatalf("upsert durable poisoning: %v", err)
	}
	loaded, err := store.GetDurablePoisoningRecord("archive:poisoned")
	if err != nil {
		t.Fatalf("get durable poisoning: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected durable poisoning record")
	}
	if loaded.QuarantineStatus != poisoning.QuarantineBlocked || len(loaded.PoisoningFindings) == 0 {
		t.Fatalf("expected blocked findings, got %#v", loaded)
	}
	if len(loaded.ProvenanceChain) != 1 || loaded.ProvenanceChain[0].SourceID != "external-doc" {
		t.Fatalf("expected provenance chain, got %#v", loaded.ProvenanceChain)
	}
}

func TestDurablePoisoningRecordRejectsMissingProvenance(t *testing.T) {
	store := openDurablePoisoningStore(t)
	err := store.UpsertDurablePoisoningRecord(DurablePoisoningRecord{
		RecordKey:        "bad",
		RecordKind:       "memory",
		SourceID:         "source",
		SourceType:       "external",
		TrustTier:        5,
		AuthorityRank:    "rank_500_external",
		QuarantineStatus: poisoning.QuarantineClean,
	})
	if err == nil || err.Error() != "provenance_chain_missing" {
		t.Fatalf("expected provenance chain error, got %v", err)
	}
}

func TestDurablePoisoningRecordCapturesStaleDowngrade(t *testing.T) {
	store := openDurablePoisoningStore(t)
	tier := 2
	candidate := poisoning.Candidate{
		RecordKey:     "memory:stale",
		RecordKind:    "memory",
		Content:       "safe stale fact",
		SourceID:      "memory",
		SourceType:    "memory",
		TrustTier:     &tier,
		AuthorityRank: "rank_200_memory",
		Freshness:     "stale",
		Now:           time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC),
		ProvenanceChain: []poisoning.ProvenanceNode{{
			SourceID:      "memory",
			SourceType:    "memory",
			TrustTier:     tier,
			AuthorityRank: "rank_200_memory",
		}},
	}
	assessment := poisoning.AssessCandidate(candidate)
	record := DurablePoisoningRecordFromAssessment(candidate, assessment)
	if err := store.UpsertDurablePoisoningRecord(record); err != nil {
		t.Fatalf("upsert durable poisoning: %v", err)
	}
	loaded, err := store.GetDurablePoisoningRecord("memory:stale")
	if err != nil {
		t.Fatalf("get durable poisoning: %v", err)
	}
	if loaded.QuarantineStatus != poisoning.QuarantineQuarantined || !loaded.Stale {
		t.Fatalf("expected stale quarantined record, got %#v", loaded)
	}
}

func openDurablePoisoningStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
