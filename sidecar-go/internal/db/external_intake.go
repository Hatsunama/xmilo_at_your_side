package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type ExternalImportItemType string

const (
	ExternalImportItemSkill           ExternalImportItemType = "skill"
	ExternalImportItemPlugin          ExternalImportItemType = "plugin"
	ExternalImportItemToolDescriptor  ExternalImportItemType = "tool_descriptor"
	ExternalImportItemPromptPack      ExternalImportItemType = "prompt_pack"
	ExternalImportItemRetrievalSource ExternalImportItemType = "retrieval_source"
	ExternalImportItemUnknown         ExternalImportItemType = "unknown"
)

type ExternalImportSourceType string

const (
	ExternalImportSourceOnline    ExternalImportSourceType = "online"
	ExternalImportSourceGitHub    ExternalImportSourceType = "github"
	ExternalImportSourceLocal     ExternalImportSourceType = "local"
	ExternalImportSourceUserBuilt ExternalImportSourceType = "user_built"
	ExternalImportSourceBundled   ExternalImportSourceType = "bundled"
	ExternalImportSourceUnknown   ExternalImportSourceType = "unknown"
)

type ExternalImportState string

const (
	ExternalImportStateImportedUntrusted  ExternalImportState = "imported_untrusted"
	ExternalImportStateQuarantined        ExternalImportState = "quarantined"
	ExternalImportStateValidatedCandidate ExternalImportState = "validated_candidate"
	ExternalImportStateBlocked            ExternalImportState = "blocked"
	ExternalImportStateApprovedInactive   ExternalImportState = "approved_inactive"
	ExternalImportStateActiveScoped       ExternalImportState = "active_scoped"
	ExternalImportStateDisabled           ExternalImportState = "disabled"
	ExternalImportStateRemoved            ExternalImportState = "removed"
)

type ExternalActivationState string

const (
	ExternalActivationDisabled         ExternalActivationState = "disabled"
	ExternalActivationApprovedInactive ExternalActivationState = "approved_inactive"
	ExternalActivationActiveScoped     ExternalActivationState = "active_scoped"
	ExternalActivationRemoved          ExternalActivationState = "removed"
)

type ExternalImportRecord struct {
	ImportID             string
	ItemType             ExternalImportItemType
	SourceType           ExternalImportSourceType
	SourceURI            string
	SourceHash           string
	DisplayName          string
	Version              string
	State                ExternalImportState
	TrustTier            int
	AuthorityRank        string
	ActivationState      ExternalActivationState
	DeclaredCapabilities []string
	RequestedTools       []string
	RiskFindings         []string
	ValidationResult     map[string]any
	Provenance           map[string]any
	CreatedAt            string
	UpdatedAt            string
}

type ExternalImportArtifact struct {
	ArtifactID      int64
	ImportID        string
	ArtifactType    string
	ArtifactPath    string
	ContentHash     string
	QuarantineState ExternalImportState
	CreatedAt       string
}

type ExternalActivationAllowlistEntry struct {
	ImportID          string
	ActivationState   ExternalActivationState
	ScopedPermissions []string
	ActivatedBy       string
	ActivatedAt       string
	ExpiresAt         string
	CreatedAt         string
	UpdatedAt         string
}

type ExternalToolDescriptorRecord struct {
	ToolID                string
	ImportID              string
	Name                  string
	Description           string
	RiskClasses           []string
	SideEffectsDeclared   bool
	SideEffects           bool
	RequiresConfirmation  bool
	RequestedCapabilities []string
	InputSchema           map[string]any
	OutputSchema          map[string]any
	ValidationState       ExternalImportState
	RiskFindings          []string
	CreatedAt             string
	UpdatedAt             string
}

type ExternalToolActivationAllowlistEntry struct {
	ToolID            string
	ImportID          string
	ActivationState   ExternalActivationState
	ScopedPermissions []string
	ActivatedBy       string
	ActivatedAt       string
	ExpiresAt         string
	CreatedAt         string
	UpdatedAt         string
}

