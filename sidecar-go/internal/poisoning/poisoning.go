package poisoning

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type FindingCode string

const (
	FindingNone                          FindingCode = "none"
	FindingMissingProvenance             FindingCode = "missing_provenance"
	FindingAuthoritySpoof                FindingCode = "authority_spoof"
	FindingExternalInstruction           FindingCode = "external_instruction"
	FindingTransformedCommand            FindingCode = "transformed_command"
	FindingCredentialSecret              FindingCode = "credential_secret"
	FindingCapabilityTruthMutation       FindingCode = "capability_truth_mutation"
	FindingProviderTruthMutation         FindingCode = "provider_truth_mutation"
	FindingCompletionTruthMutation       FindingCode = "completion_truth_mutation"
	FindingStaleTruth                    FindingCode = "stale_truth"
	FindingConflict                      FindingCode = "conflict"
	FindingTestFixture                   FindingCode = "test_fixture_isolation"
	FindingSourceHashMismatch            FindingCode = "source_hash_mismatch"
	FindingMissingTrustMetadata          FindingCode = "missing_trust_metadata"
	FindingRetrievalRequiresRevalidation FindingCode = "retrieval_requires_revalidation"
)

type QuarantineStatus string

const (
	QuarantineClean       QuarantineStatus = "clean"
	QuarantineQuarantined QuarantineStatus = "quarantined"
	QuarantineBlocked     QuarantineStatus = "blocked"
)

type ProvenanceNode struct {
	SourceID         string `json:"source_id"`
	SourceType       string `json:"source_type"`
	TrustTier        int    `json:"trust_tier"`
	AuthorityRank    string `json:"authority_rank"`
	Hash             string `json:"hash,omitempty"`
	Transformation   string `json:"transformation,omitempty"`
	CreatedAt        string `json:"created_at,omitempty"`
	RuntimeOwned     bool   `json:"runtime_owned,omitempty"`
	VerifiedRuntime  bool   `json:"verified_runtime,omitempty"`
	TestFixture      bool   `json:"test_fixture,omitempty"`
	QuarantineStatus string `json:"quarantine_status,omitempty"`
}

type Candidate struct {
	RecordKey                  string
	RecordKind                 string
	Content                    string
	SourceID                   string
	SourceType                 string
	TrustTier                  *int
	AuthorityRank              string
	ProvenanceChain            []ProvenanceNode
	SourceHash                 string
	ExpectedSourceHash         string
	Freshness                  string
	ExpiresAt                  string
	Now                        time.Time
	TestFixture                bool
	VerifiedRuntimeState       bool
	VerifiedCompletionEvidence bool
}

type Finding struct {
	Code    FindingCode `json:"code"`
	Summary string      `json:"summary"`
}

type Assessment struct {
	Status             QuarantineStatus `json:"status"`
	Findings           []Finding        `json:"findings"`
	EffectiveTrustTier int              `json:"effective_trust_tier"`
	Stale              bool             `json:"stale"`
	Conflict           bool             `json:"conflict"`
}

