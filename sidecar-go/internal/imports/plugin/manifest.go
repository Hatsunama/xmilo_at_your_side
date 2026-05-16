package plugin

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"xmilo/sidecar-go/internal/db"
	importsafety "xmilo/sidecar-go/internal/imports"
)

const ManifestFileName = "plugin.json"

var allowedRiskClasses = []string{
	"read_only",
	"local_state_read",
	"local_state_write",
	"file_read",
	"file_write",
	"network_read",
	"network_write",
	"device_capability",
	"credential_sensitive",
	"destructive",
	"external_side_effect",
}

type Manifest struct {
	SchemaVersion string           `json:"schema_version"`
	PluginID      string           `json:"plugin_id"`
	Name          string           `json:"name"`
	Version       string           `json:"version"`
	Author        string           `json:"author"`
	SourceType    string           `json:"source_type"`
	SourceURI     string           `json:"source_uri"`
	Hash          string           `json:"hash"`
	Description   string           `json:"description"`
	Tools         []ToolDescriptor `json:"tools"`
}

type ToolDescriptor struct {
	ToolID                string         `json:"tool_id"`
	Name                  string         `json:"name"`
	Description           string         `json:"description"`
	InputSchema           map[string]any `json:"input_schema"`
	OutputSchema          map[string]any `json:"output_schema"`
	SideEffects           *bool          `json:"side_effects"`
	RequiresConfirmation  bool           `json:"requires_confirmation"`
	RequestedCapabilities []string       `json:"requested_capabilities"`
	RiskClasses           []string       `json:"risk_classes"`
}

type IntakeResult struct {
	Manifest     Manifest
	ImportID     string
	State        db.ExternalImportState
	RiskFindings []string
	ToolStates   map[string]db.ExternalImportState
}

func ParseManifest(raw []byte) (Manifest, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return Manifest{}, errors.New("plugin_manifest_missing")
	}
	var manifest Manifest
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, errors.New("plugin_manifest_malformed")
	}
	return manifest, nil
}

func ValidateManifest(manifest Manifest) (db.ExternalImportState, []string, map[string]db.ExternalImportState, map[string][]string) {
	var findings []string
	if strings.TrimSpace(manifest.SchemaVersion) != "1" {
		findings = append(findings, "unknown_schema_version")
	}
	if !importsafety.ValidID(manifest.PluginID) {
		findings = append(findings, "invalid_plugin_id")
	}
	if strings.TrimSpace(manifest.Name) == "" {
		findings = append(findings, "missing_name")
	}
	if strings.TrimSpace(manifest.Version) == "" {
		findings = append(findings, "missing_version")
	}
	if len(manifest.Tools) == 0 {
		findings = append(findings, "missing_tools")
	}
	if importsafety.ContainsAuthoritySpoof(manifest.Description) || importsafety.ContainsHiddenAutomation(manifest.Description) || importsafety.ContainsSecretRisk(manifest.Description) || importsafety.ContainsPromptLeakage(manifest.Description) {
		findings = append(findings, "plugin_description_poisoning")
	}

	toolStates := map[string]db.ExternalImportState{}
	toolFindings := map[string][]string{}
	for _, tool := range manifest.Tools {
		state, risks := ValidateToolDescriptor(tool)
		toolID := strings.TrimSpace(tool.ToolID)
		if toolID == "" {
			toolID = "missing_tool_id"
		}
		toolStates[toolID] = state
		toolFindings[toolID] = risks
		if state == db.ExternalImportStateBlocked {
			findings = append(findings, "blocked_tool:"+toolID)
		}
	}
	if len(findings) > 0 {
		return db.ExternalImportStateBlocked, findings, toolStates, toolFindings
	}
	return db.ExternalImportStateValidatedCandidate, findings, toolStates, toolFindings
}

