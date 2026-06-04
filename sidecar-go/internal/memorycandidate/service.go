package memorycandidate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"xmilo/sidecar-go/internal/db"
	"xmilo/sidecar-go/internal/promptsecrecy"
)

type Options struct {
	RunID       string
	ArchiveDate string
	Now         time.Time
}

type Result struct {
	CandidateCount   int
	FindingCount     int
	QuarantinedCount int
	SuppressedCount  int
}

type sourceRecord struct {
	sourceType         string
	sourceID           string
	title              string
	summary            string
	content            string
	trustTier          int
	authorityRank      string
	evidenceKind       string
	freshnessState     string
	confidence         float64
	contradictionState string
	quarantineStatus   string
	suppressionStatus  string
	candidateType      string
	status             string
	findingType        string
	memoryID           string
}

func Generate(ctx context.Context, store *db.Store, opts Options) (Result, error) {
	if store == nil {
		return Result{}, errors.New("memory_candidate_missing_store")
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	opts.RunID = strings.TrimSpace(opts.RunID)
	opts.ArchiveDate = strings.TrimSpace(opts.ArchiveDate)
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}

	records, err := taskHistoryRecords(ctx, store, opts)
	if err != nil {
		return Result{}, err
	}
	memoryRecords, err := memoryRecords(store)
	if err != nil {
		return Result{}, err
	}
	records = append(records, memoryRecords...)
	runRecords, err := consolidationRunRecords(store)
	if err != nil {
		return Result{}, err
	}
	records = append(records, runRecords...)

	var result Result
	for _, record := range records {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		candidate, evidence := buildCandidate(record, opts)
		if existing, err := store.GetMemoryCandidate(candidate.CandidateID); err != nil {
			return result, err
		} else if existing != nil && existing.Status == "rejected" {
			continue
		}
		if err := store.UpsertMemoryCandidate(candidate); err != nil {
			return result, err
		}
		if err := store.UpsertMemoryEvidenceRef(evidence); err != nil {
			return result, err
		}
		if record.findingType != "" {
			if err := store.UpsertMemoryFinding(db.MemoryFinding{
				FindingID:         stableID("finding", record.findingType, record.sourceType, record.sourceID, record.memoryID),
				MemoryIDs:         compactStrings(record.memoryID),
				CandidateIDs:      []string{candidate.CandidateID},
				FindingType:       record.findingType,
				Confidence:        record.confidence,
				EvidenceRefs:      []string{evidence.EvidenceID},
				RecommendedAction: "Review inert candidate; do not promote or mutate memory in Phase 18F-V1.",
				Status:            "needs_review",
				UserVisible:       true,
				CreatedAt:         opts.Now.UTC().Format(time.RFC3339),
				UpdatedAt:         opts.Now.UTC().Format(time.RFC3339),
			}); err != nil {
				return result, err
			}
			result.FindingCount++
		}
		result.CandidateCount++
		if candidate.QuarantineStatus == "quarantined" || candidate.Status == "quarantined" {
			result.QuarantinedCount++
		}
		if candidate.SuppressionStatus != "active" || candidate.Status == "suppressed" {
			result.SuppressedCount++
		}
	}
	return result, nil
}

func taskHistoryRecords(ctx context.Context, store *db.Store, opts Options) ([]sourceRecord, error) {
	query := `SELECT task_id, prompt, summary, created_at FROM task_history WHERE status = 'completed' AND task_id NOT LIKE 'nightly_archive_%'`
	args := []any{}
	if opts.ArchiveDate != "" {
		start, err := time.ParseInLocation("2006-01-02", opts.ArchiveDate, time.Local)
		if err != nil {
			return nil, err
		}
		end := start.Add(24 * time.Hour)
		query += ` AND created_at >= ? AND created_at < ?`
		args = append(args, start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339))
	}
	query += ` ORDER BY task_id ASC`
	rows, err := store.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []sourceRecord
	for rows.Next() {
		var taskID, prompt, summary, createdAt string
		if err := rows.Scan(&taskID, &prompt, &summary, &createdAt); err != nil {
			return nil, err
		}
		records = append(records, taskRecord(taskID, prompt, summary, createdAt))
	}
	return records, rows.Err()
}

