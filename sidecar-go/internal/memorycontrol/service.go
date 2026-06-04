package memorycontrol

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"xmilo/sidecar-go/internal/db"
	"xmilo/sidecar-go/internal/promptsecrecy"
)

const (
	CodeInvalidRequest                    = "memory_invalid_request"
	CodeMemoryNotFound                    = "memory_not_found"
	CodeCandidateNotFound                 = "memory_candidate_not_found"
	CodeUnsupportedAction                 = "memory_unsupported_action"
	CodeForbiddenAction                   = "memory_forbidden_action"
	CodeConfirmationRequired              = "memory_confirmation_required"
	CodeProtectedTruthCannotModify        = "memory_protected_truth_cannot_modify"
	CodeCanonMemoryCannotModify           = "memory_canon_memory_cannot_modify"
	CodeRuntimeTruthCannotModify          = "memory_runtime_truth_cannot_modify"
	CodeApprovedSummaryCorrectionDeferred = "memory_approved_summary_correction_deferred"
	CodeCandidateApprovalDeferred         = "memory_candidate_approval_deferred"
	CodeRollbackDeferred                  = "memory_rollback_deferred"
	CodeEvidenceDisplayBlocked            = "memory_evidence_display_blocked"
	CodeUnsafeSecretBlocked               = "memory_unsafe_secret_blocked"
	CodeCandidatePromotionNotAllowed      = "memory_candidate_promotion_not_allowed"
	CodeValidationFailed                  = "memory_action_validation_failed"
)

type Error struct {
	Code   string
	Status int
}

func (e *Error) Error() string {
	return e.Code
}

type Service struct {
	store *db.Store
}

var auditIDCounter uint64

type ActionRequest struct {
	Reason         string `json:"reason"`
	Confirmation   bool   `json:"confirmation"`
	Title          string `json:"title"`
	Summary        string `json:"summary"`
	Content        string `json:"content"`
	ContentExcerpt string `json:"content_excerpt"`
}

type MemoryProjection struct {
	MemoryID           string   `json:"memory_id"`
	MemoryClass        string   `json:"memory_class"`
	Status             string   `json:"status"`
	Category           string   `json:"category"`
	Title              string   `json:"title"`
	Summary            string   `json:"summary"`
	ContentExcerpt     string   `json:"content_excerpt"`
	SourceType         string   `json:"source_type"`
	SourceID           string   `json:"source_id"`
	FreshnessState     string   `json:"freshness_state"`
	Confidence         float64  `json:"confidence"`
	ContradictionState string   `json:"contradiction_state"`
	QuarantineStatus   string   `json:"quarantine_status"`
	SuppressionStatus  string   `json:"suppression_status"`
	RetrievalEligible  bool     `json:"retrieval_eligible"`
	RetrievalReason    string   `json:"retrieval_reason"`
	RollbackAvailable  bool     `json:"rollback_available"`
	UserVisible        bool     `json:"user_visible"`
	Warnings           []string `json:"warnings"`
	AllowedActions     []string `json:"allowed_actions"`
}

type EvidenceProjection struct {
	EvidenceID       string `json:"evidence_id"`
	SourceType       string `json:"source_type"`
	SourceID         string `json:"source_id"`
	SourceRef        string `json:"source_ref"`
	EvidenceKind     string `json:"evidence_kind"`
	TrustTier        int    `json:"trust_tier"`
	AuthorityRank    string `json:"authority_rank"`
	Timestamp        string `json:"timestamp"`
	ContentHash      string `json:"content_hash"`
	RedactionStatus  string `json:"redaction_status"`
	DisplayAllowed   bool   `json:"display_allowed"`
	PromotionAllowed bool   `json:"promotion_allowed"`
}

type AuditProjection struct {
	AuditID                  string         `json:"audit_id"`
	MemoryID                 string         `json:"memory_id"`
	CandidateID              string         `json:"candidate_id"`
	Action                   string         `json:"action"`
	Actor                    string         `json:"actor"`
	Reason                   string         `json:"reason"`
	BeforeState              map[string]any `json:"before_state"`
	AfterState               map[string]any `json:"after_state"`
	Timestamp                string         `json:"timestamp"`
	RollbackRef              string         `json:"rollback_ref"`
	GateResult               map[string]any `json:"gate_result"`
	UserConfirmationRequired bool           `json:"user_confirmation_required"`
	SourceRequestID          string         `json:"source_request_id"`
}