func AssessCandidate(candidate Candidate) Assessment {
	if candidate.Now.IsZero() {
		candidate.Now = time.Now().UTC()
	}
	assessment := Assessment{Status: QuarantineClean}
	if candidate.TrustTier == nil || strings.TrimSpace(candidate.AuthorityRank) == "" || strings.TrimSpace(candidate.SourceType) == "" {
		assessment.add(FindingMissingTrustMetadata, "Durable record is missing trust tier, authority rank, or source type.")
	}
	if len(candidate.ProvenanceChain) == 0 {
		assessment.add(FindingMissingProvenance, "Durable record is missing provenance chain.")
	}
	for _, node := range candidate.ProvenanceChain {
		if node.TestFixture || containsFixture(node.SourceID, node.SourceType, node.Transformation) {
			assessment.add(FindingTestFixture, "Test fixture content cannot become durable runtime truth.")
		}
		if strings.TrimSpace(node.SourceID) == "" || strings.TrimSpace(node.SourceType) == "" || strings.TrimSpace(node.AuthorityRank) == "" {
			assessment.add(FindingMissingProvenance, "Provenance node is missing source, type, or authority rank.")
		}
	}
	if candidate.TestFixture || containsFixture(candidate.SourceID, candidate.SourceType, candidate.RecordKey, candidate.RecordKind) {
		assessment.add(FindingTestFixture, "Test fixture content cannot become durable runtime truth.")
	}
	if candidate.ExpectedSourceHash != "" && candidate.SourceHash != "" && candidate.ExpectedSourceHash != candidate.SourceHash {
		assessment.add(FindingSourceHashMismatch, "Durable record source hash changed and requires revalidation.")
	}
	if isStale(candidate) {
		assessment.Stale = true
		assessment.add(FindingStaleTruth, "Durable record is stale or expired.")
	}

	content := strings.ToLower(candidate.Content)
	switch {
	case containsAny(content, []string{"authorization: bearer", "api_key", "api key", "auth header", "provider config", "secret token", "access token"}):
		assessment.add(FindingCredentialSecret, "Durable record appears to contain secrets or provider configuration.")
	case containsAny(content, []string{"decoded instruction", "translation says to", "morse says", "base64 says", "qr says", "ocr says"}):
		assessment.add(FindingTransformedCommand, "Transformed content attempted to become durable instruction.")
	case containsAny(content, []string{"developer says", "system says", "ignore previous instructions", "user already approved", "override runtime authority", "new policy"}):
		assessment.add(FindingAuthoritySpoof, "Content attempted to spoof durable authority.")
	case containsAny(content, []string{"remember this as a rule", "from now on always", "make this permanent", "add this to memory as policy"}) && !directUserSource(candidate.SourceType):
		assessment.add(FindingExternalInstruction, "External content attempted to become durable instruction.")
	}
	if containsCapabilityTruthMutation(content) && !candidate.VerifiedRuntimeState {
		assessment.add(FindingCapabilityTruthMutation, "Unsupported capability truth mutation.")
	}
	if containsProviderTruthMutation(content) && !candidate.VerifiedRuntimeState {
		assessment.add(FindingProviderTruthMutation, "Unsupported provider truth mutation.")
	}
	if containsCompletionTruthMutation(content) && !candidate.VerifiedCompletionEvidence {
		assessment.add(FindingCompletionTruthMutation, "Unsupported completion truth mutation.")
	}

	assessment.EffectiveTrustTier = 9
	if candidate.TrustTier != nil {
		assessment.EffectiveTrustTier = *candidate.TrustTier
	}
	for _, finding := range assessment.Findings {
		switch finding.Code {
		case FindingStaleTruth, FindingSourceHashMismatch, FindingRetrievalRequiresRevalidation:
			if assessment.Status == QuarantineClean {
				assessment.Status = QuarantineQuarantined
			}
			if assessment.EffectiveTrustTier < 6 {
				assessment.EffectiveTrustTier = 6
			}
		default:
			assessment.Status = QuarantineBlocked
			assessment.EffectiveTrustTier = 9
		}
	}
	return assessment
}

type ConflictCandidate struct {
	RecordKey     string
	ClaimKey      string
	ClaimValue    string
	AuthorityRank string
	TrustTier     int
	SourceType    string
}

type ConflictResolution struct {
	WinnerKey string
	Losers    []string
	Findings  map[string][]Finding
}