func (s *Store) CreateExternalImport(record ExternalImportRecord) error {
	normalized, err := normalizeNewExternalImport(record)
	if err != nil {
		return err
	}
	declaredCapabilities, err := encodeStringList(normalized.DeclaredCapabilities)
	if err != nil {
		return err
	}
	requestedTools, err := encodeStringList(normalized.RequestedTools)
	if err != nil {
		return err
	}
	riskFindings, err := encodeStringList(normalized.RiskFindings)
	if err != nil {
		return err
	}
	validationResult, err := encodeStringMap(normalized.ValidationResult)
	if err != nil {
		return err
	}
	provenance, err := encodeStringMap(normalized.Provenance)
	if err != nil {
		return err
	}

	_, err = s.DB.Exec(`
        INSERT INTO external_imports(
            import_id, item_type, source_type, source_uri, source_hash, display_name, version,
            state, trust_tier, authority_rank, activation_state,
            declared_capabilities_json, requested_tools_json, risk_findings_json,
            validation_result_json, provenance_json, created_at, updated_at
        )
        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    `,
		normalized.ImportID, normalized.ItemType, normalized.SourceType, normalized.SourceURI, normalized.SourceHash,
		normalized.DisplayName, normalized.Version, normalized.State, normalized.TrustTier, normalized.AuthorityRank,
		normalized.ActivationState, declaredCapabilities, requestedTools, riskFindings, validationResult, provenance,
		normalized.CreatedAt, normalized.UpdatedAt,
	)
	return err
}

func (s *Store) AddExternalToolDescriptor(record ExternalToolDescriptorRecord) error {
	normalized, err := normalizeExternalToolDescriptor(record)
	if err != nil {
		return err
	}
	riskClasses, err := encodeStringList(normalized.RiskClasses)
	if err != nil {
		return err
	}
	requestedCapabilities, err := encodeStringList(normalized.RequestedCapabilities)
	if err != nil {
		return err
	}
	inputSchema, err := encodeStringMap(normalized.InputSchema)
	if err != nil {
		return err
	}
	outputSchema, err := encodeStringMap(normalized.OutputSchema)
	if err != nil {
		return err
	}
	riskFindings, err := encodeStringList(normalized.RiskFindings)
	if err != nil {
		return err
	}

	_, err = s.DB.Exec(`
        INSERT INTO external_import_tool_descriptors(
            tool_id, import_id, name, description, risk_classes_json, side_effects_declared,
            side_effects, requires_confirmation, requested_capabilities_json, input_schema_json,
            output_schema_json, validation_state, risk_findings_json, created_at, updated_at
        )
        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(tool_id) DO UPDATE SET
            import_id = excluded.import_id,
            name = excluded.name,
            description = excluded.description,
            risk_classes_json = excluded.risk_classes_json,
            side_effects_declared = excluded.side_effects_declared,
            side_effects = excluded.side_effects,
            requires_confirmation = excluded.requires_confirmation,
            requested_capabilities_json = excluded.requested_capabilities_json,
            input_schema_json = excluded.input_schema_json,
            output_schema_json = excluded.output_schema_json,
            validation_state = excluded.validation_state,
            risk_findings_json = excluded.risk_findings_json,
            updated_at = excluded.updated_at
    `, normalized.ToolID, normalized.ImportID, normalized.Name, normalized.Description, riskClasses, boolToInt(normalized.SideEffectsDeclared),
		boolToInt(normalized.SideEffects), boolToInt(normalized.RequiresConfirmation), requestedCapabilities, inputSchema, outputSchema,
		normalized.ValidationState, riskFindings, normalized.CreatedAt, normalized.UpdatedAt)
	return err
}