type CandidateProjection struct {
	CandidateID        string   `json:"candidate_id"`
	CandidateType      string   `json:"candidate_type"`
	Status             string   `json:"status"`
	Title              string   `json:"title"`
	Summary            string   `json:"summary"`
	SourceType         string   `json:"source_type"`
	SourceID           string   `json:"source_id"`
	TrustTier          int      `json:"trust_tier"`
	AuthorityRank      string   `json:"authority_rank"`
	FreshnessState     string   `json:"freshness_state"`
	Confidence         float64  `json:"confidence"`
	ContradictionState string   `json:"contradiction_state"`
	QuarantineStatus   string   `json:"quarantine_status"`
	SuppressionStatus  string   `json:"suppression_status"`
	Warnings           []string `json:"warnings"`
	AllowedActions     []string `json:"allowed_actions"`
}

type ActionResponse struct {
	OK                   bool                 `json:"ok"`
	Action               string               `json:"action"`
	Status               string               `json:"status"`
	AuditID              string               `json:"audit_id,omitempty"`
	Memory               *MemoryProjection    `json:"memory,omitempty"`
	CorrectedMemory      *MemoryProjection    `json:"corrected_memory,omitempty"`
	Candidate            *CandidateProjection `json:"candidate,omitempty"`
	AllowedActions       []string             `json:"allowed_actions,omitempty"`
	Warnings             []string             `json:"warnings,omitempty"`
	ConfirmationRequired bool                 `json:"confirmation_required,omitempty"`
	Message              string               `json:"message,omitempty"`
}

func New(store *db.Store) *Service {
	return &Service{store: store}
}

func (s *Service) ListMemory() ([]MemoryProjection, error) {
	entries, err := s.store.ListMemoryEntriesForRetrievalPack()
	if err != nil {
		return nil, serviceError(CodeValidationFailed, http.StatusInternalServerError)
	}
	out := make([]MemoryProjection, 0, len(entries))
	for _, entry := range entries {
		out = append(out, projectMemory(entry))
	}
	return out, nil
}

func (s *Service) GetMemory(memoryID string) (*MemoryProjection, error) {
	entry, err := s.loadMemory(memoryID)
	if err != nil {
		return nil, err
	}
	projection := projectMemory(*entry)
	return &projection, nil
}

func (s *Service) GetProvenance(memoryID string) ([]EvidenceProjection, string, error) {
	entry, err := s.loadMemory(memoryID)
	if err != nil {
		return nil, "", err
	}
	refs, err := s.store.ListMemoryEvidenceRefsForMemoryIDs([]string{entry.MemoryID})
	if err != nil {
		return nil, "", serviceError(CodeValidationFailed, http.StatusInternalServerError)
	}
	projections := make([]EvidenceProjection, 0, len(refs))
	for _, ref := range refs {
		if !ref.DisplayAllowed || promptsecrecy.Classify(ref.SourceRef).Forbidden() {
			continue
		}
		projections = append(projections, projectEvidence(ref))
	}
	auditID, err := s.appendAudit(db.MemoryActionAudit{
		MemoryID:    entry.MemoryID,
		Action:      "view_provenance",
		Reason:      "view provenance",
		BeforeState: memoryState(*entry),
		AfterState:  map[string]any{"displayed_evidence_count": len(projections)},
	})
	if err != nil {
		return nil, "", err
	}
	return projections, auditID, nil
}

func (s *Service) ListAudit() ([]AuditProjection, error) {
	audits, err := s.store.ListMemoryActionAudit()
	if err != nil {
		return nil, serviceError(CodeValidationFailed, http.StatusInternalServerError)
	}
	out := make([]AuditProjection, 0, len(audits))
	for _, audit := range audits {
		out = append(out, projectAudit(audit))
	}
	return out, nil
}