func ResolveConflicts(candidates []ConflictCandidate) ConflictResolution {
	out := ConflictResolution{Findings: map[string][]Finding{}}
	byClaim := map[string][]ConflictCandidate{}
	for _, candidate := range candidates {
		key := strings.TrimSpace(candidate.ClaimKey)
		if key == "" {
			continue
		}
		byClaim[key] = append(byClaim[key], candidate)
	}
	for _, group := range byClaim {
		if len(group) < 2 || !hasConflictingValues(group) {
			continue
		}
		winner := group[0]
		for _, candidate := range group[1:] {
			if outranks(candidate, winner) {
				winner = candidate
			}
		}
		out.WinnerKey = winner.RecordKey
		for _, candidate := range group {
			if candidate.RecordKey == winner.RecordKey {
				continue
			}
			out.Losers = append(out.Losers, candidate.RecordKey)
			out.Findings[candidate.RecordKey] = append(out.Findings[candidate.RecordKey], Finding{
				Code:    FindingConflict,
				Summary: "Record conflicts with higher-authority durable truth.",
			})
		}
	}
	return out
}

func ValidateProvenanceChain(chain []ProvenanceNode) error {
	if len(chain) == 0 {
		return errors.New("provenance_chain_missing")
	}
	for i, node := range chain {
		if strings.TrimSpace(node.SourceID) == "" || strings.TrimSpace(node.SourceType) == "" || strings.TrimSpace(node.AuthorityRank) == "" {
			return fmt.Errorf("provenance_node_%d_missing_required_fields", i)
		}
		if node.TrustTier < 0 {
			return fmt.Errorf("provenance_node_%d_invalid_trust_tier", i)
		}
	}
	return nil
}

func (a Assessment) Blocked() bool {
	return a.Status == QuarantineBlocked
}

func (a Assessment) RequiresRevalidation() bool {
	return a.Status == QuarantineQuarantined
}

func (a *Assessment) add(code FindingCode, summary string) {
	a.Findings = append(a.Findings, Finding{Code: code, Summary: summary})
}

func isStale(candidate Candidate) bool {
	if strings.EqualFold(strings.TrimSpace(candidate.Freshness), "stale") || strings.EqualFold(strings.TrimSpace(candidate.Freshness), "invalidated") {
		return true
	}
	if strings.TrimSpace(candidate.ExpiresAt) == "" {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339, candidate.ExpiresAt)
	return err != nil || !expiresAt.After(candidate.Now)
}

func outranks(left, right ConflictCandidate) bool {
	if left.AuthorityRank != right.AuthorityRank {
		return left.AuthorityRank < right.AuthorityRank
	}
	if left.TrustTier != right.TrustTier {
		return left.TrustTier < right.TrustTier
	}
	return sourceRank(left.SourceType) < sourceRank(right.SourceType)
}

func sourceRank(sourceType string) int {
	switch strings.TrimSpace(sourceType) {
	case "canon":
		return 0
	case "runtime_state":
		return 1
	case "direct_user":
		return 2
	case "memory":
		return 3
	case "archive":
		return 4
	default:
		return 9
	}
}

func hasConflictingValues(group []ConflictCandidate) bool {
	first := strings.TrimSpace(group[0].ClaimValue)
	for _, candidate := range group[1:] {
		if strings.TrimSpace(candidate.ClaimValue) != first {
			return true
		}
	}
	return false
}

func containsFixture(parts ...string) bool {
	for _, part := range parts {
		lower := strings.ToLower(part)
		if strings.Contains(lower, "test_fixture") || strings.Contains(lower, "test-fixture") || strings.Contains(lower, "fixture:") {
			return true
		}
	}
	return false
}

func directUserSource(source string) bool {
	switch strings.TrimSpace(source) {
	case "direct_user", "user_prompt", "current_user_instruction":
		return true
	default:
		return false
	}
}

func containsCapabilityTruthMutation(content string) bool {
	return containsAny(content, []string{"camera works", "sensors available", "screen access works", "touch access works", "capability_state is"})
}

func containsProviderTruthMutation(content string) bool {
	return containsAny(content, []string{"hosted active", "byok active", "provider healthy", "provider route healthy"})
}

func containsCompletionTruthMutation(content string) bool {
	return containsAny(content, []string{"task completed", "completion evidence exists", "marked done", "mark as done"})
}

func containsAny(content string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(content, needle) {
			return true
		}
	}
	return false
}