func ValidateToolDescriptor(tool ToolDescriptor) (db.ExternalImportState, []string) {
	var findings []string
	if !importsafety.ValidID(tool.ToolID) {
		findings = append(findings, "invalid_tool_id")
	}
	if strings.TrimSpace(tool.Name) == "" {
		findings = append(findings, "missing_tool_name")
	}
	if tool.SideEffects == nil {
		findings = append(findings, "missing_side_effect_declaration")
	}
	if len(tool.InputSchema) == 0 {
		findings = append(findings, "missing_input_schema")
	}
	if len(tool.OutputSchema) == 0 {
		findings = append(findings, "missing_output_schema")
	}
	for _, riskClass := range tool.RiskClasses {
		if !slices.Contains(allowedRiskClasses, strings.TrimSpace(riskClass)) {
			findings = append(findings, "unknown_risk_class:"+riskClass)
		}
	}
	scanText := strings.Join([]string{
		tool.Description,
		importsafety.JoinStringFields(tool.RequestedCapabilities),
		importsafety.JoinStringFields(tool.RiskClasses),
	}, "\n")
	if importsafety.ContainsAuthoritySpoof(scanText) {
		findings = append(findings, "authority_spoofing")
	}
	if importsafety.ContainsHiddenAutomation(scanText) {
		findings = append(findings, "hidden_or_bypass_automation")
	}
	if importsafety.ContainsSecretRisk(scanText) {
		findings = append(findings, "credential_secret_risk")
	}
	if importsafety.ContainsToolPoisoning(scanText) {
		findings = append(findings, "tool_description_poisoning")
	}
	if importsafety.ContainsPromptLeakage(scanText) {
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
			return IntakeResult{}, errors.New("plugin_manifest_missing")
		}
		return IntakeResult{}, err
	}
	manifest, err := ParseManifest(raw)
	if err != nil {
		return IntakeResult{}, err
	}
	state, findings, toolStates, toolFindings := ValidateManifest(manifest)
	importID := strings.TrimSpace(manifest.PluginID)
	if importID == "" {
		importID = "invalid_plugin:" + importsafety.SHA256Hex(raw)
	}
	sourceHash := strings.TrimSpace(manifest.Hash)
	if sourceHash == "" {
		sourceHash = importsafety.SHA256Hex(raw)
	}
	sourceURI := strings.TrimSpace(manifest.SourceURI)
	if sourceURI == "" {
		sourceURI = manifestPath
	}
	record := db.ExternalImportRecord{
		ImportID:        importID,
		ItemType:        db.ExternalImportItemPlugin,
		SourceType:      normalizePluginSourceType(manifest.SourceType),
		SourceURI:       sourceURI,
		SourceHash:      sourceHash,
		DisplayName:     manifest.Name,
		Version:         manifest.Version,
		State:           state,
		ActivationState: db.ExternalActivationDisabled,
		TrustTier:       0,
		AuthorityRank:   "external_untrusted",
		RiskFindings:    findings,
		ValidationResult: map[string]any{
			"schema_version": manifest.SchemaVersion,
			"valid":          state == db.ExternalImportStateValidatedCandidate,
			"execution":      "disabled",
		},
		Provenance: map[string]any{
			"source_type": manifest.SourceType,
			"source_uri":  sourceURI,
			"hash":        sourceHash,
			"origin":      "local_plugin_manifest_intake",
		},
	}
	if err := store.CreateExternalImport(record); err != nil {
		return IntakeResult{}, err
	}
	if err := store.AddExternalImportArtifact(db.ExternalImportArtifact{
		ImportID:        importID,
		ArtifactType:    "plugin_manifest",
		ArtifactPath:    manifestPath,
		ContentHash:     sourceHash,
		QuarantineState: db.ExternalImportStateQuarantined,
	}); err != nil {
		return IntakeResult{}, err
	}
	for _, tool := range manifest.Tools {
		sideEffectsDeclared := tool.SideEffects != nil
		sideEffects := false
		if tool.SideEffects != nil {
			sideEffects = *tool.SideEffects
		}
		toolID := strings.TrimSpace(tool.ToolID)
		if !importsafety.ValidID(toolID) || !sideEffectsDeclared || len(tool.InputSchema) == 0 || len(tool.OutputSchema) == 0 || strings.TrimSpace(tool.Name) == "" {
			continue
		}
		if err := store.AddExternalToolDescriptor(db.ExternalToolDescriptorRecord{
			ToolID:                toolID,
			ImportID:              importID,
			Name:                  tool.Name,
			Description:           tool.Description,
			RiskClasses:           tool.RiskClasses,
			SideEffectsDeclared:   sideEffectsDeclared,
			SideEffects:           sideEffects,
			RequiresConfirmation:  tool.RequiresConfirmation,
			RequestedCapabilities: tool.RequestedCapabilities,
			InputSchema:           tool.InputSchema,
			OutputSchema:          tool.OutputSchema,
			ValidationState:       toolStates[toolID],
			RiskFindings:          toolFindings[toolID],
		}); err != nil {
			return IntakeResult{}, err
		}
	}
	return IntakeResult{Manifest: manifest, ImportID: importID, State: state, RiskFindings: findings, ToolStates: toolStates}, nil
}

func normalizePluginSourceType(sourceType string) db.ExternalImportSourceType {
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