func (s *Service) ListCandidates() ([]CandidateProjection, error) {
	candidates, err := s.store.ListMemoryCandidates()
	if err != nil {
		return nil, serviceError(CodeValidationFailed, http.StatusInternalServerError)
	}
	out := make([]CandidateProjection, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, projectCandidate(candidate))
	}
	return out, nil
}

func (s *Service) Suppress(memoryID string, req ActionRequest) (*ActionResponse, error) {
	entry, err := s.loadMemory(memoryID)
	if err != nil {
		return nil, err
	}
	if err := ensureMutableEntry(*entry, false); err != nil {
		return nil, err
	}
	before := memoryState(*entry)
	entry.Status = "suppressed"
	entry.SuppressionStatus = "suppressed"
	entry.RetrievalEligible = false
	entry.RetrievalReason = "suppressed by user"
	if err := s.store.UpsertMemoryEntry(*entry); err != nil {
		return nil, mapMemoryWriteError(err)
	}
	updated, _ := s.loadMemory(memoryID)
	auditID, err := s.appendAudit(db.MemoryActionAudit{
		MemoryID:    memoryID,
		Action:      "suppress",
		Reason:      req.Reason,
		BeforeState: before,
		AfterState:  memoryState(*updated),
	})
	if err != nil {
		return nil, err
	}
	projection := projectMemory(*updated)
	return &ActionResponse{OK: true, Action: "suppress", Status: updated.Status, AuditID: auditID, Memory: &projection, AllowedActions: projection.AllowedActions, Warnings: projection.Warnings}, nil
}

func (s *Service) RestoreSuppression(memoryID string, req ActionRequest) (*ActionResponse, error) {
	entry, err := s.loadMemory(memoryID)
	if err != nil {
		return nil, err
	}
	if err := ensureMutableEntry(*entry, false); err != nil {
		return nil, err
	}
	if entry.QuarantineStatus != "clean" || entry.ContradictionState == "confirmed" {
		return nil, serviceError(CodeForbiddenAction, http.StatusForbidden)
	}
	before := memoryState(*entry)
	entry.Status = "active"
	entry.SuppressionStatus = "active"
	entry.RetrievalEligible = true
	entry.RetrievalReason = "restored by user"
	if err := s.store.UpsertMemoryEntry(*entry); err != nil {
		return nil, mapMemoryWriteError(err)
	}
	updated, _ := s.loadMemory(memoryID)
	auditID, err := s.appendAudit(db.MemoryActionAudit{
		MemoryID:    memoryID,
		Action:      "restore_suppression",
		Reason:      req.Reason,
		BeforeState: before,
		AfterState:  memoryState(*updated),
	})
	if err != nil {
		return nil, err
	}
	projection := projectMemory(*updated)
	return &ActionResponse{OK: true, Action: "restore_suppression", Status: updated.Status, AuditID: auditID, Memory: &projection, AllowedActions: projection.AllowedActions, Warnings: projection.Warnings}, nil
}

func (s *Service) Delete(memoryID string, req ActionRequest) (*ActionResponse, error) {
	entry, err := s.loadMemory(memoryID)
	if err != nil {
		return nil, err
	}
	if err := ensureMutableEntry(*entry, false); err != nil {
		return nil, err
	}
	if !req.Confirmation {
		return nil, serviceError(CodeConfirmationRequired, http.StatusConflict)
	}
	before := memoryState(*entry)
	entry.Status = "deleted_by_user"
	entry.SuppressionStatus = "suppressed"
	entry.RetrievalEligible = false
	entry.RetrievalReason = "deleted by user"
	if err := s.store.UpsertMemoryEntry(*entry); err != nil {
		return nil, mapMemoryWriteError(err)
	}
	updated, _ := s.loadMemory(memoryID)
	auditID, err := s.appendAudit(db.MemoryActionAudit{
		MemoryID:                 memoryID,
		Action:                   "delete_user_remove",
		Reason:                   req.Reason,
		BeforeState:              before,
		AfterState:               memoryState(*updated),
		UserConfirmationRequired: true,
	})
	if err != nil {
		return nil, err
	}
	projection := projectMemory(*updated)
	return &ActionResponse{OK: true, Action: "delete_user_remove", Status: updated.Status, AuditID: auditID, Memory: &projection, AllowedActions: projection.AllowedActions, Warnings: projection.Warnings}, nil
}

