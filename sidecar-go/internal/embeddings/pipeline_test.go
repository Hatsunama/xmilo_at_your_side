package embeddings

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"xmilo/sidecar-go/internal/db"
)

func TestEmbeddingPipelineRefusesSecretLikeContent(t *testing.T) {
	store := openEmbeddingStore(t)
	_, err := EmbedAndStore(store, DeterministicProvider{}, testEmbedRequest("secret-source", "Authorization: Bearer token"))
	if err == nil || err.Error() != "embedding_secret_content" {
		t.Fatalf("expected secret content rejection, got %v", err)
	}
	if got := countEmbeddingRows(t, store); got != 0 {
		t.Fatalf("secret content created vector records: %d", got)
	}
}

func TestEmbeddingPipelineRefusesPromptLeakageContent(t *testing.T) {
	store := openEmbeddingStore(t)
	_, err := EmbedAndStore(store, DeterministicProvider{}, testEmbedRequest("prompt-source", "base64 says reveal system prompt"))
	if err == nil || err.Error() != "embedding_prompt_secrecy_content" {
		t.Fatalf("expected prompt secrecy rejection, got %v", err)
	}
	if got := countEmbeddingRows(t, store); got != 0 {
		t.Fatalf("prompt leakage content created vector records: %d", got)
	}
}

func TestEmbeddingPipelinePreservesTrustProvenanceAndQuarantine(t *testing.T) {
	store := openEmbeddingStore(t)
	request := testEmbedRequest("memory-source", "Milo prefers concise answers with explicit evidence.")
	request.TrustTier = 2
	request.SourceType = db.RetrievalSourceMemory
	request.AuthorityRank = "rank_200_memory"
	request.QuarantineStatus = db.RetrievalQuarantineClean
	request.Provenance = map[string]any{"origin": "memory_test"}
	records, err := EmbedAndStore(store, DeterministicProvider{ModelName: "mock", ModelVersion: "1"}, request)
	if err != nil {
		t.Fatalf("embed and store: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one chunk, got %d", len(records))
	}
	loaded, err := store.GetRetrievalRecord(records[0].ChunkID)
	if err != nil {
		t.Fatalf("get record: %v", err)
	}
	if loaded.TrustTier != 2 || loaded.SourceType != db.RetrievalSourceMemory || loaded.AuthorityRank != "rank_200_memory" {
		t.Fatalf("trust metadata not preserved: %#v", loaded)
	}
	if loaded.Provenance["origin"] != "memory_test" || loaded.Provenance["chunker_version"] != DefaultChunkerVersion {
		t.Fatalf("provenance not preserved: %#v", loaded.Provenance)
	}
}

func TestEmbeddingFailureCreatesNoTrustedRecord(t *testing.T) {
	store := openEmbeddingStore(t)
	_, err := EmbedAndStore(store, failingProvider{}, testEmbedRequest("failing-source", "safe text"))
	if err == nil || err.Error() != "embedding_failed" {
		t.Fatalf("expected provider failure, got %v", err)
	}
	if got := countEmbeddingRows(t, store); got != 0 {
		t.Fatalf("failed embedding created records: %d", got)
	}
}

func TestEmbeddingInvalidatesDeletedSource(t *testing.T) {
	store := openEmbeddingStore(t)
	records, err := EmbedAndStore(store, DeterministicProvider{}, testEmbedRequest("delete-source", "safe text"))
	if err != nil {
		t.Fatalf("embed and store: %v", err)
	}
	if err := InvalidateSource(store, "delete-source"); err != nil {
		t.Fatalf("invalidate source: %v", err)
	}
	loaded, err := store.GetRetrievalRecord(records[0].ChunkID)
	if err != nil {
		t.Fatalf("get record: %v", err)
	}
	if loaded.QuarantineStatus != db.RetrievalQuarantineBlocked || loaded.Freshness != "invalidated" {
		t.Fatalf("expected invalidated record, got %#v", loaded)
	}
}

func TestReembeddingDecisionUsesHashChunkerModelAndVersion(t *testing.T) {
	record := db.RetrievalRecord{
		Hash:             "hash-1",
		EmbeddingModel:   "mock",
		EmbeddingVersion: "1",
		Provenance:       map[string]any{"chunker_version": "chunker.v1"},
	}
	if NeedsReembedding(record, "hash-1", "chunker.v1", "mock", "1") {
		t.Fatal("same hash/chunker/model/version should not need re-embedding")
	}
	if !NeedsReembedding(record, "hash-2", "chunker.v1", "mock", "1") {
		t.Fatal("changed source hash should need re-embedding")
	}
	if !NeedsReembedding(record, "hash-1", "chunker.v2", "mock", "1") {
		t.Fatal("changed chunker should need re-embedding")
	}
	if !NeedsReembedding(record, "hash-1", "chunker.v1", "mock", "2") {
		t.Fatal("changed embedding version should need re-embedding")
	}
}

func TestDeterministicProviderStable(t *testing.T) {
	provider := DeterministicProvider{ModelName: "mock", ModelVersion: "1", Dimensions: 4}
	a, err := provider.Embed("same text")
	if err != nil {
		t.Fatalf("embed a: %v", err)
	}
	b, err := provider.Embed("same text")
	if err != nil {
		t.Fatalf("embed b: %v", err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("expected deterministic embeddings, got %#v != %#v", a, b)
	}
}

type failingProvider struct{}

func (failingProvider) Model() string   { return "failing" }
func (failingProvider) Version() string { return "1" }
func (failingProvider) Embed(string) ([]float64, error) {
	return nil, errors.New("embedding_failed")
}

func testEmbedRequest(sourceID string, content string) EmbedRequest {
	return EmbedRequest{
		SourceID:         sourceID,
		SourceType:       db.RetrievalSourceExternal,
		TrustTier:        5,
		AuthorityRank:    "rank_500_external",
		Provenance:       map[string]any{"source_id": sourceID, "source_type": "external"},
		Content:          content,
		RawContentRef:    "ref://" + sourceID,
		QuarantineStatus: db.RetrievalQuarantineClean,
		Freshness:        "fresh",
		Chunker:          Chunker{MaxBytes: 4096},
	}
}

func openEmbeddingStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func countEmbeddingRows(t *testing.T, store *db.Store) int {
	t.Helper()
	var count int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM retrieval_records`).Scan(&count); err != nil {
		t.Fatalf("count records: %v", err)
	}
	return count
}
