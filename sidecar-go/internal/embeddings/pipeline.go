package embeddings

import (
	"errors"
	"fmt"
	"hash/fnv"
	"strings"

	"xmilo/sidecar-go/internal/db"
	importsafety "xmilo/sidecar-go/internal/imports"
	"xmilo/sidecar-go/internal/promptsecrecy"
)

const DefaultChunkerVersion = "chunker.v1"

type Provider interface {
	Model() string
	Version() string
	Embed(text string) ([]float64, error)
}

type DeterministicProvider struct {
	ModelName    string
	ModelVersion string
	Dimensions   int
}

func (p DeterministicProvider) Model() string {
	if strings.TrimSpace(p.ModelName) == "" {
		return "local_mock_embedding"
	}
	return p.ModelName
}

func (p DeterministicProvider) Version() string {
	if strings.TrimSpace(p.ModelVersion) == "" {
		return "1"
	}
	return p.ModelVersion
}

func (p DeterministicProvider) Embed(text string) ([]float64, error) {
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("embedding_empty_text")
	}
	dimensions := p.Dimensions
	if dimensions <= 0 {
		dimensions = 8
	}
	out := make([]float64, dimensions)
	for i := 0; i < dimensions; i++ {
		h := fnv.New64a()
		_, _ = h.Write([]byte(fmt.Sprintf("%s:%d:%s", p.Model(), i, text)))
		out[i] = float64(h.Sum64()%1000000) / 1000000
	}
	return out, nil
}

type Chunk struct {
	ID      string
	Text    string
	Index   int
	Hash    string
	Summary string
}

type Chunker struct {
	MaxBytes int
	Version  string
}

func (c Chunker) Chunk(sourceID, content string) ([]Chunk, error) {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		return nil, errors.New("embedding_missing_source_id")
	}
	if strings.TrimSpace(content) == "" {
		return nil, errors.New("embedding_empty_content")
	}
	maxBytes := c.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 1024
	}
	var chunks []Chunk
	remaining := content
	for len([]byte(remaining)) > 0 {
		text := remaining
		if len([]byte(text)) > maxBytes {
			cut := maxBytes
			for cut > 0 && cut < len(remaining) && (remaining[cut]&0xC0) == 0x80 {
				cut--
			}
			text = remaining[:cut]
			remaining = remaining[cut:]
		} else {
			remaining = ""
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		hash := importsafety.SHA256Hex([]byte(text))
		chunks = append(chunks, Chunk{
			ID:      fmt.Sprintf("%s:%03d:%s", sourceID, len(chunks), strings.TrimPrefix(hash, "sha256:")[:12]),
			Text:    text,
			Index:   len(chunks),
			Hash:    hash,
			Summary: safeSummary(text),
		})
	}
	return chunks, nil
}

func (c Chunker) EffectiveVersion() string {
	if strings.TrimSpace(c.Version) == "" {
		return DefaultChunkerVersion
	}
	return c.Version
}

type EmbedRequest struct {
	SourceID                    string
	SourceType                  db.RetrievalSourceType
	TrustTier                   int
	AuthorityRank               string
	Provenance                  map[string]any
	Content                     string
	RawContentRef               string
	QuarantineStatus            db.RetrievalQuarantineStatus
	ContainsExternalInstruction bool
	ExpiresAt                   string
	Freshness                   string
	Chunker                     Chunker
}

func EmbedAndStore(store *db.Store, provider Provider, request EmbedRequest) ([]db.RetrievalRecord, error) {
	if provider == nil {
		return nil, errors.New("embedding_provider_missing")
	}
	if importsafety.ContainsSecretRisk(request.Content) {
		return nil, errors.New("embedding_secret_content")
	}
	if promptsecrecy.Classify(request.Content).Forbidden() {
		return nil, errors.New("embedding_prompt_secrecy_content")
	}
	if request.QuarantineStatus == "" {
		request.QuarantineStatus = db.RetrievalQuarantineClean
	}
	chunks, err := request.Chunker.Chunk(request.SourceID, request.Content)
	if err != nil {
		return nil, err
	}
	var records []db.RetrievalRecord
	for _, chunk := range chunks {
		vector, err := provider.Embed(chunk.Text)
		if err != nil {
			return nil, err
		}
		provenance := cloneMap(request.Provenance)
		if provenance == nil {
			provenance = map[string]any{}
		}
		provenance["source_id"] = request.SourceID
		provenance["source_type"] = string(request.SourceType)
		provenance["chunker_version"] = request.Chunker.EffectiveVersion()
		record := db.RetrievalRecord{
			ChunkID:                     chunk.ID,
			SourceID:                    request.SourceID,
			SourceType:                  request.SourceType,
			TrustTier:                   request.TrustTier,
			AuthorityRank:               request.AuthorityRank,
			Provenance:                  provenance,
			ExpiresAt:                   request.ExpiresAt,
			Freshness:                   request.Freshness,
			Hash:                        chunk.Hash,
			QuarantineStatus:            request.QuarantineStatus,
			ContainsExternalInstruction: request.ContainsExternalInstruction,
			ContainsSecret:              false,
			EmbeddingModel:              provider.Model(),
			EmbeddingVersion:            provider.Version(),
			ContentSummary:              chunk.Summary,
			RawContentRef:               request.RawContentRef,
			Embedding:                   vector,
		}
		if err := store.UpsertRetrievalRecord(record); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func NeedsReembedding(existing db.RetrievalRecord, sourceHash, chunkerVersion, embeddingModel, embeddingVersion string) bool {
	if existing.Hash != sourceHash {
		return true
	}
	if existing.EmbeddingModel != embeddingModel || existing.EmbeddingVersion != embeddingVersion {
		return true
	}
	if existing.Provenance == nil {
		return true
	}
	return fmt.Sprint(existing.Provenance["chunker_version"]) != chunkerVersion
}

func InvalidateSource(store *db.Store, sourceID string) error {
	return store.InvalidateRetrievalRecordsBySource(sourceID)
}

func safeSummary(content string) string {
	content = strings.TrimSpace(strings.ReplaceAll(content, "\n", " "))
	content = promptsecrecy.Redact(content)
	if len(content) > 180 {
		return content[:180]
	}
	return content
}

func cloneMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	out := make(map[string]any, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}
