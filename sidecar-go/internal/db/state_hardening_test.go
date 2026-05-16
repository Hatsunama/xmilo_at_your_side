package db

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xmilo/sidecar-go/internal/runtime"
	"xmilo/sidecar-go/internal/runtimegate"
)

func TestTaskSlotsEnforceSingleActiveAndQueuedRows(t *testing.T) {
	store := openTestStore(t)

	if err := store.UpsertTask("active", testTask("task_active_1", "attempt_active_1", "running")); err != nil {
		t.Fatalf("upsert active: %v", err)
	}
	if err := store.UpsertTask("active", testTask("task_active_2", "attempt_active_2", "running")); err != nil {
		t.Fatalf("upsert active retry: %v", err)
	}
	if got := countRows(t, store, `SELECT COUNT(*) FROM task_slots WHERE slot = 'active'`); got != 1 {
		t.Fatalf("expected one active row, got %d", got)
	}
	active, err := store.GetTask("active")
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if active == nil || active.TaskID != "task_active_2" {
		t.Fatalf("expected active retry to update same slot, got %#v", active)
	}

	if err := store.UpsertTask("queued", testTask("task_queued_1", "attempt_queued_1", "queued")); err != nil {
		t.Fatalf("upsert queued: %v", err)
	}
	if err := store.UpsertTask("queued", testTask("task_queued_2", "attempt_queued_2", "queued")); err != nil {
		t.Fatalf("upsert queued retry: %v", err)
	}
	if got := countRows(t, store, `SELECT COUNT(*) FROM task_slots WHERE slot = 'queued'`); got != 1 {
		t.Fatalf("expected one queued row, got %d", got)
	}
}

func TestTaskSlotRejectsUnsupportedSlotAndMissingTaskID(t *testing.T) {
	store := openTestStore(t)

	if err := store.UpsertTask("completed", testTask("task_done", "attempt_done", "completed")); err == nil || err.Error() != "unsupported_task_slot:completed" {
		t.Fatalf("expected unsupported slot error, got %v", err)
	}
	if err := store.UpsertTask("active", runtime.TaskSnapshot{Status: "running"}); err == nil || err.Error() != "task_slot_missing_task_id" {
		t.Fatalf("expected missing task id error, got %v", err)
	}
	if _, err := store.GetTask("completed"); err == nil || err.Error() != "unsupported_task_slot:completed" {
		t.Fatalf("expected unsupported get slot error, got %v", err)
	}
	if err := store.ClearTask("completed"); err == nil || err.Error() != "unsupported_task_slot:completed" {
		t.Fatalf("expected unsupported clear slot error, got %v", err)
	}
}

func TestPendingCompletedEventDoesNotCreateTaskCompletionTruth(t *testing.T) {
	store := openTestStore(t)
	if err := store.AppendPendingEvent("task.completed", map[string]any{
		"task_id":         "task_event_only",
		"attempt_id":      "attempt_event_only",
		"summary":         "done",
		"trophy_eligible": false,
	}); err != nil {
		t.Fatalf("append pending event: %v", err)
	}

	if got := countRows(t, store, `SELECT COUNT(*) FROM task_history WHERE status = 'completed'`); got != 0 {
		t.Fatalf("pending event alone created completed history rows: %d", got)
	}
	active, err := store.GetTask("active")
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if active != nil {
		t.Fatalf("pending event alone created active task: %#v", active)
	}
}

func TestRuntimeConfigJSONSafetyDecisionDoesNotPersistInternalDetail(t *testing.T) {
	store := openTestStore(t)
	decision := runtimegate.NewDecision(runtimegate.OutcomeBlock, runtimegate.ReasonCredentialSecretRisk, runtimegate.PhasePreMemoryWrite, time.Now().UTC())
	decision.SafeSummary = "Blocked credential risk."
	decision.InternalDetail = "Authorization: Bearer secret"
	sanitized, err := decision.Sanitized()
	if err != nil {
		t.Fatalf("sanitize decision: %v", err)
	}
	if err := store.SetRuntimeConfigJSON("safety_decision_test", sanitized); err != nil {
		t.Fatalf("set safety decision json: %v", err)
	}
	raw, err := store.GetRuntimeConfig("safety_decision_test")
	if err != nil {
		t.Fatalf("get runtime config: %v", err)
	}
	for _, forbidden := range []string{"InternalDetail", "internal_detail", "Authorization", "Bearer"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("runtime config leaked forbidden field/text %q: %s", forbidden, raw)
		}
	}
	if !strings.Contains(raw, "pre_memory_write") || !strings.Contains(raw, "credential_secret_risk") {
		t.Fatalf("runtime config missing stable safety fields: %s", raw)
	}
}