func (s *Store) GetExternalToolDescriptor(toolID string) (*ExternalToolDescriptorRecord, error) {
	toolID = strings.TrimSpace(toolID)
	if toolID == "" {
		return nil, errors.New("external_tool_missing_id")
	}
	row := s.DB.QueryRow(`
        SELECT tool_id, import_id, name, description, risk_classes_json, side_effects_declared,
            side_effects, requires_confirmation, requested_capabilities_json, input_schema_json,
            output_schema_json, validation_state, risk_findings_json, created_at, updated_at
        FROM external_import_tool_descriptors WHERE tool_id = ?
    `, toolID)
	var record ExternalToolDescriptorRecord
	var riskClasses, requestedCapabilities, inputSchema, outputSchema, riskFindings string
	var sideEffectsDeclared, sideEffects, requiresConfirmation int
	if err := row.Scan(&record.ToolID, &record.ImportID, &record.Name, &record.Description, &riskClasses, &sideEffectsDeclared,
		&sideEffects, &requiresConfirmation, &requestedCapabilities, &inputSchema, &outputSchema, &record.ValidationState,
		&riskFindings, &record.CreatedAt, &record.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var err error
	if record.RiskClasses, err = decodeStringList(riskClasses); err != nil {
		return nil, err
	}
	if record.RequestedCapabilities, err = decodeStringList(requestedCapabilities); err != nil {
		return nil, err
	}
	if record.InputSchema, err = decodeStringMap(inputSchema); err != nil {
		return nil, err
	}
	if record.OutputSchema, err = decodeStringMap(outputSchema); err != nil {
		return nil, err
	}
	if record.RiskFindings, err = decodeStringList(riskFindings); err != nil {
		return nil, err
	}
	record.SideEffectsDeclared = sideEffectsDeclared == 1
	record.SideEffects = sideEffects == 1
	record.RequiresConfirmation = requiresConfirmation == 1
	return &record, nil
}

func (s *Store) SetExternalToolActivationAllowlist(entry ExternalToolActivationAllowlistEntry) error {
	toolID := strings.TrimSpace(entry.ToolID)
	importID := strings.TrimSpace(entry.ImportID)
	if toolID == "" {
		return errors.New("external_tool_missing_id")
	}
	if importID == "" {
		return errors.New("external_import_missing_id")
	}
	if entry.ActivationState == "" {
		entry.ActivationState = ExternalActivationDisabled
	}
	if !isAllowedExternalActivationState(entry.ActivationState) {
		return fmt.Errorf("unsupported_external_activation_state:%s", entry.ActivationState)
	}
	if entry.ActivationState == ExternalActivationActiveScoped && len(entry.ScopedPermissions) == 0 {
		return errors.New("external_tool_activation_missing_scope")
	}
	scopedPermissions, err := encodeStringList(entry.ScopedPermissions)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if entry.CreatedAt == "" {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now
	if entry.ActivatedBy == "" {
		entry.ActivatedBy = "runtime"
	}
	_, err = s.DB.Exec(`
        INSERT INTO external_tool_activation_allowlist(tool_id, import_id, activation_state, scoped_permissions_json, activated_by, activated_at, expires_at, created_at, updated_at)
        VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(tool_id) DO UPDATE SET
            import_id = excluded.import_id,
            activation_state = excluded.activation_state,
            scoped_permissions_json = excluded.scoped_permissions_json,
            activated_by = excluded.activated_by,
            activated_at = excluded.activated_at,
            expires_at = excluded.expires_at,
            updated_at = excluded.updated_at
    `, toolID, importID, entry.ActivationState, scopedPermissions, entry.ActivatedBy, entry.ActivatedAt, entry.ExpiresAt, entry.CreatedAt, entry.UpdatedAt)
	return err
}

func (s *Store) IsExternalToolActiveScoped(toolID string) (bool, error) {
	toolID = strings.TrimSpace(toolID)
	if toolID == "" {
		return false, errors.New("external_tool_missing_id")
	}
	var state string
	err := s.DB.QueryRow(`SELECT activation_state FROM external_tool_activation_allowlist WHERE tool_id = ?`, toolID).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return state == string(ExternalActivationActiveScoped), nil
}

func (s *Store) GetExternalImport(importID string) (*ExternalImportRecord, error) {
	importID = strings.TrimSpace(importID)
	if importID == "" {
		return nil, errors.New("external_import_missing_id")
	}

	row := s.DB.QueryRow(`
        SELECT import_id, item_type, source_type, source_uri, source_hash, display_name, version,
            state, trust_tier, authority_rank, activation_state,
            declared_capabilities_json, requested_tools_json, risk_findings_json,
            validation_result_json, provenance_json, created_at, updated_at
        FROM external_imports WHERE import_id = ?
    `, importID)

	var record ExternalImportRecord
	var declaredCapabilities, requestedTools, riskFindings, validationResult, provenance string
	if err := row.Scan(
		&record.ImportID, &record.ItemType, &record.SourceType, &record.SourceURI, &record.SourceHash,
		&record.DisplayName, &record.Version, &record.State, &record.TrustTier, &record.AuthorityRank,
		&record.ActivationState, &declaredCapabilities, &requestedTools, &riskFindings, &validationResult,
		&provenance, &record.CreatedAt, &record.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	var err error
	if record.DeclaredCapabilities, err = decodeStringList(declaredCapabilities); err != nil {
		return nil, err
	}
	if record.RequestedTools, err = decodeStringList(requestedTools); err != nil {
		return nil, err
	}
	if record.RiskFindings, err = decodeStringList(riskFindings); err != nil {
		return nil, err
	}
	if record.ValidationResult, err = decodeStringMap(validationResult); err != nil {
		return nil, err
	}
	if record.Provenance, err = decodeStringMap(provenance); err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *Store) SetExternalImportValidation(importID string, state ExternalImportState, validationResult map[string]any, riskFindings []string) error {
	importID = strings.TrimSpace(importID)
	if importID == "" {
		return errors.New("external_import_missing_id")
	}
	if !isAllowedExternalImportState(state) {
		return fmt.Errorf("unsupported_external_import_state:%s", state)
	}
	if state == ExternalImportStateActiveScoped {
		return errors.New("external_import_validation_cannot_activate")
	}
	validationJSON, err := encodeStringMap(validationResult)
	if err != nil {
		return err
	}
	riskJSON, err := encodeStringList(riskFindings)
	if err != nil {
		return err
	}

	result, err := s.DB.Exec(`
        UPDATE external_imports
        SET state = ?, validation_result_json = ?, risk_findings_json = ?, updated_at = ?
        WHERE import_id = ?
    `, state, validationJSON, riskJSON, time.Now().UTC().Format(time.RFC3339), importID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) AddExternalImportArtifact(artifact ExternalImportArtifact) error {
	importID := strings.TrimSpace(artifact.ImportID)
	if importID == "" {
		return errors.New("external_import_missing_id")
	}
	if strings.TrimSpace(artifact.ArtifactType) == "" {
		return errors.New("external_import_artifact_missing_type")
	}
	if strings.TrimSpace(artifact.ArtifactPath) == "" {
		return errors.New("external_import_artifact_missing_path")
	}
	if artifact.QuarantineState == "" {
		artifact.QuarantineState = ExternalImportStateQuarantined
	}
	if !isAllowedExternalImportState(artifact.QuarantineState) {
		return fmt.Errorf("unsupported_external_import_state:%s", artifact.QuarantineState)
	}
	createdAt := artifact.CreatedAt
	if createdAt == "" {
		createdAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.DB.Exec(`
        INSERT INTO external_import_artifacts(import_id, artifact_type, artifact_path, content_hash, quarantine_state, created_at)
        VALUES(?, ?, ?, ?, ?, ?)
    `, importID, strings.TrimSpace(artifact.ArtifactType), strings.TrimSpace(artifact.ArtifactPath), strings.TrimSpace(artifact.ContentHash), artifact.QuarantineState, createdAt)
	return err
}

func (s *Store) SetExternalActivationAllowlist(entry ExternalActivationAllowlistEntry) error {
	importID := strings.TrimSpace(entry.ImportID)
	if importID == "" {
		return errors.New("external_import_missing_id")
	}
	if entry.ActivationState == "" {
		entry.ActivationState = ExternalActivationDisabled
	}
	if !isAllowedExternalActivationState(entry.ActivationState) {
		return fmt.Errorf("unsupported_external_activation_state:%s", entry.ActivationState)
	}
	if entry.ActivationState == ExternalActivationActiveScoped && len(entry.ScopedPermissions) == 0 {
		return errors.New("external_activation_missing_scope")
	}
	scopedPermissions, err := encodeStringList(entry.ScopedPermissions)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if entry.CreatedAt == "" {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now
	if entry.ActivatedBy == "" {
		entry.ActivatedBy = "runtime"
	}

	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
        UPDATE external_imports
        SET activation_state = ?, state = ?, updated_at = ?
        WHERE import_id = ?
    `, entry.ActivationState, importStateForActivation(entry.ActivationState), entry.UpdatedAt, importID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}

	if _, err := tx.Exec(`
        INSERT INTO external_activation_allowlist(import_id, activation_state, scoped_permissions_json, activated_by, activated_at, expires_at, created_at, updated_at)
        VALUES(?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(import_id) DO UPDATE SET
            activation_state = excluded.activation_state,
            scoped_permissions_json = excluded.scoped_permissions_json,
            activated_by = excluded.activated_by,
            activated_at = excluded.activated_at,
            expires_at = excluded.expires_at,
            updated_at = excluded.updated_at
    `, importID, entry.ActivationState, scopedPermissions, entry.ActivatedBy, entry.ActivatedAt, entry.ExpiresAt, entry.CreatedAt, entry.UpdatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) IsExternalImportActiveScoped(importID string) (bool, error) {
	importID = strings.TrimSpace(importID)
	if importID == "" {
		return false, errors.New("external_import_missing_id")
	}
	var state string
	err := s.DB.QueryRow(`SELECT activation_state FROM external_activation_allowlist WHERE import_id = ?`, importID).Scan(&state)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return state == string(ExternalActivationActiveScoped), nil
}

func normalizeNewExternalImport(record ExternalImportRecord) (ExternalImportRecord, error) {
	record.ImportID = strings.TrimSpace(record.ImportID)
	if record.ImportID == "" {
		return record, errors.New("external_import_missing_id")
	}
	if record.ItemType == "" {
		record.ItemType = ExternalImportItemUnknown
	}
	if !isAllowedExternalImportItemType(record.ItemType) {
		return record, fmt.Errorf("unsupported_external_import_item_type:%s", record.ItemType)
	}
	if record.SourceType == "" {
		record.SourceType = ExternalImportSourceUnknown
	}
	if !isAllowedExternalImportSourceType(record.SourceType) {
		return record, fmt.Errorf("unsupported_external_import_source_type:%s", record.SourceType)
	}
	if record.State == "" {
		record.State = ExternalImportStateImportedUntrusted
	}
	if !isAllowedExternalImportState(record.State) {
		return record, fmt.Errorf("unsupported_external_import_state:%s", record.State)
	}
	if record.State == ExternalImportStateActiveScoped {
		return record, errors.New("external_import_create_cannot_activate")
	}
	if record.ActivationState == "" {
		record.ActivationState = ExternalActivationDisabled
	}
	if !isAllowedExternalActivationState(record.ActivationState) {
		return record, fmt.Errorf("unsupported_external_activation_state:%s", record.ActivationState)
	}
	if record.ActivationState == ExternalActivationActiveScoped {
		return record, errors.New("external_import_create_cannot_activate")
	}
	if record.TrustTier < 0 {
		return record, errors.New("external_import_invalid_trust_tier")
	}
	if strings.TrimSpace(record.AuthorityRank) == "" {
		record.AuthorityRank = "external_untrusted"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if record.CreatedAt == "" {
		record.CreatedAt = now
	}
	if record.UpdatedAt == "" {
		record.UpdatedAt = record.CreatedAt
	}
	return record, nil
}

func normalizeExternalToolDescriptor(record ExternalToolDescriptorRecord) (ExternalToolDescriptorRecord, error) {
	record.ToolID = strings.TrimSpace(record.ToolID)
	record.ImportID = strings.TrimSpace(record.ImportID)
	if record.ToolID == "" {
		return record, errors.New("external_tool_missing_id")
	}
	if record.ImportID == "" {
		return record, errors.New("external_import_missing_id")
	}
	if strings.TrimSpace(record.Name) == "" {
		return record, errors.New("external_tool_missing_name")
	}
	if !record.SideEffectsDeclared {
		return record, errors.New("external_tool_missing_side_effect_declaration")
	}
	if len(record.InputSchema) == 0 {
		return record, errors.New("external_tool_missing_input_schema")
	}
	if len(record.OutputSchema) == 0 {
		return record, errors.New("external_tool_missing_output_schema")
	}
	if record.ValidationState == "" {
		record.ValidationState = ExternalImportStateQuarantined
	}
	if !isAllowedExternalImportState(record.ValidationState) {
		return record, fmt.Errorf("unsupported_external_import_state:%s", record.ValidationState)
	}
	if record.ValidationState == ExternalImportStateActiveScoped {
		return record, errors.New("external_tool_descriptor_cannot_activate")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if record.CreatedAt == "" {
		record.CreatedAt = now
	}
	if record.UpdatedAt == "" {
		record.UpdatedAt = record.CreatedAt
	}
	return record, nil
}

func importStateForActivation(state ExternalActivationState) ExternalImportState {
	switch state {
	case ExternalActivationApprovedInactive:
		return ExternalImportStateApprovedInactive
	case ExternalActivationActiveScoped:
		return ExternalImportStateActiveScoped
	case ExternalActivationRemoved:
		return ExternalImportStateRemoved
	default:
		return ExternalImportStateDisabled
	}
}

func isAllowedExternalImportItemType(itemType ExternalImportItemType) bool {
	switch itemType {
	case ExternalImportItemSkill, ExternalImportItemPlugin, ExternalImportItemToolDescriptor, ExternalImportItemPromptPack, ExternalImportItemRetrievalSource, ExternalImportItemUnknown:
		return true
	default:
		return false
	}
}

func isAllowedExternalImportSourceType(sourceType ExternalImportSourceType) bool {
	switch sourceType {
	case ExternalImportSourceOnline, ExternalImportSourceGitHub, ExternalImportSourceLocal, ExternalImportSourceUserBuilt, ExternalImportSourceBundled, ExternalImportSourceUnknown:
		return true
	default:
		return false
	}
}

func isAllowedExternalImportState(state ExternalImportState) bool {
	switch state {
	case ExternalImportStateImportedUntrusted, ExternalImportStateQuarantined, ExternalImportStateValidatedCandidate, ExternalImportStateBlocked, ExternalImportStateApprovedInactive, ExternalImportStateActiveScoped, ExternalImportStateDisabled, ExternalImportStateRemoved:
		return true
	default:
		return false
	}
}

func isAllowedExternalActivationState(state ExternalActivationState) bool {
	switch state {
	case ExternalActivationDisabled, ExternalActivationApprovedInactive, ExternalActivationActiveScoped, ExternalActivationRemoved:
		return true
	default:
		return false
	}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func encodeStringList(values []string) (string, error) {
	if values == nil {
		values = []string{}
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func decodeStringList(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return []string{}, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return values, nil
}

func encodeStringMap(values map[string]any) (string, error) {
	if values == nil {
		values = map[string]any{}
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func decodeStringMap(raw string) (map[string]any, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}, nil
	}
	var values map[string]any
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return values, nil
}
