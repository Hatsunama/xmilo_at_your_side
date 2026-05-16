package retrieval

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"xmilo/sidecar-go/internal/db"
	"xmilo/sidecar-go/internal/poisoning"
	"xmilo/sidecar-go/internal/promptsecrecy"
	"xmilo/sidecar-go/internal/runtimegate"
)

type RetrievalRequest struct {
	Query              string
	AllowedSourceTypes []db.RetrievalSourceType
	MaxTrustTier       int
	ContextBudgetBytes int
	MaxChunks          int
	Required           bool
	Now                time.Time
}

type RetrievalCandidate struct {
	Record     db.RetrievalRecord
	Similarity float64
}

type RetrievalResult struct {
	ChunkID       string
	SourceID      string
	SourceType    db.RetrievalSourceType
	TrustTier     int
	AuthorityRank string
	Provenance    map[string]any
	Content       string
	Label         string
}

type RetrievalDecision struct {
	Results    []RetrievalResult
	Safe       bool
	Reason     string
	Required   bool
	TotalSeen  int
	TotalKept  int
	TotalBytes int
}

func Retrieve(store *db.Store, request RetrievalRequest) (RetrievalDecision, error) {
	if strings.TrimSpace(request.Query) == "" {
		return RetrievalDecision{}, errors.New("retrieval_missing_query")
	}
	if request.Now.IsZero() {
		request.Now = time.Now().UTC()
	}
	if request.MaxTrustTier <= 0 {
		request.MaxTrustTier = 5
	}
	if request.ContextBudgetBytes <= 0 {
		request.ContextBudgetBytes = 8 * 1024
	}
	if request.MaxChunks <= 0 {
		request.MaxChunks = 8
	}

	records, err := store.ListRetrievalRecords()
	if err != nil {
		return RetrievalDecision{}, err
	}
	allowedSources := allowedSourceSet(request.AllowedSourceTypes)
	var candidates []RetrievalCandidate
	for _, record := range records {
		if len(allowedSources) > 0 && !allowedSources[record.SourceType] {
			continue
		}
		if record.TrustTier > request.MaxTrustTier {
			continue
		}
		if !retrievalPoisoningRevalidationAllows(record, request.Now) {
			continue
		}
		if !db.RetrievalRecordEligibleForTrustedSelection(record, request.Now) {
			continue
		}
		if retrievalContentUnsafe(record) {
			continue
		}
		if promptsecrecy.Classify(record.ContentSummary).Forbidden() {
			continue
		}
		candidates = append(candidates, RetrievalCandidate{
			Record:     record,
			Similarity: lexicalSimilarity(request.Query, record.ContentSummary),
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if left.Record.AuthorityRank != right.Record.AuthorityRank {
			return left.Record.AuthorityRank < right.Record.AuthorityRank
		}
		if left.Record.TrustTier != right.Record.TrustTier {
			return left.Record.TrustTier < right.Record.TrustTier
		}
		if left.Similarity != right.Similarity {
			return left.Similarity > right.Similarity
		}
		return left.Record.ChunkID < right.Record.ChunkID
	})

	decision := RetrievalDecision{Safe: true, Required: request.Required, TotalSeen: len(records)}
	for _, candidate := range candidates {
		if len(decision.Results) >= request.MaxChunks {
			break
		}
		content := strings.TrimSpace(candidate.Record.ContentSummary)
		if content == "" {
			continue
		}
		nextBytes := len([]byte(content))
		if decision.TotalBytes+nextBytes > request.ContextBudgetBytes {
			break
		}
		decision.Results = append(decision.Results, RetrievalResult{
			ChunkID:       candidate.Record.ChunkID,
			SourceID:      candidate.Record.SourceID,
			SourceType:    candidate.Record.SourceType,
			TrustTier:     candidate.Record.TrustTier,
			AuthorityRank: candidate.Record.AuthorityRank,
			Provenance:    candidate.Record.Provenance,
			Content:       promptsecrecy.Redact(content),
			Label:         "retrieved_content_as_labeled_data_only",
		})
		decision.TotalBytes += nextBytes
	}
	decision.TotalKept = len(decision.Results)
	if decision.TotalKept == 0 {
		decision.Reason = "no_safe_retrieval_results"
	}
	return decision, nil
}

func retrievalPoisoningRevalidationAllows(record db.RetrievalRecord, now time.Time) bool {
	trustTier := record.TrustTier
	assessment := poisoning.AssessCandidate(poisoning.Candidate{
		RecordKey:     record.ChunkID,
		RecordKind:    "retrieval_record",
		Content:       record.ContentSummary,
		SourceID:      record.SourceID,
		SourceType:    string(record.SourceType),
		TrustTier:     &trustTier,
		AuthorityRank: record.AuthorityRank,
		ProvenanceChain: []poisoning.ProvenanceNode{{
			SourceID:         record.SourceID,
			SourceType:       string(record.SourceType),
			TrustTier:        record.TrustTier,
			AuthorityRank:    record.AuthorityRank,
			Hash:             record.Hash,
			RuntimeOwned:     record.SourceType == db.RetrievalSourceRuntimeState,
			QuarantineStatus: string(record.QuarantineStatus),
		}},
		SourceHash:  record.Hash,
		Freshness:   record.Freshness,
		ExpiresAt:   record.ExpiresAt,
		Now:         now,
		TestFixture: provenanceContainsTestFixture(record.Provenance),
	})
	return assessment.Status == poisoning.QuarantineClean
}

func retrievalContentUnsafe(record db.RetrievalRecord) bool {
	if record.SourceType == db.RetrievalSourceCanon || record.SourceType == db.RetrievalSourceRuntimeState {
		return false
	}
	trustTier := record.TrustTier
	decision := runtimegate.EvaluateRetrievalContext(runtimegate.RetrievalContextInput{
		ChunkID:                     record.ChunkID,
		Content:                     record.ContentSummary,
		SourceID:                    record.SourceID,
		SourceType:                  string(record.SourceType),
		TrustTier:                   &trustTier,
		Provenance:                  record.Provenance,
		QuarantineStatus:            string(record.QuarantineStatus),
		ContainsSecret:              record.ContainsSecret,
		ContainsExternalInstruction: record.ContainsExternalInstruction,
	}, time.Now().UTC())
	return decision.Outcome != runtimegate.OutcomeAllow
}

func provenanceContainsTestFixture(provenance map[string]any) bool {
	for _, value := range provenance {
		if strings.Contains(strings.ToLower(strings.TrimSpace(fmtAny(value))), "test-fixture") || strings.Contains(strings.ToLower(strings.TrimSpace(fmtAny(value))), "test_fixture") {
			return true
		}
	}
	return false
}

func fmtAny(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(fmt.Sprint(value), "\n", " "), "\r", " "))
}

func allowedSourceSet(values []db.RetrievalSourceType) map[db.RetrievalSourceType]bool {
	if len(values) == 0 {
		return nil
	}
	out := make(map[db.RetrievalSourceType]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

func lexicalSimilarity(query, content string) float64 {
	queryWords := wordSet(query)
	contentWords := wordSet(content)
	if len(queryWords) == 0 || len(contentWords) == 0 {
		return 0
	}
	matches := 0
	for word := range queryWords {
		if contentWords[word] {
			matches++
		}
	}
	return float64(matches) / float64(len(queryWords))
}

func wordSet(value string) map[string]bool {
	out := map[string]bool{}
	for _, token := range strings.Fields(strings.ToLower(value)) {
		token = strings.Trim(token, ".,:;!?()[]{}\"'")
		if token != "" {
			out[token] = true
		}
	}
	return out
}