func (s *Service) Correct(memoryID string, req ActionRequest) (*ActionResponse, error) {
	entry, err := s.loadMemory(memoryID)
	if err != nil {
		return nil, err
	}
	if entry.MemoryClass == "approved_summary" {
		return nil, serviceError(CodeApprovedSummaryCorrectionDeferred, http.StatusNotImplemented)
	}
	if err := ensureMutableEntry(*entry, true); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Summary) == "" && strings.TrimSpace(req.Content) == "" && strings.TrimSpace(req.Title) == "" {
		return nil, serviceError(CodeInvalidRequest, http.StatusBadRequest)
	}
	before := memoryState(*entry)
	corrected := *entry
	corrected.MemoryID = entry.MemoryID + ".corrected." + time.Now().UTC().Format("20060102T150405000000000Z")
	corrected.Status = "active"
	corrected.SourceType = "direct_user"
	corrected.SourceID = "memory_control"
	corrected.Provenance = map[string]any{"source_type": "direct_user", "source_id": "memory_control", "supersedes_memory_id": entry.MemoryID}
	corrected.SupersedesMemoryID = entry.MemoryID
	corrected.Title = firstNonBlank(req.Title, entry.Title)
	corrected.Summary = firstNonBlank(req.Summary, entry.Summary)
	corrected.Content = firstNonBlank(req.Content, entry.Content)
	corrected.ContentExcerpt = firstNonBlank(req.ContentExcerpt, corrected.Content)
	corrected.FreshnessState = "fresh"
	corrected.Confidence = minNonZero(entry.Confidence, 0.8)
	corrected.ContradictionState = "none"
	corrected.QuarantineStatus = "clean"
	corrected.SuppressionStatus = "active"
	corrected.RetrievalEligible = true
	corrected.RetrievalReason = "user correction"
	corrected.UserVisible = true
	corrected.CreatedAt = ""
	corrected.UpdatedAt = ""
	entry.Status = "superseded"
	entry.RetrievalEligible = false
	entry.RetrievalReason = "superseded by user correction"
	if err := s.store.UpsertMemoryEntry(*entry); err != nil {
		return nil, mapMemoryWriteError(err)
	}
	if err := s.store.UpsertMemoryEntry(corrected); err != nil {
		return nil, mapMemoryWriteError(err)
	}
	updatedOld, _ := s.loadMemory(memoryID)
	updatedNew, _ := s.loadMemory(corrected.MemoryID)
	auditID, err := s.appendAudit(db.MemoryActionAudit{
		MemoryID:    memoryID,
		Action:      "correct_supersede",
		Reason:      req.Reason,
		BeforeState: before,
		AfterState: map[string]any{
			"status":              updatedOld.Status,
			"corrected_memory_id": updatedNew.MemoryID,
		},
	})
	if err != nil {
		return nil, err
	}
	oldProjection := projectMemory(*updatedOld)
	newProjection := projectMemory(*updatedNew)
	return &ActionResponse{OK: true, Action: "correct_supersede", Status: updatedOld.Status, AuditID: auditID, Memory: &oldProjection, CorrectedMemory: &newProjection, AllowedActions: oldProjection.AllowedActions, Warnings: oldProjection.Warnings}, nil
}

func (s *Service) MarkStale(memoryID string, req ActionRequest) (*ActionResponse, error) {
	entry, err := s.loadMemory(memoryID)
	if err != nil {
		return nil, err
	}
	if err := ensureMutableEntry(*entry, false); err != nil {
		return nil, err
	}
	before := memoryState(*entry)
	entry.Status = "stale"
	entry.FreshnessState = "stale"
	entry.RetrievalEligible = false
	entry.RetrievalReason = "marked stale"
	if err := s.store.UpsertMemoryEntry(*entry); err != nil {
		return nil, mapMemoryWriteError(err)
	}
	updated, _ := s.loadMemory(memoryID)
	auditID, err := s.appendAudit(db.MemoryActionAudit{
		MemoryID:    memoryID,
		Action:      "mark_stale",
		Reason:      req.Reason,
		BeforeState: before,
		AfterState:  memoryState(*updated),
	})
	if err != nil {
		return nil, err
	}
	projection := projectMemory(*updated)
	return &ActionResponse{OK: true, Action: "mark_stale", Status: updated.Status, AuditID: auditID, Memory: &projection, AllowedActions: projection.AllowedActions, Warnings: projection.Warnings}, nil
}

