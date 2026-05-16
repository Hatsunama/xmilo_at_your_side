package skill

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"xmilo/sidecar-go/internal/db"
	importsafety "xmilo/sidecar-go/internal/imports"
)

const ManifestFileName = "skill.json"

type Manifest struct {
	SchemaVersion        string   `json:"schema_version"`
	SkillID              string   `json:"skill_id"`
	Name                 string   `json:"name"`
	Version              string   `json:"version"`
	Author               string   `json:"author"`
	SourceType           string   `json:"source_type"`
	SourceURI            string   `json:"source_uri"`
	Hash                 string   `json:"hash"`
	Description          string   `json:"description"`
	DeclaredCapabilities []string `json:"declared_capabilities"`
	RequestedTools       []string `json:"requested_tools"`
	Entrypoints          []string `json:"entrypoints"`
	Permissions          []string `json:"permissions"`
	NetworkAccess        bool     `json:"network_access"`
	FileAccess           string   `json:"file_access"`
	RiskNotes            []string `json:"risk_notes"`
}

type IntakeResult struct {
	Manifest     Manifest
	ImportID     string
	State        db.ExternalImportState
	RiskFindings []string
}

func ParseManifest(raw []byte) (Manifest, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return Manifest{}, errors.New("skill_manifest_missing")
	}
	var manifest Manifest
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, errors.New("skill_manifest_malformed")
	}
	return manifest, nil
}

func ValidateManifest(manifest Manifest) (db.ExternalImportState, []string) {
	var findings []string
	if strings.TrimSpace(manifest.SchemaVersion) != "1" {
		findings = append(findings, "unknown_schema_version")
	}
	if !importsafety.ValidID(manifest.SkillID) {
		findings = append(findings, "invalid_skill_id")
	}
	if strings.TrimSpace(manifest.Name) == "" {
		findings = append(findings, "missing_name")
	}
	if strings.TrimSpace(manifest.Version) == "" {
		findings = append(findings, "missing_version")
	}
	if importsafety.ContainsAuthoritySpoof(manifest.Description, importsafety.JoinStringFields(manifest.RiskNotes), importsafety.JoinStringFields(manifest.Entrypoints), importsafety.JoinStringFields(manifest.Permissions)) {
		findings = append(findings, "authority_spoofing")
	}
	if importsafety.ContainsHiddenAutomation(manifest.Description, importsafety.JoinStringFields(manifest.RiskNotes), importsafety.JoinStringFields(manifest.Entrypoints), importsafety.JoinStringFields(manifest.Permissions)) {
		findings = append(findings, "hidden_or_bypass_automation")
	}
	if importsafety.ContainsSecretRisk(manifest.Description, importsafety.JoinStringFields(manifest.RiskNotes), importsafety.JoinStringFields(manifest.Entrypoints), importsafety.JoinStringFields(manifest.Permissions)) {
		findings = append(findings, "credential_secret_risk")
	}
	if importsafety.ContainsPromptLeakage(manifest.Description, importsafety.JoinStringFields(manifest.RiskNotes), importsafety.JoinStringFields(manifest.Entrypoints), importsafety.JoinStringFields(manifest.Permissions)) {
		findings = append(findings, "prompt_secrecy_leakage")
	}
	if len(findings) > 0 {
		return db.ExternalImportStateBlocked, findings
	}
	return db.ExternalImportStateValidatedCandidate, findings
}

func IntakeLocalManifest(store *db.Store, path string) (IntakeResult, error) {
	manifestPath := path
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		manifestPath = filepath.Join(path, ManifestFileName)
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return IntakeResult{}, errors.New("skill_manifest_missing")
		}
		return IntakeResult{}, err
	}
	manifest, err := ParseManifest(raw)
	if err != nil {
		return IntakeResult{}, err
	}
	state, findings := ValidateManifest(manifest)
	importID := strings.TrimSpace(manifest.SkillID)
	if importID == "" {
		importID = "invalid_skill:" + importsafety.SHA256Hex(raw)
	}
	sourceHash := strings.TrimSpace(manifest.Hash)
	if sourceHash == "" {
		sourceHash = importsafety.SHA256Hex(raw)
	}
	sourceType := normalizeSkillSourceType(manifest.SourceType)
	sourceURI := strings.TrimSpace(manifest.SourceURI)
	if sourceURI == "" {
		sourceURI = manifestPath
	}

	record := db.ExternalImportRecord{
		ImportID:             importID,
		ItemType:             db.ExternalImportItemSkill,
		SourceType:           sourceType,
		SourceURI:            sourceURI,
		SourceHash:           sourceHash,
		DisplayName:          manifest.Name,
		Version:              manifest.Version,
		State:                state,
		ActivationState:      db.ExternalActivationDisabled,
		TrustTier:            0,
		AuthorityRank:        "external_untrusted",
		DeclaredCapabilities: manifest.DeclaredCapabilities,
		RequestedTools:       manifest.RequestedTools,
		RiskFindings:         findings,
		ValidationResult: map[string]any{
			"schema_version": manifest.SchemaVersion,
			"valid":          state == db.ExternalImportStateValidatedCandidate,
			"execution":      "disabled",
		},
		Provenance: map[string]any{
			"source_type": string(sourceType),
			"source_uri":  sourceURI,
			"hash":        sourceHash,
			"origin":      "local_manifest_intake",
		},
	}
	if err := store.CreateExternalImport(record); err != nil {
		return IntakeResult{}, err
	}
	if err := store.AddExternalImportArtifact(db.ExternalImportArtifact{
		ImportID:        importID,
		ArtifactType:    "skill_manifest",
		ArtifactPath:    manifestPath,
		ContentHash:     sourceHash,
		QuarantineState: db.ExternalImportStateQuarantined,
	}); err != nil {
		return IntakeResult{}, err
	}
	return IntakeResult{Manifest: manifest, ImportID: importID, State: state, RiskFindings: findings}, nil
}

func normalizeSkillSourceType(sourceType string) db.ExternalImportSourceType {
	switch strings.TrimSpace(sourceType) {
	case "online":
		return db.ExternalImportSourceOnline
	case "github":
		return db.ExternalImportSourceGitHub
	case "local":
		return db.ExternalImportSourceLocal
	case "user_built":
		return db.ExternalImportSourceUserBuilt
	case "bundled":
		return db.ExternalImportSourceBundled
	default:
		return db.ExternalImportSourceUnknown
	}
}
