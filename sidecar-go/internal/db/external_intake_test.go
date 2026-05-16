package db

import (
	"database/sql"
	"errors"
	"path/filepath"
	"slices"
	"testing"
)

func TestExternalImportMigrationCreatesDenyByDefaultTables(t *testing.T) {
	store := openExternalIntakeTestStore(t)

	version, err := store.SchemaVersion()
	if err != nil {
		t.Fatalf("schema version: %v", err)
	}
	if version < 5 {
		t.Fatalf("expected schema version at least 5, got %d", version)
	}

	for _, table := range []string{"external_imports", "external_import_artifacts", "external_activation_allowlist", "external_import_tool_descriptors", "external_tool_activation_allowlist"} {
		if got := countRows(t, store, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = '`+table+`'`); got != 1 {
			t.Fatalf("expected table %s to exist, got count %d", table, got)
		}
	}
}

func TestExternalImportCreateDefaultsToUntrustedAndDisabled(t *testing.T) {
	store := openExternalIntakeTestStore(t)

	if err := store.CreateExternalImport(ExternalImportRecord{
		ImportID:   "skill.local.weather",
		ItemType:   ExternalImportItemSkill,
		SourceType: ExternalImportSourceLocal,
		SourceURI:  "quarantine://skill.local.weather",
		SourceHash: "sha256-test",
		DeclaredCapabilities: []string{
			"network_read",
		},
		RequestedTools: []string{
			"weather.lookup",
		},
		RiskFindings: []string{
			"requires_validation",
		},
		ValidationResult: map[string]any{
			"valid": false,
		},
		Provenance: map[string]any{
			"source_type": "local",
			"source_uri":  "quarantine://skill.local.weather",
		},
	}); err != nil {
		t.Fatalf("create external import: %v", err)
	}

	loaded, err := store.GetExternalImport("skill.local.weather")
	if err != nil {
		t.Fatalf("get external import: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected external import record")
	}
	if loaded.State != ExternalImportStateImportedUntrusted {
		t.Fatalf("expected imported_untrusted state, got %q", loaded.State)
	}
	if loaded.ActivationState != ExternalActivationDisabled {
		t.Fatalf("expected disabled activation state, got %q", loaded.ActivationState)
	}
	if loaded.TrustTier != 0 || loaded.AuthorityRank != "external_untrusted" {
		t.Fatalf("expected deny-by-default trust, got tier=%d rank=%q", loaded.TrustTier, loaded.AuthorityRank)
	}
	if !slices.Contains(loaded.DeclaredCapabilities, "network_read") || !slices.Contains(loaded.RequestedTools, "weather.lookup") {
		t.Fatalf("expected capabilities/tools to round-trip, got %#v", loaded)
	}
	active, err := store.IsExternalImportActiveScoped("skill.local.weather")
	if err != nil {
		t.Fatalf("active scoped check: %v", err)
	}
	if active {
		t.Fatal("new external import should not be active scoped")
	}
}

func TestExternalImportRejectsInvalidStateAndSelfActivation(t *testing.T) {
	store := openExternalIntakeTestStore(t)

	tests := []struct {
		name   string
		record ExternalImportRecord
		want   string
	}{
		{
			name:   "missing id",
			record: ExternalImportRecord{ItemType: ExternalImportItemSkill},
			want:   "external_import_missing_id",
		},
		{
			name: "invalid source",
			record: ExternalImportRecord{
				ImportID:   "bad.source",
				ItemType:   ExternalImportItemSkill,
				SourceType: ExternalImportSourceType("marketplace_trusted"),
			},
			want: "unsupported_external_import_source_type:marketplace_trusted",
		},
		{
			name: "self active import state",
			record: ExternalImportRecord{
				ImportID: "self.active.state",
				ItemType: ExternalImportItemSkill,
				State:    ExternalImportStateActiveScoped,
			},
			want: "external_import_create_cannot_activate",
		},
		{
			name: "self active activation state",
			record: ExternalImportRecord{
				ImportID:        "self.active.activation",
				ItemType:        ExternalImportItemPlugin,
				ActivationState: ExternalActivationActiveScoped,
			},
			want: "external_import_create_cannot_activate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := store.CreateExternalImport(tt.record)
			if err == nil || err.Error() != tt.want {
				t.Fatalf("expected %q, got %v", tt.want, err)
			}
		})
	}
}

func TestExternalImportValidationAndArtifactsRoundTrip(t *testing.T) {
	store := openExternalIntakeTestStore(t)
	if err := store.CreateExternalImport(ExternalImportRecord{
		ImportID:   "plugin.validation",
		ItemType:   ExternalImportItemPlugin,
		SourceType: ExternalImportSourceUserBuilt,
	}); err != nil {
		t.Fatalf("create external import: %v", err)
	}

	if err := store.SetExternalImportValidation("plugin.validation", ExternalImportStateQuarantined, map[string]any{
		"manifest_present": true,
		"valid":            false,
	}, []string{"tool_descriptor_poisoning_scan_required"}); err != nil {
		t.Fatalf("set validation: %v", err)
	}
	if err := store.AddExternalImportArtifact(ExternalImportArtifact{
		ImportID:     "plugin.validation",
		ArtifactType: "manifest",
		ArtifactPath: "quarantine/plugin.validation/manifest.json",
		ContentHash:  "sha256-manifest",
	}); err != nil {
		t.Fatalf("add artifact: %v", err)
	}

	loaded, err := store.GetExternalImport("plugin.validation")
	if err != nil {
		t.Fatalf("get external import: %v", err)
	}
	if loaded.State != ExternalImportStateQuarantined {
		t.Fatalf("expected quarantined state, got %q", loaded.State)
	}
	if got, _ := loaded.ValidationResult["manifest_present"].(bool); !got {
		t.Fatalf("expected validation result to round-trip, got %#v", loaded.ValidationResult)
	}
	if !slices.Contains(loaded.RiskFindings, "tool_descriptor_poisoning_scan_required") {
		t.Fatalf("expected risk finding to round-trip, got %#v", loaded.RiskFindings)
	}
	if got := countRows(t, store, `SELECT COUNT(*) FROM external_import_artifacts WHERE import_id = 'plugin.validation' AND quarantine_state = 'quarantined'`); got != 1 {
		t.Fatalf("expected one quarantined artifact, got %d", got)
	}
}

func TestExternalActivationAllowlistRequiresScopedRuntimeActivation(t *testing.T) {
	store := openExternalIntakeTestStore(t)
	if err := store.CreateExternalImport(ExternalImportRecord{
		ImportID:   "plugin.scope",
		ItemType:   ExternalImportItemPlugin,
		SourceType: ExternalImportSourceLocal,
	}); err != nil {
		t.Fatalf("create external import: %v", err)
	}

	if err := store.SetExternalActivationAllowlist(ExternalActivationAllowlistEntry{
		ImportID:        "plugin.scope",
		ActivationState: ExternalActivationActiveScoped,
	}); err == nil || err.Error() != "external_activation_missing_scope" {
		t.Fatalf("expected missing scope error, got %v", err)
	}
	active, err := store.IsExternalImportActiveScoped("plugin.scope")
	if err != nil {
		t.Fatalf("active scoped check before allowlist: %v", err)
	}
	if active {
		t.Fatal("import should remain inactive after rejected activation")
	}

	if err := store.SetExternalActivationAllowlist(ExternalActivationAllowlistEntry{
		ImportID:          "plugin.scope",
		ActivationState:   ExternalActivationActiveScoped,
		ScopedPermissions: []string{"read_only"},
		ActivatedBy:       "runtime_test",
	}); err != nil {
		t.Fatalf("set activation allowlist: %v", err)
	}
	active, err = store.IsExternalImportActiveScoped("plugin.scope")
	if err != nil {
		t.Fatalf("active scoped check after allowlist: %v", err)
	}
	if !active {
		t.Fatal("expected explicit scoped activation to be active")
	}
	loaded, err := store.GetExternalImport("plugin.scope")
	if err != nil {
		t.Fatalf("get external import: %v", err)
	}
	if loaded.State != ExternalImportStateActiveScoped || loaded.ActivationState != ExternalActivationActiveScoped {
		t.Fatalf("expected active scoped import after allowlist, got %#v", loaded)
	}
}

func TestExternalImportValidationCannotActivateAndMissingRowsFailClosed(t *testing.T) {
	store := openExternalIntakeTestStore(t)

	if err := store.SetExternalImportValidation("missing", ExternalImportStateQuarantined, nil, nil); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected no rows error for missing validation target, got %v", err)
	}
	if err := store.SetExternalActivationAllowlist(ExternalActivationAllowlistEntry{
		ImportID:          "missing",
		ActivationState:   ExternalActivationActiveScoped,
		ScopedPermissions: []string{"read_only"},
	}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected no rows error for missing activation target, got %v", err)
	}

	if err := store.CreateExternalImport(ExternalImportRecord{ImportID: "validation.cannot.activate"}); err != nil {
		t.Fatalf("create external import: %v", err)
	}
	if err := store.SetExternalImportValidation("validation.cannot.activate", ExternalImportStateActiveScoped, nil, nil); err == nil || err.Error() != "external_import_validation_cannot_activate" {
		t.Fatalf("expected validation cannot activate error, got %v", err)
	}
}

func TestExternalToolDescriptorsAreInactiveByDefaultAndPerToolScoped(t *testing.T) {
	store := openExternalIntakeTestStore(t)
	if err := store.CreateExternalImport(ExternalImportRecord{
		ImportID:   "plugin.tools",
		ItemType:   ExternalImportItemPlugin,
		SourceType: ExternalImportSourceLocal,
	}); err != nil {
		t.Fatalf("create import: %v", err)
	}
	if err := store.AddExternalToolDescriptor(ExternalToolDescriptorRecord{
		ToolID:               "plugin.tools.lookup",
		ImportID:             "plugin.tools",
		Name:                 "Lookup",
		Description:          "Read-only lookup",
		RiskClasses:          []string{"read_only"},
		SideEffectsDeclared:  true,
		SideEffects:          false,
		RequiresConfirmation: false,
		InputSchema:          map[string]any{"type": "object"},
		OutputSchema:         map[string]any{"type": "object"},
		ValidationState:      ExternalImportStateValidatedCandidate,
	}); err != nil {
		t.Fatalf("add tool descriptor: %v", err)
	}
	active, err := store.IsExternalToolActiveScoped("plugin.tools.lookup")
	if err != nil {
		t.Fatalf("tool active scoped check: %v", err)
	}
	if active {
		t.Fatal("tool descriptor should not be active without per-tool allowlist")
	}
	if err := store.SetExternalToolActivationAllowlist(ExternalToolActivationAllowlistEntry{
		ToolID:          "plugin.tools.lookup",
		ImportID:        "plugin.tools",
		ActivationState: ExternalActivationActiveScoped,
	}); err == nil || err.Error() != "external_tool_activation_missing_scope" {
		t.Fatalf("expected missing tool scope error, got %v", err)
	}
	if err := store.SetExternalToolActivationAllowlist(ExternalToolActivationAllowlistEntry{
		ToolID:            "plugin.tools.lookup",
		ImportID:          "plugin.tools",
		ActivationState:   ExternalActivationActiveScoped,
		ScopedPermissions: []string{"read_only"},
	}); err != nil {
		t.Fatalf("activate tool: %v", err)
	}
	active, err = store.IsExternalToolActiveScoped("plugin.tools.lookup")
	if err != nil {
		t.Fatalf("tool active scoped check: %v", err)
	}
	if !active {
		t.Fatal("expected explicitly scoped tool to become active")
	}
}

func TestExternalToolDescriptorRejectsMissingSideEffectDeclaration(t *testing.T) {
	store := openExternalIntakeTestStore(t)
	if err := store.AddExternalToolDescriptor(ExternalToolDescriptorRecord{
		ToolID:       "plugin.bad.tool",
		ImportID:     "plugin.bad",
		Name:         "Bad Tool",
		InputSchema:  map[string]any{"type": "object"},
		OutputSchema: map[string]any{"type": "object"},
	}); err == nil || err.Error() != "external_tool_missing_side_effect_declaration" {
		t.Fatalf("expected side-effect declaration error, got %v", err)
	}
}

func openExternalIntakeTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}