func (s *Service) RejectCandidate(candidateID string, req ActionRequest) (*ActionResponse, error) {
	candidate, err := s.loadCandidate(candidateID)
	if err != nil {
		return nil, err
	}
	before := candidateState(*candidate)
	candidate.Status = "rejected"
	if err := s.store.UpsertMemoryCandidate(*candidate); err != nil {
		return nil, mapMemoryWriteError(err)
	}
	updated, _ := s.loadCandidate(candidateID)
	auditID, err := s.appendAudit(db.MemoryActionAudit{
		CandidateID:     candidateID,
		Action:          "reject_candidate",
		Reason:          req.Reason,
		BeforeState:     before,
		AfterState:      candidateState(*updated),
		GateResult:      map[string]any{"candidate_promotion": "not_allowed_in_v1"},
		SourceRequestID: "",
	})
	if err != nil {
		return nil, err
	}
	projection := projectCandidate(*updated)
	return &ActionResponse{OK: true, Action: "reject_candidate", Status: updated.Status, AuditID: auditID, Candidate: &projection, AllowedActions: projection.AllowedActions, Warnings: projection.Warnings}, nil
}

func (s *Service) ApproveCandidate(string, ActionRequest) (*ActionResponse, error) {
	return nil, serviceError(CodeCandidateApprovalDeferred, http.StatusNotImplemented)
}

func (s *Service) Rollback(string, ActionRequest) (*ActionResponse, error) {
	return nil, serviceError(CodeRollbackDeferred, http.StatusNotImplemented)
}

func (s *Service) loadMemory(memoryID string) (*db.MemoryEntry, error) {
	if strings.TrimSpace(memoryID) == "" {
		return nil, serviceError(CodeInvalidRequest, http.StatusBadRequest)
	}
	entry, err := s.store.GetMemoryEntry(memoryID)
	if err != nil {
		return nil, serviceError(CodeValidationFailed, http.StatusInternalServerError)
	}
	if entry == nil {
		return nil, serviceError(CodeMemoryNotFound, http.StatusNotFound)
	}
	return entry, nil
}

func (s *Service) loadCandidate(candidateID string) (*db.MemoryCandidate, error) {
	if strings.TrimSpace(candidateID) == "" {
		return nil, serviceError(CodeInvalidRequest, http.StatusBadRequest)
	}
	candidate, err := s.store.GetMemoryCandidate(candidateID)
	if err != nil {
		return nil, serviceError(CodeValidationFailed, http.StatusInternalServerError)
	}
	if candidate == nil {
		return nil, serviceError(CodeCandidateNotFound, http.StatusNotFound)
	}
	return candidate, nil
}

func (s *Service) appendAudit(audit db.MemoryActionAudit) (string, error) {
	audit.AuditID = fmt.Sprintf("memory_audit.%d.%d", time.Now().UTC().UnixNano(), atomic.AddUint64(&auditIDCounter, 1))
	audit.Actor = "user"
	if audit.GateResult == nil {
		audit.GateResult = map[string]any{"result": "allowed"}
	}
	if err := s.store.AppendMemoryActionAudit(audit); err != nil {
		return "", fmt.Errorf("%w: %s", serviceError(CodeValidationFailed, http.StatusInternalServerError), err.Error())
	}
	return audit.AuditID, nil
}

