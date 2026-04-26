package db

import (
	"path/filepath"
	"testing"
	"time"

	"xmilo/sidecar-go/internal/runtime"
)

func TestMemoryEntryQuarantineRemovesFromActiveReads(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.SetActiveMemoryEntry(runtime.MemoryEntry{
		Class:     runtime.MemoryClassUserPreference,
		Key:       "preference_memory",
		Value:     "I prefer cinnamon.",
		Source:    "user_prompt",
		Effect:    "user_teaching",
		TrustTier: 5,
	}); err != nil {
		t.Fatalf("set active: %v", err)
	}

	if err := store.QuarantineActiveMemoryEntry(runtime.MemoryClassUserPreference, "preference_memory", "suspicious"); err != nil {
		t.Fatalf("quarantine: %v", err)
	}

	value, err := store.GetActiveMemoryValue(runtime.MemoryClassUserPreference, "preference_memory")
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if value != "" {
		t.Fatalf("expected no active value after quarantine, got %q", value)
	}
}

func TestMemoryEntryRestoreReactivatesSupersededValue(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.SetActiveMemoryEntry(runtime.MemoryEntry{
		Class:     runtime.MemoryClassUserPreference,
		Key:       "preference_memory",
		Value:     "I prefer cinnamon.",
		Source:    "user_prompt",
		Effect:    "user_teaching",
		TrustTier: 5,
	}); err != nil {
		t.Fatalf("set active: %v", err)
	}
	if err := store.SetActiveMemoryEntry(runtime.MemoryEntry{
		Class:     runtime.MemoryClassUserPreference,
		Key:       "preference_memory",
		Value:     "I prefer mint.",
		Source:    "user_prompt",
		Effect:    "user_teaching",
		TrustTier: 5,
	}); err != nil {
		t.Fatalf("supersede: %v", err)
	}

	value, err := store.GetActiveMemoryValue(runtime.MemoryClassUserPreference, "preference_memory")
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if value != "I prefer mint." {
		t.Fatalf("expected latest active value, got %q", value)
	}

	if err := store.RestoreMostRecentSupersededMemoryEntry(runtime.MemoryClassUserPreference, "preference_memory"); err != nil {
		t.Fatalf("restore: %v", err)
	}

	value, err = store.GetActiveMemoryValue(runtime.MemoryClassUserPreference, "preference_memory")
	if err != nil {
		t.Fatalf("get active (post-restore): %v", err)
	}
	if value != "I prefer cinnamon." {
		t.Fatalf("expected restored value, got %q", value)
	}
}

func TestConsolidateMemoryBoundedNeverDeletesActive(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.SetActiveMemoryEntry(runtime.MemoryEntry{
		Class:     runtime.MemoryClassUserPreference,
		Key:       "preference_memory",
		Value:     "I prefer mint.",
		Source:    "user_prompt",
		Effect:    "user_teaching",
		TrustTier: 5,
	}); err != nil {
		t.Fatalf("set active: %v", err)
	}

	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)
	for i := 0; i < 3; i++ {
		_, err := store.DB.Exec(`INSERT INTO memory_entries(class, mkey, value, status, source, effect, trust_tier, quarantine_reason, created_at, updated_at)
            VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			string(runtime.MemoryClassUserPreference),
			"preference_memory",
			"old superseded",
			string(runtime.MemoryEntryStatusSuperseded),
			"user_prompt",
			"user_teaching",
			5,
			"",
			old,
			old,
		)
		if err != nil {
			t.Fatalf("seed superseded: %v", err)
		}
	}
	for i := 0; i < 3; i++ {
		_, err := store.DB.Exec(`INSERT INTO memory_entries(class, mkey, value, status, source, effect, trust_tier, quarantine_reason, created_at, updated_at)
            VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			string(runtime.MemoryClassUserPreference),
			"preference_memory",
			"old quarantined",
			string(runtime.MemoryEntryStatusQuarantined),
			"user_prompt",
			"user_teaching",
			5,
			"suspicious",
			old,
			old,
		)
		if err != nil {
			t.Fatalf("seed quarantined: %v", err)
		}
	}

	if _, err := store.ConsolidateMemoryBounded(runtime.MemoryClassUserPreference, "preference_memory", 1, 1, 0); err != nil {
		t.Fatalf("consolidate: %v", err)
	}

	value, err := store.GetActiveMemoryValue(runtime.MemoryClassUserPreference, "preference_memory")
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if value != "I prefer mint." {
		t.Fatalf("expected active value preserved, got %q", value)
	}

	var supersededCount int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM memory_entries WHERE class = ? AND mkey = ? AND status = ?`,
		string(runtime.MemoryClassUserPreference), "preference_memory", string(runtime.MemoryEntryStatusSuperseded)).Scan(&supersededCount); err != nil {
		t.Fatalf("count superseded: %v", err)
	}
	if supersededCount > 1 {
		t.Fatalf("expected superseded pruned to <= 1, got %d", supersededCount)
	}

	var quarantinedCount int
	if err := store.DB.QueryRow(`SELECT COUNT(*) FROM memory_entries WHERE class = ? AND mkey = ? AND status = ?`,
		string(runtime.MemoryClassUserPreference), "preference_memory", string(runtime.MemoryEntryStatusQuarantined)).Scan(&quarantinedCount); err != nil {
		t.Fatalf("count quarantined: %v", err)
	}
	if quarantinedCount > 1 {
		t.Fatalf("expected quarantined pruned to <= 1, got %d", quarantinedCount)
	}
}