func taskRecord(taskID, prompt, summary, createdAt string) sourceRecord {
	content := strings.TrimSpace(prompt + "\n" + summary)
	status := "generated"
	candidateType := "memory_candidate"
	findingType := ""
	title := "Memory review candidate"
	quarantineStatus := "clean"
	suppressionStatus := "active"
	contradictionState := "none"
	confidence := 0.45
	if looksProcedure(content) {
		candidateType = "procedure_candidate"
		title = "Procedure review candidate"
		confidence = 0.5
	}
	if unsafeSourceContent(content) {
		candidateType = "contradiction_staleness_finding"
		status = "quarantined"
		title = "Quarantined archive finding"
		content = "Archive input was omitted because it matched secret, external-instruction, or poisoning risk."
		findingType = "poisoning"
		quarantineStatus = "quarantined"
		suppressionStatus = "suppressed"
		contradictionState = "suspected"
		confidence = 0.8
	}
	return sourceRecord{
		sourceType:         "archive",
		sourceID:           taskID,
		title:              title,
		summary:            "Candidate generated from a redacted completed task archive record.",
		content:            content,
		trustTier:          5,
		authorityRank:      "rank_600_archive",
		evidenceKind:       "archive_ref",
		freshnessState:     "unknown",
		confidence:         confidence,
		contradictionState: contradictionState,
		quarantineStatus:   quarantineStatus,
		suppressionStatus:  suppressionStatus,
		candidateType:      candidateType,
		status:             status,
		findingType:        findingType,
		memoryID:           "",
	}
}

func memoryRecords(store *db.Store) ([]sourceRecord, error) {
	entries, err := store.ListMemoryEntriesForRetrievalPack()
	if err != nil {
		return nil, err
	}
	var records []sourceRecord
	for _, entry := range entries {
		if entry.MemoryClass == "approved_summary" && entry.Status == "active" && entry.QuarantineStatus == "clean" && entry.SuppressionStatus == "active" {
			records = append(records, sourceRecord{
				sourceType:         coalesce(entry.SourceType, "archive"),
				sourceID:           coalesce(entry.SourceID, entry.MemoryID),
				title:              "Retrieval anchor review candidate",
				summary:            "Candidate suggests a retrieval anchor for an approved summary without activating retrieval.",
				content:            entry.ContentExcerpt,
				trustTier:          entry.TrustTier + 1,
				authorityRank:      entry.AuthorityRank,
				evidenceKind:       evidenceKindForSource(entry.SourceType),
				freshnessState:     entry.FreshnessState,
				confidence:         boundedConfidence(entry.Confidence - 0.1),
				contradictionState: entry.ContradictionState,
				quarantineStatus:   "clean",
				suppressionStatus:  "active",
				candidateType:      "retrieval_anchor_candidate",
				status:             "needs_review",
				memoryID:           entry.MemoryID,
			})
		}
		if entry.FreshnessState == "stale" || entry.FreshnessState == "expired" || entry.ContradictionState == "suspected" || entry.ContradictionState == "confirmed" || entry.QuarantineStatus != "clean" || entry.SuppressionStatus != "active" {
			status := "needs_review"
			quarantineStatus := entry.QuarantineStatus
			suppressionStatus := entry.SuppressionStatus
			findingType := "stale"
			if entry.ContradictionState == "suspected" || entry.ContradictionState == "confirmed" {
				findingType = "contradiction"
			}
			if quarantineStatus != "clean" {
				status = "quarantined"
				findingType = "poisoning"
			}
			if suppressionStatus != "active" {
				status = "suppressed"
			}
			records = append(records, sourceRecord{
				sourceType:         coalesce(entry.SourceType, "archive"),
				sourceID:           coalesce(entry.SourceID, entry.MemoryID),
				title:              "Contradiction or staleness review candidate",
				summary:            "Candidate records stale, contradicted, quarantined, or suppressed memory for review only.",
				content:            entry.ContentExcerpt,
				trustTier:          entry.TrustTier + 1,
				authorityRank:      entry.AuthorityRank,
				evidenceKind:       evidenceKindForSource(entry.SourceType),
				freshnessState:     entry.FreshnessState,
				confidence:         boundedConfidence(entry.Confidence),
				contradictionState: entry.ContradictionState,
				quarantineStatus:   coalesce(quarantineStatus, "clean"),
				suppressionStatus:  coalesce(suppressionStatus, "active"),
				candidateType:      "contradiction_staleness_finding",
				status:             status,
				findingType:        findingType,
				memoryID:           entry.MemoryID,
			})
		}
	}
	return records, nil
}

func consolidationRunRecords(store *db.Store) ([]sourceRecord, error) {
	runs, err := store.ListConsolidationRuns()
	if err != nil {
		return nil, err
	}
	var records []sourceRecord
	for _, run := range runs {
		if run.Status != db.ConsolidationRunFailedSafe || run.ErrorCode == "" {
			continue
		}
		records = append(records, sourceRecord{
			sourceType:         "verified_runtime",
			sourceID:           run.RunID,
			title:              "Improvement proposal candidate",
			summary:            "Candidate proposes review of a failed-safe nightly maintenance path.",
			content:            run.ErrorCode + ": " + run.ErrorSummary,
			trustTier:          3,
			authorityRank:      "rank_200_runtime",
			evidenceKind:       "runtime_state",
			freshnessState:     "fresh",
			confidence:         0.7,
			contradictionState: "none",
			quarantineStatus:   "clean",
			suppressionStatus:  "active",
			candidateType:      "improvement_proposal",
			status:             "needs_review",
		})
	}
	return records, nil
}