func projectMemory(entry db.MemoryEntry) MemoryProjection {
	warnings := memoryWarnings(entry)
	return MemoryProjection{
		MemoryID:           entry.MemoryID,
		MemoryClass:        entry.MemoryClass,
		Status:             entry.Status,
		Category:           entry.MemoryClass,
		Title:              promptsecrecy.Redact(entry.Title),
		Summary:            promptsecrecy.Redact(entry.Summary),
		ContentExcerpt:     promptsecrecy.Redact(firstNonBlank(entry.ContentExcerpt, entry.Summary)),
		SourceType:         entry.SourceType,
		SourceID:           promptsecrecy.Redact(entry.SourceID),
		FreshnessState:     entry.FreshnessState,
		Confidence:         entry.Confidence,
		ContradictionState: entry.ContradictionState,
		QuarantineStatus:   entry.QuarantineStatus,
		SuppressionStatus:  entry.SuppressionStatus,
		RetrievalEligible:  entry.RetrievalEligible,
		RetrievalReason:    promptsecrecy.Redact(entry.RetrievalReason),
		RollbackAvailable:  false,
		UserVisible:        entry.UserVisible,
		Warnings:           warnings,
		AllowedActions:     allowedActions(entry),
	}
}

func projectEvidence(ref db.MemoryEvidenceRef) EvidenceProjection {
	return EvidenceProjection{
		EvidenceID:       ref.EvidenceID,
		SourceType:       ref.SourceType,
		SourceID:         promptsecrecy.Redact(ref.SourceID),
		SourceRef:        promptsecrecy.Redact(ref.SourceRef),
		EvidenceKind:     ref.EvidenceKind,
		TrustTier:        ref.TrustTier,
		AuthorityRank:    ref.AuthorityRank,
		Timestamp:        ref.Timestamp,
		ContentHash:      ref.ContentHash,
		RedactionStatus:  ref.RedactionStatus,
		DisplayAllowed:   ref.DisplayAllowed,
		PromotionAllowed: ref.PromotionAllowed,
	}
}

func projectAudit(audit db.MemoryActionAudit) AuditProjection {
	return AuditProjection{
		AuditID:                  audit.AuditID,
		MemoryID:                 audit.MemoryID,
		CandidateID:              audit.CandidateID,
		Action:                   audit.Action,
		Actor:                    audit.Actor,
		Reason:                   promptsecrecy.Redact(audit.Reason),
		BeforeState:              sanitizeMap(audit.BeforeState),
		AfterState:               sanitizeMap(audit.AfterState),
		Timestamp:                audit.Timestamp,
		RollbackRef:              audit.RollbackRef,
		GateResult:               sanitizeMap(audit.GateResult),
		UserConfirmationRequired: audit.UserConfirmationRequired,
		SourceRequestID:          audit.SourceRequestID,
	}
}

func projectCandidate(candidate db.MemoryCandidate) CandidateProjection {
	return CandidateProjection{
		CandidateID:        candidate.CandidateID,
		CandidateType:      candidate.CandidateType,
		Status:             candidate.Status,
		Title:              promptsecrecy.Redact(candidate.Title),
		Summary:            promptsecrecy.Redact(candidate.Summary),
		SourceType:         candidate.SourceType,
		SourceID:           promptsecrecy.Redact(candidate.SourceID),
		TrustTier:          candidate.TrustTier,
		AuthorityRank:      candidate.AuthorityRank,
		FreshnessState:     candidate.FreshnessState,
		Confidence:         candidate.Confidence,
		ContradictionState: candidate.ContradictionState,
		QuarantineStatus:   candidate.QuarantineStatus,
		SuppressionStatus:  candidate.SuppressionStatus,
		Warnings:           []string{"candidate approval and promotion are deferred in Phase 18E2A-V1"},
		AllowedActions:     []string{"reject_candidate"},
	}
}

func allowedActions(entry db.MemoryEntry) []string {
	actions := []string{"view", "view_provenance"}
	if ensureMutableEntry(entry, false) != nil {
		return actions
	}
	if entry.Status != "deleted_by_user" {
		actions = append(actions, "suppress", "mark_stale")
	}
	if entry.SuppressionStatus == "suppressed" || entry.Status == "suppressed" {
		actions = append(actions, "restore_suppression")
	}
	if entry.Status != "deleted_by_user" {
		actions = append(actions, "delete_user_remove")
	}
	if userCorrectionClass(entry.MemoryClass) && entry.Status != "deleted_by_user" {
		actions = append(actions, "correct_supersede")
	}
	return actions
}

