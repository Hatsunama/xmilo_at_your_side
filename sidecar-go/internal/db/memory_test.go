package db

import (
	"path/filepath"
	"testing"
	"time"

	"xmilo/sidecar-go/internal/runtime"
)

func TestTaskSlotPersistsMemoryIntentAssessment(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	task := runtime.TaskSnapshot{
		TaskID:    "task_memory_write",
		AttemptID: "attempt_memory_write",
		Prompt:    "remember that I prefer cinnamon",
		Intent:    "memory",
		RoomID:    "archive",
		AnchorID:  "archive_lectern",
		Status:    "running",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		IntakeAssessment: &runtime.IntakeAssessment{
			PrimaryClass:       "MEMORY_WRITE_CANDIDATE",
			ValidationState:    "PENDING_APPROVAL",
			ChosenClosedAction: "DECLINE_MEMORY_WRITE",
			MemoryIntent: &runtime.MemoryIntent{
				Class:        "preference",
				Source:       "user_prompt",
				Effect:       "preference_write",
				SafetyStatus: "needs_confirmation",
			},
		},
	}

	if err := store.UpsertTask("active", task); err != nil {
		t.Fatalf("upsert task: %v", err)
	}

	loaded, err := store.GetTask("active")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if loaded == nil || loaded.IntakeAssessment == nil || loaded.IntakeAssessment.MemoryIntent == nil {
		t.Fatalf("expected persisted memory intent assessment, got %#v", loaded)
	}
	if got := loaded.IntakeAssessment.MemoryIntent.SafetyStatus; got != "needs_confirmation" {
		t.Fatalf("expected memory safety status to round-trip, got %q", got)
	}
	if got := loaded.AttemptID; got != "attempt_memory_write" {
		t.Fatalf("expected attempt_id to round-trip, got %q", got)
	}
}

func TestResumeCheckpointPersistsMemoryIntentAssessment(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	checkpoint := &runtime.ResumeCheckpoint{
		TaskID:             "task_memory_resume",
		SourceStatus:       "awaiting_user_choice",
		Phase:              "resume_after_user_choice",
		Blocker:            "memory_needs_confirmation",
		ContinuationStatus: "awaiting_user_choice",
		NextStepType:       "await_user_choice",
		ContextHash:        "context-hash",
		Status:             "awaiting_user_choice",
		CreatedAt:          time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:          time.Now().UTC().Format(time.RFC3339),
		ExpiresAt:          time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339),
		IntakeAssessment: &runtime.IntakeAssessment{
			PrimaryClass:       "MEMORY_WRITE_CANDIDATE",
			ValidationState:    "PENDING_APPROVAL",
			ChosenClosedAction: "DECLINE_MEMORY_WRITE",
			MemoryIntent: &runtime.MemoryIntent{
				Class:        "preference",
				Source:       "user_prompt",
				Effect:       "preference_write",
				SafetyStatus: "needs_confirmation",
			},
		},
	}

	if err := store.SetResumeCheckpoint(checkpoint); err != nil {
		t.Fatalf("set checkpoint: %v", err)
	}

	loaded, err := store.GetResumeCheckpoint()
	if err != nil {
		t.Fatalf("get checkpoint: %v", err)
	}
	if loaded == nil || loaded.IntakeAssessment == nil || loaded.IntakeAssessment.MemoryIntent == nil {
		t.Fatalf("expected persisted checkpoint memory intent, got %#v", loaded)
	}
	if got := loaded.IntakeAssessment.MemoryIntent.Effect; got != "preference_write" {
		t.Fatalf("expected memory effect to round-trip, got %q", got)
	}
}

func TestClearingResumeCheckpointRemovesMemoryIntentState(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	checkpoint := &runtime.ResumeCheckpoint{
		TaskID:      "task_memory_clear",
		Status:      "awaiting_user_choice",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
		ExpiresAt:   time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339),
		ContextHash: "context-hash",
		IntakeAssessment: &runtime.IntakeAssessment{
			MemoryIntent: &runtime.MemoryIntent{SafetyStatus: "needs_confirmation"},
		},
	}

	if err := store.SetResumeCheckpoint(checkpoint); err != nil {
		t.Fatalf("set checkpoint: %v", err)
	}
	if err := store.ClearResumeCheckpoint(); err != nil {
		t.Fatalf("clear checkpoint: %v", err)
	}

	loaded, err := store.GetResumeCheckpoint()
	if err != nil {
		t.Fatalf("get checkpoint: %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected checkpoint cleared, got %#v", loaded)
	}
}