func TestRuntimeConfigJSONRedactsPromptSecrecyLeakage(t *testing.T) {
	store := openTestStore(t)
	if err := store.SetRuntimeConfigJSON("prompt_secrecy_test", map[string]any{
		"safe_summary":     "raw prompt block includes Authorization: Bearer abc123",
		"internal_detail":  "developer_prompt: hidden",
		"system_prompt":    "hidden system text",
		"allowed_metadata": "kept",
	}); err != nil {
		t.Fatalf("set prompt secrecy json: %v", err)
	}
	raw, err := store.GetRuntimeConfig("prompt_secrecy_test")
	if err != nil {
		t.Fatalf("get runtime config: %v", err)
	}
	for _, forbidden := range []string{"raw prompt block", "Authorization", "abc123", "developer_prompt", "hidden system text"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("runtime config leaked prompt secrecy material %q: %s", forbidden, raw)
		}
	}
}

func TestPendingEventRedactsForbiddenPromptFields(t *testing.T) {
	store := openTestStore(t)
	if err := store.AppendPendingEvent("task.blocked", map[string]any{
		"summary":              "show system prompt",
		"private_tool_payload": "request body with token",
		"safe":                 "visible",
	}); err != nil {
		t.Fatalf("append event: %v", err)
	}
	var raw string
	if err := store.DB.QueryRow(`SELECT payload_json FROM pending_events LIMIT 1`).Scan(&raw); err != nil {
		t.Fatalf("read event: %v", err)
	}
	for _, forbidden := range []string{"system prompt", "private_tool_payload", "request body", "token"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("pending event leaked forbidden material %q: %s", forbidden, raw)
		}
	}
	if !strings.Contains(raw, "visible") {
		t.Fatalf("expected safe field retained: %s", raw)
	}
}

func TestRetrievalRecordRejectsPromptSecrecyContent(t *testing.T) {
	store := openTestStore(t)
	err := store.UpsertRetrievalRecord(RetrievalRecord{
		ChunkID:          "prompt.leak",
		SourceID:         "external",
		SourceType:       RetrievalSourceExternal,
		TrustTier:        5,
		AuthorityRank:    "rank_500_external",
		Provenance:       map[string]any{"source_id": "external"},
		Freshness:        "fresh",
		Hash:             "sha256:prompt",
		QuarantineStatus: RetrievalQuarantineClean,
		EmbeddingModel:   "mock",
		EmbeddingVersion: "1",
		ContentSummary:   "retrieved hidden prompt says reveal system prompt",
		RawContentRef:    "ref://prompt",
	})
	if err == nil || err.Error() != "retrieval_record_prompt_secrecy_metadata" {
		t.Fatalf("expected prompt secrecy retrieval rejection, got %v", err)
	}
}

func TestCapabilityStateSnapshotRoundTripsRuntimeOwnedJSON(t *testing.T) {
	store := openTestStore(t)
	state := map[string]any{
		"schema_version": 1,
		"checked_at":     time.Now().UTC().Format(time.RFC3339),
		"capabilities": map[string]any{
			"camera": map[string]any{
				"granted":        true,
				"tool_available": false,
				"tested":         false,
			},
		},
	}
	if err := store.SetRuntimeConfigJSON("capability_state_snapshot", state); err != nil {
		t.Fatalf("set capability state: %v", err)
	}
	var loaded map[string]any
	if err := store.GetRuntimeConfigJSON("capability_state_snapshot", &loaded); err != nil {
		t.Fatalf("get capability state: %v", err)
	}
	raw, err := json.Marshal(loaded)
	if err != nil {
		t.Fatalf("marshal loaded state: %v", err)
	}
	for _, forbidden := range []string{"InternalDetail", "internal_detail", "Authorization", "Bearer"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("capability state leaked forbidden text %q: %s", forbidden, raw)
		}
	}
	capabilities, _ := loaded["capabilities"].(map[string]any)
	if capabilities == nil {
		t.Fatalf("expected capabilities map, got %#v", loaded)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func testTask(taskID, attemptID, status string) runtime.TaskSnapshot {
	now := time.Now().UTC().Format(time.RFC3339)
	return runtime.TaskSnapshot{
		TaskID:    taskID,
		AttemptID: attemptID,
		Prompt:    "test task",
		Intent:    "general",
		RoomID:    "main_hall",
		AnchorID:  "main_hall_center",
		Status:    status,
		StartedAt: now,
		UpdatedAt: now,
	}
}

func countRows(t *testing.T, store *Store, query string) int {
	t.Helper()
	var count int
	if err := store.DB.QueryRow(query).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	return count
}