func memoryWarnings(entry db.MemoryEntry) []string {
	var warnings []string
	if entry.MemoryClass == "canon_memory" || entry.SourceType == "canon" || entry.SourceType == "main_hub" {
		warnings = append(warnings, "canon/main_hub memory is protected from user mutation")
	}
	if entry.MemoryClass == "runtime_observation" || entry.SourceType == "verified_runtime" {
		warnings = append(warnings, "runtime truth is view-only")
	}
	if entry.MemoryClass == "approved_summary" {
		warnings = append(warnings, "approved_summary correction is deferred")
	}
	if entry.QuarantineStatus != "clean" {
		warnings = append(warnings, "quarantined memory cannot drive retrieval")
	}
	if entry.ContradictionState == "confirmed" || entry.FreshnessState == "stale" {
		warnings = append(warnings, "memory requires review before it can safely drive behavior")
	}
	return warnings
}

func ensureMutableEntry(entry db.MemoryEntry, correction bool) error {
	if entry.MemoryClass == "canon_memory" || entry.SourceType == "canon" || entry.SourceType == "main_hub" {
		return serviceError(CodeCanonMemoryCannotModify, http.StatusForbidden)
	}
	if entry.MemoryClass == "runtime_observation" || entry.SourceType == "verified_runtime" {
		return serviceError(CodeRuntimeTruthCannotModify, http.StatusForbidden)
	}
	if protectedTruthClass(entry.MemoryClass) {
		return serviceError(CodeProtectedTruthCannotModify, http.StatusForbidden)
	}
	if correction && !userCorrectionClass(entry.MemoryClass) {
		return serviceError(CodeForbiddenAction, http.StatusForbidden)
	}
	return nil
}

func userCorrectionClass(memoryClass string) bool {
	switch memoryClass {
	case "durable_user_preference", "user_profile_context_fact", "task_continuity":
		return true
	default:
		return false
	}
}

func protectedTruthClass(memoryClass string) bool {
	switch memoryClass {
	case "quarantined_suppressed":
		return true
	default:
		return false
	}
}

func memoryState(entry db.MemoryEntry) map[string]any {
	return map[string]any{
		"memory_id":           entry.MemoryID,
		"memory_class":        entry.MemoryClass,
		"status":              entry.Status,
		"title":               entry.Title,
		"summary":             entry.Summary,
		"source_type":         entry.SourceType,
		"freshness_state":     entry.FreshnessState,
		"contradiction_state": entry.ContradictionState,
		"quarantine_status":   entry.QuarantineStatus,
		"suppression_status":  entry.SuppressionStatus,
		"retrieval_eligible":  entry.RetrievalEligible,
	}
}

func candidateState(candidate db.MemoryCandidate) map[string]any {
	return map[string]any{
		"candidate_id":   candidate.CandidateID,
		"candidate_type": candidate.CandidateType,
		"status":         candidate.Status,
		"title":          candidate.Title,
		"summary":        candidate.Summary,
		"source_type":    candidate.SourceType,
	}
}

func sanitizeMap(input map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range input {
		switch v := value.(type) {
		case string:
			out[key] = promptsecrecy.Redact(v)
		default:
			out[key] = v
		}
	}
	return out
}

func mapMemoryWriteError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "secret") {
		return serviceError(CodeUnsafeSecretBlocked, http.StatusForbidden)
	}
	if strings.Contains(err.Error(), "protected") || strings.Contains(err.Error(), "runtime_truth") {
		return serviceError(CodeProtectedTruthCannotModify, http.StatusForbidden)
	}
	return fmt.Errorf("%w: %s", serviceError(CodeValidationFailed, http.StatusBadRequest), err.Error())
}

func serviceError(code string, status int) *Error {
	return &Error{Code: code, Status: status}
}

func AsError(err error) (*Error, bool) {
	var serviceErr *Error
	if errors.As(err, &serviceErr) {
		return serviceErr, true
	}
	return nil, false
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func minNonZero(value, fallback float64) float64 {
	if value > 0 {
		return value
	}
	return fallback
}