func buildCandidate(record sourceRecord, opts Options) (db.MemoryCandidate, db.MemoryEvidenceRef) {
	sourceID := promptsecrecy.Redact(strings.TrimSpace(record.sourceID))
	candidateID := stableID("candidate", record.candidateType, record.sourceType, sourceID, record.memoryID)
	evidenceID := stableID("evidence", candidateID, record.sourceType, sourceID)
	content := safeContent(record.content)
	displayAllowed := record.status != "quarantined" && !promptsecrecy.Classify(record.content).Forbidden()
	if !displayAllowed {
		content = "Secret-bearing or unsafe source content was omitted."
	}
	now := opts.Now.UTC().Format(time.RFC3339)
	provenance := map[string]any{
		"phase":                    "18F-V1",
		"generation":               "deterministic_candidate_only_nightly_learning",
		"source_type":              record.sourceType,
		"source_id":                sourceID,
		"source_hash":              hashHex(record.sourceType + "\n" + sourceID + "\n" + safeContent(record.content)),
		"consolidation_run_id":     opts.RunID,
		"llm_reflection_used":      false,
		"approval_promotion_state": "deferred",
	}
	gateResult := map[string]any{
		"phase":               "18F-V1",
		"candidate_only":      true,
		"promotion_allowed":   false,
		"approval_allowed":    false,
		"reason":              "approval_promotion_deferred",
		"runtime_effect":      "none",
		"retrieval_effect":    "none",
		"llm_reflection_used": false,
	}
	candidate := db.MemoryCandidate{
		CandidateID:         candidateID,
		CandidateType:       record.candidateType,
		Status:              record.status,
		Title:               safeContent(record.title),
		Summary:             safeContent(record.summary),
		Content:             content,
		SourceType:          record.sourceType,
		SourceID:            sourceID,
		TrustTier:           record.trustTier,
		AuthorityRank:       record.authorityRank,
		Provenance:          provenance,
		EvidenceRefs:        []string{evidenceID},
		FreshnessState:      coalesce(record.freshnessState, "unknown"),
		Confidence:          boundedConfidence(record.confidence),
		ContradictionState:  coalesce(record.contradictionState, "none"),
		QuarantineStatus:    coalesce(record.quarantineStatus, "clean"),
		SuppressionStatus:   coalesce(record.suppressionStatus, "active"),
		ConsolidationRunID:  opts.RunID,
		PromotionGateResult: gateResult,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	evidence := db.MemoryEvidenceRef{
		EvidenceID:       evidenceID,
		CandidateID:      candidateID,
		SourceType:       record.sourceType,
		SourceID:         sourceID,
		SourceRef:        content,
		EvidenceKind:     record.evidenceKind,
		TrustTier:        record.trustTier,
		AuthorityRank:    record.authorityRank,
		Timestamp:        now,
		ContentHash:      hashHex(record.sourceType + "\n" + sourceID + "\n" + safeContent(record.content)),
		RedactionStatus:  redactionStatus(record.content),
		DisplayAllowed:   displayAllowed,
		PromotionAllowed: false,
		CreatedAt:        now,
	}
	return candidate, evidence
}

func looksProcedure(content string) bool {
	lower := strings.ToLower(content)
	for _, needle := range []string{"procedure", "setup", "step", "configure", "install", "workflow", "runbook"} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func unsafeSourceContent(content string) bool {
	if promptsecrecy.Classify(content).Forbidden() {
		return true
	}
	lower := strings.ToLower(content)
	if strings.Contains(lower, "external") && (strings.Contains(lower, "send all keys") || strings.Contains(lower, "redirect") || strings.Contains(lower, "exfiltrat") || strings.Contains(lower, "secret")) {
		return true
	}
	return false
}

func safeContent(content string) string {
	return promptsecrecy.Redact(strings.TrimSpace(content))
}

func redactionStatus(content string) string {
	redacted := safeContent(content)
	if redacted != strings.TrimSpace(content) || promptsecrecy.Classify(content).Forbidden() {
		return "redacted"
	}
	return "safe"
}

func stableID(parts ...string) string {
	return strings.Join([]string{strings.TrimSpace(parts[0]), hashHex(strings.Join(parts[1:], "\n"))[:20]}, ".")
}

func hashHex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func evidenceKindForSource(sourceType string) string {
	switch strings.TrimSpace(sourceType) {
	case "direct_user":
		return "user_statement"
	case "verified_runtime":
		return "runtime_state"
	case "external":
		return "external_ref"
	default:
		return "archive_ref"
	}
}

func boundedConfidence(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func compactStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func coalesce(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value != "" {
		return value
	}
	return fallback
}
