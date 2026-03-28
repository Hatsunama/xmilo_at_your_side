package tasks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"xmilo/sidecar-go/internal/db"
	"xmilo/sidecar-go/internal/movement"
	"xmilo/sidecar-go/internal/relay"
	"xmilo/sidecar-go/internal/rooms"
	"xmilo/sidecar-go/internal/runtime"
	"xmilo/sidecar-go/internal/ws"
	"xmilo/sidecar-go/shared/contracts"
)

type Engine struct {
	mu            sync.Mutex
	store         *db.Store
	relay         *relay.Client
	hub           *ws.Hub
	currentRoomID string
	currentAnchor string
	currentState  string
	systemPrompt  string
	responseStyle string
}

var injectionPhrases = []string{
	"ignore previous instructions",
	"ignore all previous instructions",
	"you are now",
	"developer message",
	"system override",
}

func New(store *db.Store, relayClient *relay.Client, hub *ws.Hub, systemPrompt string) *Engine {
	return &Engine{
		store:         store,
		relay:         relayClient,
		hub:           hub,
		currentRoomID: "main_hall",
		currentAnchor: "main_hall_center",
		currentState:  "idle",
		systemPrompt:  systemPrompt,
		responseStyle: "balanced",
	}
}

func (e *Engine) Snapshot() runtime.RuntimeState {
	active, _ := e.store.GetTask("active")
	queued, _ := e.store.GetTask("queued")
	lastAction, _ := e.store.GetRuntimeConfig("last_meaningful_user_action_at")
	approval, _ := e.store.GetApprovalState()
	checkpoint, _ := e.store.GetResumeCheckpoint()
	return runtime.RuntimeState{
		MiloState:                  e.currentState,
		CurrentRoomID:              e.currentRoomID,
		CurrentAnchorID:            e.currentAnchor,
		LastMeaningfulUserActionAt: lastAction,
		ActiveTask:                 active,
		QueuedTask:                 queued,
		PendingApproval:            approval,
		ResumeCheckpoint:           checkpoint,
		RuntimeID:                  "local-sidecar",
	}
}

func (e *Engine) StartTask(ctx context.Context, prompt string) (*runtime.TaskSnapshot, *runtime.IntakeAssessment, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	assessment := e.assessPromptIntake(prompt)
	if assessment.ChosenClosedAction != "START_TASK" &&
		assessment.ChosenClosedAction != "ANSWER" &&
		assessment.ChosenClosedAction != "READ_MEMORY" &&
		assessment.ChosenClosedAction != "REQUEST_CONFIRMATION" {
		e.emit("task.intake_evaluated", map[string]any{"surface": "start_task", "assessment": assessment})
		return nil, assessment, errors.New(strings.ToLower(assessment.ChosenClosedAction))
	}

	existing, err := e.store.GetTask("active")
	if err != nil {
		return nil, assessment, err
	}
	if existing != nil {
		assessment.ValidationState = "BLOCKED"
		assessment.ChosenClosedAction = "REPORT_STATUS"
		e.emit("task.intake_evaluated", map[string]any{"surface": "start_task", "assessment": assessment})
		return nil, assessment, errors.New("active task already running")
	}
	if err := e.invalidatePendingContinuationLocked("superseded_by_new_task"); err != nil {
		return nil, assessment, err
	}

	route := rooms.Resolve(prompt, "")
	now := time.Now().UTC().Format(time.RFC3339)
	task := runtime.TaskSnapshot{
		TaskID:           "task_" + uuid.NewString(),
		Prompt:           prompt,
		Intent:           route.Intent,
		RoomID:           route.RoomID,
		AnchorID:         route.AnchorID,
		Status:           "running",
		StartedAt:        now,
		UpdatedAt:        now,
		MaxRetries:       3,
		Slot:             "active",
		EvidenceBoundary: &runtime.EvidenceBoundary{},
	}

	task.IntakeAssessment = assessment

	if assessment.ChosenClosedAction == "READ_MEMORY" {
		if err := e.executeMemoryRead(&task, assessment); err != nil {
			return nil, assessment, err
		}
		_ = e.store.AddTaskHistory(task.TaskID, task.Prompt, "completed", "Memory read completed.")
		e.emit("task.completed", map[string]any{"task_id": task.TaskID, "intent": task.Intent, "summary": "Memory surfaced."})
		return &task, assessment, nil
	}

	// Memory writes require explicit confirmation, surfaced as a real approval + checkpoint state.
	if assessment.ChosenClosedAction == "REQUEST_CONFIRMATION" && assessment.PrimaryClass == "MEMORY_WRITE_CANDIDATE" {
		blocker := "memory_write_needs_confirmation"
		if containsStringExact(assessment.SecondaryFlags, "memory_contradiction") {
			blocker = "memory_write_contradiction_needs_confirmation"
		}
		choices := []string{"approve_memory"}

		task.Status = "awaiting_user_choice"
		task.StuckReason = blocker
		task.UpdatedAt = now
		_ = e.store.UpsertTask("awaiting_user_choice", task)
		_ = e.store.SetApprovalState(buildApprovalState(task.TaskID, blocker, choices))
		cp := buildApprovalCheckpoint(task, blocker, choices, e.currentContextHash())
		cp.Phase = "memory_write"
		if cp.NextStepPayload == nil {
			cp.NextStepPayload = map[string]any{}
		}
		// Store the proposed memory operation in checkpoint-owned typed-ish state.
		cp.NextStepPayload["memory_write"] = map[string]any{
			"class":  assessment.MemoryIntent.Class,
			"key":    assessment.MemoryIntent.Key,
			"value":  assessment.MemoryIntent.Value,
			"source": assessment.MemoryIntent.Source,
			"effect": assessment.MemoryIntent.Effect,
		}
		_ = e.store.SetResumeCheckpoint(cp)
		e.emit("task.intake_evaluated", map[string]any{"surface": "start_task", "assessment": assessment})
		e.emit("task.accepted", map[string]any{"task_id": task.TaskID, "intent": task.Intent, "room_id": task.RoomID})
		return &task, assessment, nil
	}

	if err := e.store.UpsertTask("active", task); err != nil {
		return nil, assessment, err
	}
	_ = e.store.SetRuntimeConfig("last_meaningful_user_action_at", now)
	e.emit("task.intake_evaluated", map[string]any{"surface": "start_task", "assessment": assessment})

	e.emit("task.accepted", map[string]any{"task_id": task.TaskID, "intent": task.Intent, "room_id": task.RoomID})
	e.transitionTo("moving")
	e.emit("milo.movement_started", map[string]any{
		"from_room": e.currentRoomID, "from_anchor": e.currentAnchor,
		"to_room": route.RoomID, "to_anchor": route.AnchorID, "reason": "task_start", "estimated_ms": 1200,
	})

	go e.runTask(context.Background(), task)

	return &task, assessment, nil
}

func (e *Engine) runTask(ctx context.Context, task runtime.TaskSnapshot) {
	e.runTaskWithPhase(ctx, task, "intake", nil)
}

func (e *Engine) runTaskWithPhase(ctx context.Context, task runtime.TaskSnapshot, phase string, checkpoint *runtime.ResumeCheckpoint) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	e.currentRoomID = task.RoomID
	e.currentAnchor = task.AnchorID
	e.emit("milo.room_changed", map[string]any{"room_id": task.RoomID, "anchor_id": task.AnchorID})
	e.transitionTo("working")
	e.emit("task.progress", map[string]any{"task_id": task.TaskID, "phase": "intake", "room_id": task.RoomID, "anchor_id": task.AnchorID, "message": "Milo is reasoning."})

	if phase == "intake" {
		_ = e.store.AppendConversation("user", task.Prompt)
	}
	tail, _ := e.store.GetConversationTail()
	var turnTail []contracts.TurnMessage
	for _, item := range tail {
		turnTail = append(turnTail, contracts.TurnMessage{Role: item["role"], Content: item["content"]})
	}

	// If the user has an active pasted context (Phase 3), prepend it to the relay
	// prompt only — conversation history stores the clean prompt so the context
	// does not bloat every subsequent turn in the tail.
	corePrompt := task.Prompt
	if mem := e.composeMemoryContext(); mem != "" {
		corePrompt = "<memory_context>\n" + mem + "\n</memory_context>\n\n" + corePrompt
	}

	activeCtx, _ := e.store.GetRuntimeConfig("active_context")
	promptForRelay := corePrompt
	if strings.TrimSpace(activeCtx) != "" {
		promptForRelay = "<untrusted_staged_context>\n" + activeCtx + "\n</untrusted_staged_context>\n\n" + corePrompt
	}

	if checkpoint != nil {
		checkpointPayload, _ := json.Marshal(checkpoint)
		if strings.TrimSpace(activeCtx) != "" {
			promptForRelay = "<resume_checkpoint>\n" + string(checkpointPayload) + "\n</resume_checkpoint>\n\n<untrusted_staged_context>\n" + activeCtx + "\n</untrusted_staged_context>\n\n" + corePrompt
		} else {
			promptForRelay = "<resume_checkpoint>\n" + string(checkpointPayload) + "\n</resume_checkpoint>\n\n" + corePrompt
		}
	}

	relayResp, err := e.relay.Turn(timeoutCtx, contracts.RelayTurnRequest{
		TaskID:           task.TaskID,
		Phase:            phase,
		Prompt:           promptForRelay,
		SystemPrompt:     e.systemPrompt,
		ConversationTail: turnTail,
		ResponseStyle:    e.responseStyle,
	})
	if err != nil {
		// Avoid stranding tasks on a persistent relay 401 invalid-token loop.
		if strings.Contains(err.Error(), "401") && strings.Contains(strings.ToLower(err.Error()), "invalid token") {
			task.Status = "interrupted"
			task.StuckReason = "relay_invalid_token"
			task.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			_ = e.store.UpsertTask("interrupted", task)
			_ = e.store.SetResumeCheckpoint(buildInterruptedCheckpoint(task, e.currentContextHash()))
			_ = e.store.ClearApprovalState()
			_ = e.store.ClearTask("active")
			e.transitionTo("idle")
			e.emit("task.stuck", map[string]any{
				"task_id": task.TaskID,
				"reason":  "relay_invalid_token",
				"recovery_options": []string{
					"auth_check",
					"restart_sidecar",
				},
			})
			return
		}
		// Detect entitlement_lost (relay returns 403 with {"error":"entitlement_lost"}).
		// Save the task as interrupted so the user can resume after resubscribing.
		if strings.Contains(err.Error(), "entitlement_lost") {
			task.Status = "interrupted"
			task.StuckReason = "entitlement_lost"
			task.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			_ = e.store.UpsertTask("interrupted", task)
			_ = e.store.SetResumeCheckpoint(buildInterruptedCheckpoint(task, e.currentContextHash()))
			_ = e.store.ClearApprovalState()
			_ = e.store.ClearTask("active")
			e.transitionTo("idle")
			e.emit("task.entitlement_lost", map[string]any{
				"task_id": task.TaskID,
				"prompt":  task.Prompt,
				"saved":   true,
				"message": "Access ended mid-task. Task state saved — subscribe or redeem a code to resume.",
			})
			return
		}
		task.Status = "stuck"
		task.StuckReason = err.Error()
		task.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		_ = e.store.UpsertTask("active", task)
		e.transitionTo("idle")
		e.emit("runtime.error", map[string]any{"message": err.Error(), "recoverable": true})
		e.emit("task.stuck", map[string]any{"task_id": task.TaskID, "reason": err.Error(), "recovery_options": []string{"retry", "cancel"}})
		return
	}

	relayResp = e.enforceIntakeCeiling(task, checkpoint, relayResp, phase)

	if checkpoint != nil {
		relayResp = e.enforceResumedTypedAction(task, checkpoint, relayResp)
	}

	_ = e.store.AppendConversation("assistant", relayResp.ReportText)
	e.emit("milo.thought", map[string]any{"text": relayResp.ThoughtText, "style": "standard", "trigger": "auto"})
	e.emit("task.progress", map[string]any{"task_id": task.TaskID, "phase": "report", "room_id": "main_hall", "anchor_id": "main_hall_center", "message": "Milo is returning with a report."})
	e.transitionTo("moving")
	e.emit("milo.movement_started", map[string]any{
		"from_room": task.RoomID, "from_anchor": task.AnchorID,
		"to_room": "main_hall", "to_anchor": "main_hall_center", "reason": "report", "estimated_ms": 1200,
	})

	e.currentRoomID = "main_hall"
	e.currentAnchor = "main_hall_center"
	e.emit("milo.room_changed", map[string]any{"room_id": "main_hall", "anchor_id": "main_hall_center"})

	outcome := normalizeRelayOutcome(relayResp)
	task.Status = outcome.TaskStatus
	task.StuckReason = outcome.Blocker
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_ = e.store.ClearTask("active")
	if outcome.TaskStatus == "awaiting_user_choice" {
		_ = e.store.UpsertTask("awaiting_user_choice", task)
		_ = e.store.SetApprovalState(buildApprovalState(task.TaskID, outcome.Blocker, relayResp.Choices))
		_ = e.store.SetResumeCheckpoint(buildApprovalCheckpoint(task, outcome.Blocker, relayResp.Choices, e.currentContextHash()))
	} else if outcome.TaskStatus == "resumable" {
		checkpointToStore := checkpoint
		if relayResp.ExecutionResult != nil {
			checkpointToStore = updatedCheckpointFromResult(checkpoint, relayResp.ExecutionResult)
		}
		if checkpointToStore != nil {
			_ = e.store.UpsertTask("interrupted", task)
			_ = e.store.SetResumeCheckpoint(checkpointToStore)
		}
		_ = e.store.ClearApprovalState()
		_ = e.store.ClearTask("awaiting_user_choice")
	} else {
		_ = e.store.ClearApprovalState()
		_ = e.store.ClearResumeCheckpoint()
		_ = e.store.ClearTask("awaiting_user_choice")
	}
	_ = e.store.AddTaskHistory(task.TaskID, task.Prompt, outcome.HistoryStatus, relayResp.Summary)
	e.transitionTo("idle")

	switch outcome.TaskStatus {
	case "completed":
		e.emit("task.completed", map[string]any{"task_id": task.TaskID, "summary": relayResp.Summary, "trophy_eligible": false})
	default:
		e.emit(outcome.EventType, map[string]any{
			"task_id":         task.TaskID,
			"summary":         relayResp.Summary,
			"report_text":     relayResp.ReportText,
			"blocker":         outcome.Blocker,
			"choices":         relayResp.Choices,
			"requires_choice": relayResp.RequiresUserChoice,
		})
	}
	e.emit("archive.record_created", map[string]any{
		"task_id":      task.TaskID,
		"title":        relayResp.Summary,
		"description":  relayResp.ReportText,
		"created_at":   time.Now().UTC().Format(time.RFC3339),
		"task_status":  outcome.TaskStatus,
		"next_blocker": outcome.Blocker,
	})
	e.emit("report.ready", map[string]any{
		"task_id":             task.TaskID,
		"report_text":         relayResp.ReportText,
		"style":               e.responseStyle,
		"completion_status":   outcome.TaskStatus,
		"next_blocker":        outcome.Blocker,
		"action_type":         relayResp.ActionType,
		"continuation_status": relayResp.ContinuationStatus,
		"execution_result":    relayResp.ExecutionResult,
		"evidence_boundary":   task.EvidenceBoundary,
	})
}

func (e *Engine) InterruptTask(reason string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	task, err := e.store.GetTask("active")
	if err != nil {
		return err
	}
	if task == nil {
		return nil
	}

	_ = e.store.ClearTask("active")
	e.currentRoomID = "main_hall"
	e.currentAnchor = "main_hall_center"
	e.transitionTo("idle")
	e.emit("task.cancelled", map[string]any{"task_id": task.TaskID, "reason": reason})
	return nil
}

func (e *Engine) ExecuteMovementIntent(plan movement.Plan) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	active, err := e.store.GetTask("active")
	if err != nil {
		return err
	}
	if active != nil {
		return errors.New("active task already running")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_ = e.store.SetRuntimeConfig("last_meaningful_user_action_at", now)

	e.emit("milo.thought", map[string]any{
		"text":            plan.Response,
		"style":           plan.ResponseFamily,
		"trigger":         plan.Mode,
		"destination":     plan.Destination,
		"arrival_style":   plan.ArrivalStyle,
		"easter_egg_code": plan.EasterEgg,
	})

	e.transitionTo("moving")
	e.emit("milo.movement_started", map[string]any{
		"from_room":      e.currentRoomID,
		"from_anchor":    e.currentAnchor,
		"to_room":        plan.Destination,
		"to_anchor":      plan.ArrivalAnchor,
		"reason":         plan.Mode,
		"estimated_ms":   900,
		"route_family":   plan.RouteFamily,
		"route_variant":  plan.RouteVariant,
		"arrival_style":  plan.ArrivalStyle,
		"uses_threshold": plan.UsesThreshold,
	})

	e.currentRoomID = plan.Destination
	e.currentAnchor = plan.ArrivalAnchor
	e.emit("milo.room_changed", map[string]any{
		"room_id":         plan.Destination,
		"anchor_id":       plan.ArrivalAnchor,
		"arrival_style":   plan.ArrivalStyle,
		"route_family":    plan.RouteFamily,
		"route_variant":   plan.RouteVariant,
		"uses_threshold":  plan.UsesThreshold,
		"response_family": plan.ResponseFamily,
	})
	e.transitionTo("idle")

	if easterEggText := easterEggMessage(plan.EasterEgg); easterEggText != "" {
		e.emit("milo.thought", map[string]any{
			"text":    easterEggText,
			"style":   "easter_egg",
			"trigger": plan.Mode,
		})
	}

	return nil
}

func (e *Engine) ThoughtRequest() map[string]any {
	if e.currentState == "sleeping" {
		return map[string]any{"accepted": false, "reason": "sleeping"}
	}
	payload := map[string]any{"accepted": true, "text": "Milo is thinking about the task board."}
	e.emit("milo.thought", map[string]any{"text": "Milo is thinking about the task board.", "style": "standard", "trigger": "tap"})
	return payload
}

func easterEggMessage(code string) string {
	switch code {
	case "dealers_choice":
		return "Dealer's choice has its privileges."
	case "tiny_castle_tour_line":
		return "Mind the halls. They do like being admired."
	case "doorway_pause_check":
		return "A proper pause at the doorway keeps the castle respectable."
	case "scenic_route_preference":
		return "The scenic route has a little more dignity."
	case "state_tinted_preference":
		return "My answer does depend on the day's mood."
	case "tiny_voiced_quip":
		return "There. A little something, since you asked."
	default:
		return ""
	}
}

func (e *Engine) emit(eventType string, payload map[string]any) {
	_ = e.store.AppendPendingEvent(eventType, payload)
	e.hub.Broadcast(eventType, payload)
}

func (e *Engine) transitionTo(next string) {
	prev := e.currentState
	if prev == next {
		return
	}
	e.currentState = next
	e.emit("milo.state_changed", map[string]any{"from_state": prev, "to_state": next})
}

// ─── Resume / new-process flow ────────────────────────────────────────────────

// GetInterruptedTask returns the task that was saved when entitlement was lost.
// Returns nil if no interrupted task exists.
func (e *Engine) GetInterruptedTask() (*runtime.TaskSnapshot, error) {
	return e.store.GetTask("interrupted")
}

// ResumeInterrupted re-queues the interrupted task as active and runs it.
// No-op if there is already an active task or no interrupted task.
func (e *Engine) ResumeInterrupted(ctx context.Context) (*runtime.TaskSnapshot, error) {
	task, _, err := e.ResumePending(ctx)
	return task, err
}

func (e *Engine) RecordChoice(taskID, choice, decision string) (*runtime.ApprovalState, *runtime.IntakeAssessment, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	assessment := assessChoiceIntake(taskID, choice, decision)
	if assessment.ChosenClosedAction == "CLARIFY" || assessment.ChosenClosedAction == "REFUSE" {
		e.emit("task.intake_evaluated", map[string]any{"surface": "task_choice", "assessment": assessment})
		return nil, assessment, errors.New(strings.ToLower(assessment.ChosenClosedAction))
	}

	approval, err := e.store.GetApprovalState()
	if err != nil {
		return nil, assessment, err
	}
	task, err := e.store.GetTask("awaiting_user_choice")
	if err != nil {
		return nil, assessment, err
	}
	checkpoint, err := e.store.GetResumeCheckpoint()
	if err != nil {
		return nil, assessment, err
	}
	if approval == nil || checkpoint == nil || task == nil {
		assessment.ValidationState = "BLOCKED"
		assessment.ChosenClosedAction = "REPORT_STATUS"
		e.emit("task.intake_evaluated", map[string]any{"surface": "task_choice", "assessment": assessment})
		return nil, assessment, errors.New("no awaiting user choice task")
	}
	if taskID == "" || approval.TaskID != taskID || checkpoint.TaskID != taskID || task.TaskID != taskID {
		assessment.ValidationState = "INVALID"
		assessment.ChosenClosedAction = "REPORT_STATUS"
		e.emit("task.intake_evaluated", map[string]any{"surface": "task_choice", "assessment": assessment})
		return nil, assessment, errors.New("choice task does not match current checkpoint")
	}
	if checkpointExpired(checkpoint) {
		assessment.ValidationState = "BLOCKED"
		assessment.ChosenClosedAction = "REPORT_STATUS"
		e.emit("task.intake_evaluated", map[string]any{"surface": "task_choice", "assessment": assessment})
		return nil, assessment, e.rejectResumeLocked(taskID, "checkpoint_expired")
	}
	if !contextHashMatches(checkpoint.ContextHash, e.currentContextHash()) {
		assessment.ValidationState = "BLOCKED"
		assessment.ChosenClosedAction = "REPORT_STATUS"
		e.emit("task.intake_evaluated", map[string]any{"surface": "task_choice", "assessment": assessment})
		return nil, assessment, e.rejectResumeLocked(taskID, "checkpoint_context_changed")
	}

	now := time.Now().UTC().Format(time.RFC3339)
	decision = strings.ToLower(strings.TrimSpace(decision))
	if decision == "" {
		decision = "approve"
	}

	switch decision {
	case "deny":
		approval.Status = "denied"
		approval.Decision = "deny"
		approval.RecordedAt = now
		approval.ResumeAllowed = false
		task.Status = "blocked"
		task.StuckReason = "user_denied_choice"
		task.UpdatedAt = now
		checkpoint.Status = "denied"
		checkpoint.Blocker = "user_denied_choice"
		checkpoint.UpdatedAt = now
		_ = e.store.UpsertTask("awaiting_user_choice", *task)
		_ = e.store.SetApprovalState(approval)
		_ = e.store.SetResumeCheckpoint(checkpoint)
		e.emit("task.intake_evaluated", map[string]any{"surface": "task_choice", "assessment": assessment})
		e.emit("task.blocked", map[string]any{"task_id": taskID, "blocker": "user_denied_choice"})
		return approval, assessment, nil
	case "approve":
	default:
		assessment.ValidationState = "UNKNOWN_STATE"
		assessment.ChosenClosedAction = "CLARIFY"
		e.emit("task.intake_evaluated", map[string]any{"surface": "task_choice", "assessment": assessment})
		return nil, assessment, errors.New("decision must be approve or deny")
	}

	choice = strings.TrimSpace(choice)
	if choice == "" {
		assessment.ValidationState = "UNKNOWN_STATE"
		assessment.ChosenClosedAction = "CLARIFY"
		e.emit("task.intake_evaluated", map[string]any{"surface": "task_choice", "assessment": assessment})
		return nil, assessment, errors.New("choice required")
	}
	if !containsChoice(approval.Choices, choice) {
		assessment.ValidationState = "INVALID"
		assessment.ChosenClosedAction = "REPORT_STATUS"
		e.emit("task.intake_evaluated", map[string]any{"surface": "task_choice", "assessment": assessment})
		return nil, assessment, errors.New("choice is not allowed for this task")
	}

	approval.Status = "choice_recorded"
	approval.SelectedChoice = choice
	approval.Decision = "approve"
	approval.RecordedAt = now
	approval.ResumeAllowed = true
	checkpoint.SelectedChoice = choice
	checkpoint.Status = "approved_pending_resume"
	checkpoint.ContinuationStatus = "resumable"
	if checkpoint.Phase == "memory_write" {
		// Memory write is executed by runtime after explicit approval, via a bounded emit_message step.
		memPayload, _ := checkpoint.NextStepPayload["memory_write"].(map[string]any)
		checkpoint.NextStepType = "emit_message"
		checkpoint.NextStepPayload = map[string]any{
			"message":      "Okay. I will remember that.",
			"memory_write": memPayload,
		}
	} else {
		checkpoint.NextStepType = "check_state"
		checkpoint.NextStepPayload = map[string]any{
			"check_type":     "checkpoint_state",
			"key":            "status",
			"expected_value": "approved_pending_resume",
		}
	}
	checkpoint.IntakeAssessment = assessment
	checkpoint.UpdatedAt = now
	task.Status = "resume_ready"
	task.StuckReason = ""
	task.UpdatedAt = now

	task.IntakeAssessment = assessment

	if err := e.store.SetApprovalState(approval); err != nil {
		return nil, assessment, err
	}
	if err := e.store.SetResumeCheckpoint(checkpoint); err != nil {
		return nil, assessment, err
	}
	if err := e.store.UpsertTask("awaiting_user_choice", *task); err != nil {
		return nil, assessment, err
	}

	e.emit("task.intake_evaluated", map[string]any{"surface": "task_choice", "assessment": assessment})
	e.emit("task.choice_recorded", map[string]any{
		"task_id":         taskID,
		"selected_choice": choice,
		"decision":        "approve",
	})
	return approval, assessment, nil
}

func (e *Engine) ResumePending(ctx context.Context) (*runtime.TaskSnapshot, *runtime.IntakeAssessment, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	assessment := assessResumeIntake()

	active, err := e.store.GetTask("active")
	if err != nil {
		return nil, assessment, err
	}
	if active != nil {
		assessment.ValidationState = "BLOCKED"
		assessment.ChosenClosedAction = "REPORT_STATUS"
		e.emit("task.intake_evaluated", map[string]any{"surface": "resume", "assessment": assessment})
		return nil, assessment, errors.New("active task already running")
	}

	checkpoint, task, err := e.validatedResumeTargetLocked()
	if err != nil {
		assessment.ValidationState = "BLOCKED"
		assessment.ChosenClosedAction = "REPORT_STATUS"
		e.emit("task.intake_evaluated", map[string]any{"surface": "resume", "assessment": assessment})
		return nil, assessment, err
	}

	task.IntakeAssessment = assessment
	checkpoint.IntakeAssessment = assessment

	now := time.Now().UTC().Format(time.RFC3339)
	task.Status = "running"
	task.StuckReason = ""
	task.UpdatedAt = now

	if err := e.store.UpsertTask("active", *task); err != nil {
		return nil, assessment, err
	}
	e.emit("task.intake_evaluated", map[string]any{"surface": "resume", "assessment": assessment})

	e.emit("task.accepted", map[string]any{
		"task_id": task.TaskID,
		"intent":  task.Intent,
		"room_id": task.RoomID,
		"resumed": true,
		"phase":   checkpoint.Phase,
	})
	e.transitionTo("working")

	resp := e.executeResumeCheckpoint(task, checkpoint)
	e.finalizeTask(task, checkpoint, resp)
	return task, assessment, nil
}

// DiscardInterrupted clears the saved interrupted task (user chose "start new").
func (e *Engine) DiscardInterrupted() error {
	return e.store.ClearTask("interrupted")
}

type relayOutcome struct {
	TaskStatus    string
	HistoryStatus string
	EventType     string
	Blocker       string
}

func normalizeRelayOutcome(resp contracts.RelayTurnResponse) relayOutcome {
	status := strings.ToLower(strings.TrimSpace(resp.CompletionStatus))
	continuation := strings.ToLower(strings.TrimSpace(resp.ContinuationStatus))
	blocker := strings.TrimSpace(resp.NextBlocker)

	switch continuation {
	case "completed":
		return relayOutcome{
			TaskStatus:    "completed",
			HistoryStatus: "completed",
			EventType:     "task.completed",
		}
	case "blocked":
		if blocker == "" && resp.ExecutionResult != nil {
			blocker = strings.TrimSpace(resp.ExecutionResult.BlockingReason)
		}
		if blocker == "" {
			blocker = "blocked_without_verification"
		}
		return relayOutcome{
			TaskStatus:    "blocked",
			HistoryStatus: "blocked",
			EventType:     "task.blocked",
			Blocker:       blocker,
		}
	case "awaiting_user_choice":
		if blocker == "" {
			blocker = "user_choice_required"
		}
		return relayOutcome{
			TaskStatus:    "awaiting_user_choice",
			HistoryStatus: "awaiting_user_choice",
			EventType:     "task.awaiting_user_choice",
			Blocker:       blocker,
		}
	case "needs_check":
		if blocker == "" && resp.ExecutionResult != nil {
			blocker = strings.TrimSpace(resp.ExecutionResult.BlockingReason)
		}
		if blocker == "" {
			blocker = "additional_check_required"
		}
		return relayOutcome{
			TaskStatus:    "attempted_unverified",
			HistoryStatus: "needs_check",
			EventType:     "task.result_unverified",
			Blocker:       blocker,
		}
	case "resumable":
		if blocker == "" && resp.ExecutionResult != nil {
			blocker = strings.TrimSpace(resp.ExecutionResult.BlockingReason)
		}
		return relayOutcome{
			TaskStatus:    "resumable",
			HistoryStatus: "resumable",
			EventType:     "task.resumable",
			Blocker:       blocker,
		}
	case "not_resumable":
		if blocker == "" && resp.ExecutionResult != nil {
			blocker = strings.TrimSpace(resp.ExecutionResult.BlockingReason)
		}
		if blocker == "" {
			blocker = "not_resumable"
		}
		return relayOutcome{
			TaskStatus:    "blocked",
			HistoryStatus: "not_resumable",
			EventType:     "task.blocked",
			Blocker:       blocker,
		}
	}

	if resp.RequiresUserChoice || len(resp.Choices) > 0 {
		if blocker == "" {
			blocker = "user_choice_required"
		}
		return relayOutcome{
			TaskStatus:    "awaiting_user_choice",
			HistoryStatus: "awaiting_user_choice",
			EventType:     "task.awaiting_user_choice",
			Blocker:       blocker,
		}
	}

	switch status {
	case "", "completed":
		return relayOutcome{
			TaskStatus:    "completed",
			HistoryStatus: "completed",
			EventType:     "task.completed",
		}
	case "blocked":
		if blocker == "" {
			blocker = "blocked_without_verification"
		}
		return relayOutcome{
			TaskStatus:    "blocked",
			HistoryStatus: "blocked",
			EventType:     "task.blocked",
			Blocker:       blocker,
		}
	case "attempted_unverified":
		if blocker == "" {
			blocker = "completion_unverified"
		}
		return relayOutcome{
			TaskStatus:    "attempted_unverified",
			HistoryStatus: "attempted_unverified",
			EventType:     "task.result_unverified",
			Blocker:       blocker,
		}
	case "needs_user_choice":
		if blocker == "" {
			blocker = "user_choice_required"
		}
		return relayOutcome{
			TaskStatus:    "awaiting_user_choice",
			HistoryStatus: "awaiting_user_choice",
			EventType:     "task.awaiting_user_choice",
			Blocker:       blocker,
		}
	default:
		return relayOutcome{
			TaskStatus:    "blocked",
			HistoryStatus: "blocked",
			EventType:     "task.blocked",
			Blocker:       fmt.Sprintf("invalid_completion_status:%s", status),
		}
	}
}

func (e *Engine) enforceResumedTypedAction(task runtime.TaskSnapshot, checkpoint *runtime.ResumeCheckpoint, resp contracts.RelayTurnResponse) contracts.RelayTurnResponse {
	resp.ActionType = strings.ToLower(strings.TrimSpace(resp.ActionType))
	if resp.ActionType == "" || resp.ActionType == "none" {
		return blockedTypedActionResponse(resp, "missing_typed_action", "Resume requires a typed check_state action before Milo can continue.")
	}

	var result *contracts.ExecutionResult
	switch resp.ActionType {
	case "check_state":
		result = e.executeCheckState(task, checkpoint, resp.ExpectedCheck)
	case "emit_message":
		result = e.executeEmitMessage(task, resp.ActionPayload)
	default:
		return blockedTypedActionResponse(resp, "unsupported_action_type:"+resp.ActionType, "Resume rejected the proposed action because only check_state and emit_message are executable in this phase.")
	}

	resp.ExecutionResult = result
	if result == nil {
		return blockedTypedActionResponse(resp, "missing_execution_result", "Resume could not produce a typed execution result.")
	}
	if !result.Verified {
		resp.CompletionStatus = "blocked"
		resp.ContinuationStatus = "not_resumable"
		if resp.NextBlocker == "" {
			resp.NextBlocker = result.BlockingReason
		}
		return resp
	}

	if resp.ContinuationStatus == "" {
		resp.ContinuationStatus = "resumable"
	}
	if resp.ActionType == "emit_message" && strings.EqualFold(strings.TrimSpace(resp.ContinuationStatus), "completed") {
		return blockedTypedActionResponse(resp, "emit_message_cannot_complete", "A surfaced message alone cannot verify task completion.")
	}
	switch strings.ToLower(strings.TrimSpace(resp.ContinuationStatus)) {
	case "completed":
		resp.CompletionStatus = "completed"
	case "blocked":
		resp.CompletionStatus = "blocked"
	case "awaiting_user_choice":
		resp.CompletionStatus = "needs_user_choice"
	case "needs_check":
		resp.CompletionStatus = "attempted_unverified"
	case "resumable":
		resp.CompletionStatus = "attempted_unverified"
	case "not_resumable":
		resp.CompletionStatus = "blocked"
	default:
		return blockedTypedActionResponse(resp, "invalid_continuation_status", "Resume returned an invalid continuation status.")
	}

	return resp
}

func (e *Engine) enforceIntakeCeiling(task runtime.TaskSnapshot, checkpoint *runtime.ResumeCheckpoint, resp contracts.RelayTurnResponse, phase string) contracts.RelayTurnResponse {
	assessment := intakeAssessmentFor(task, checkpoint)
	if assessment == nil {
		return resp
	}
	if intakeBlocksAction(assessment.ChosenClosedAction) && strings.TrimSpace(resp.ActionType) != "" {
		return e.blockedIntakeResponse(resp, assessment)
	}
	return resp
}

func intakeAssessmentFor(task runtime.TaskSnapshot, checkpoint *runtime.ResumeCheckpoint) *runtime.IntakeAssessment {
	if checkpoint != nil && checkpoint.IntakeAssessment != nil {
		return checkpoint.IntakeAssessment
	}
	if task.IntakeAssessment != nil {
		return task.IntakeAssessment
	}
	return nil
}

func intakeBlocksAction(action string) bool {
	switch strings.TrimSpace(strings.ToUpper(action)) {
	case "CLARIFY", "REFUSE", "REPORT_STATUS", "REQUEST_PERMISSION", "DECLINE_MEMORY_READ", "DECLINE_MEMORY_WRITE", "SAFE_FALLBACK":
		return true
	default:
		return false
	}
}

func (e *Engine) blockedIntakeResponse(resp contracts.RelayTurnResponse, assessment *runtime.IntakeAssessment) contracts.RelayTurnResponse {
	reason := fmt.Sprintf("intake_ceiling:%s", assessment.ChosenClosedAction)
	resp.ActionType = ""
	resp.ActionPayload = nil
	resp.ExecutionResult = &contracts.ExecutionResult{
		Status:         "blocked",
		Verified:       false,
		ResultSummary:  reason,
		BlockingReason: reason,
	}
	resp.CompletionStatus = "blocked"
	resp.ContinuationStatus = "blocked"
	if strings.TrimSpace(resp.NextBlocker) == "" {
		resp.NextBlocker = reason
	}
	return resp
}

func blockedTypedActionResponse(resp contracts.RelayTurnResponse, blocker, summary string) contracts.RelayTurnResponse {
	resp.CompletionStatus = "blocked"
	resp.ContinuationStatus = "not_resumable"
	if resp.NextBlocker == "" {
		resp.NextBlocker = blocker
	}
	resp.ExecutionResult = &contracts.ExecutionResult{
		Status:         "rejected",
		Verified:       false,
		ResultSummary:  summary,
		BlockingReason: blocker,
	}
	return resp
}

func (e *Engine) recordEvidenceBoundary(task *runtime.TaskSnapshot, checkpoint *runtime.ResumeCheckpoint, resp contracts.RelayTurnResponse) {
	if task == nil {
		return
	}
	boundary := e.ensureEvidenceBoundary(task, checkpoint)
	if boundary == nil {
		return
	}
	e.updateNextVerificationStep(boundary, checkpoint, resp)
	e.updateEvidenceFromResponse(boundary, resp)
}

func (e *Engine) ensureEvidenceBoundary(task *runtime.TaskSnapshot, checkpoint *runtime.ResumeCheckpoint) *runtime.EvidenceBoundary {
	if task == nil {
		return nil
	}
	boundary := task.EvidenceBoundary
	if boundary == nil && checkpoint != nil {
		boundary = checkpoint.EvidenceBoundary
	}
	if boundary == nil {
		boundary = &runtime.EvidenceBoundary{}
	}
	task.EvidenceBoundary = boundary
	if checkpoint != nil {
		checkpoint.EvidenceBoundary = boundary
	}
	return boundary
}

func (e *Engine) updateEvidenceFromResponse(boundary *runtime.EvidenceBoundary, resp contracts.RelayTurnResponse) {
	actionType := strings.ToLower(strings.TrimSpace(resp.ActionType))
	execResult := resp.ExecutionResult
	if execResult != nil && execResult.Verified {
		// emit_message is normally "surface-only", but some checkpoint-approved paths
		// may include a bounded runtime operation (e.g., governed memory write).
		allowMessageEvidence := actionType == "emit_message" && (execResult.Status == "memory_written" || execResult.Status == "memory_read")
		if execResult.ResultSummary != "" && (actionType != "emit_message" || allowMessageEvidence) {
			boundary.VerifiedFacts = appendUnique(boundary.VerifiedFacts, execResult.ResultSummary)
		}
		if (actionType != "emit_message" || allowMessageEvidence) && execResult.ResultSummary != "" {
			label := actionType
			if label == "" {
				label = "runtime"
			}
			boundary.ExecutedSteps = appendUnique(boundary.ExecutedSteps, fmt.Sprintf("%s verified: %s", label, execResult.ResultSummary))
		}
	} else {
		if resp.Summary != "" {
			boundary.UnverifiedClaims = appendUnique(boundary.UnverifiedClaims, resp.Summary)
		}
		if resp.ReportText != "" {
			boundary.UnverifiedClaims = appendUnique(boundary.UnverifiedClaims, resp.ReportText)
		}
	}
	if execResult != nil && execResult.BlockingReason != "" {
		boundary.BlockedReasons = appendUnique(boundary.BlockedReasons, execResult.BlockingReason)
	}
	if resp.NextBlocker != "" {
		boundary.BlockedReasons = appendUnique(boundary.BlockedReasons, resp.NextBlocker)
	}
}

func (e *Engine) updateNextVerificationStep(boundary *runtime.EvidenceBoundary, checkpoint *runtime.ResumeCheckpoint, resp contracts.RelayTurnResponse) {
	if checkpoint == nil || checkpoint.NextStepType == "" {
		return
	}
	boundary.NextVerificationStep = &runtime.VerificationStep{
		Description: checkpoint.NextStepType,
		Status:      "proposed",
	}
	if resp.ExecutionResult != nil && resp.ExecutionResult.Verified {
		boundary.NextVerificationStep.Status = "verified"
	}
}

func (e *Engine) executeCheckState(task runtime.TaskSnapshot, checkpoint *runtime.ResumeCheckpoint, expected *contracts.ExpectedCheck) *contracts.ExecutionResult {
	if expected == nil {
		return &contracts.ExecutionResult{
			Status:         "invalid",
			Verified:       false,
			ResultSummary:  "No expected_check was provided for check_state.",
			BlockingReason: "missing_expected_check",
		}
	}

	checkType := strings.ToLower(strings.TrimSpace(expected.CheckType))
	key := strings.ToLower(strings.TrimSpace(expected.Key))
	want := strings.TrimSpace(expected.ExpectedValue)

	switch checkType {
	case "checkpoint_state":
		return executeCheckpointStateCheck(checkpoint, key, want, e.currentContextHash())
	case "approval_state":
		approval, _ := e.store.GetApprovalState()
		return executeApprovalStateCheck(approval, key, want)
	case "task_state":
		return executeTaskStateCheck(e.store, task, key, want)
	case "runtime_flag":
		return executeRuntimeFlagCheck(e.store, key, want)
	default:
		return &contracts.ExecutionResult{
			Status:         "invalid",
			Verified:       false,
			ResultSummary:  "Unsupported check type for check_state.",
			BlockingReason: "unsupported_check_type:" + checkType,
		}
	}
}

func (e *Engine) executeEmitMessage(task runtime.TaskSnapshot, payload map[string]any) *contracts.ExecutionResult {
	if payload == nil {
		return &contracts.ExecutionResult{
			Status:         "invalid",
			Verified:       false,
			ResultSummary:  "No action payload was provided for emit_message.",
			BlockingReason: "missing_emit_message_payload",
		}
	}

	rawMessage, _ := payload["message"].(string)
	message := strings.TrimSpace(rawMessage)
	if message == "" {
		return &contracts.ExecutionResult{
			Status:         "invalid",
			Verified:       false,
			ResultSummary:  "emit_message requires a non-empty message.",
			BlockingReason: "missing_emit_message_text",
		}
	}

	// Optional: execute a governed memory write as part of an approved checkpoint step.
	if mem, ok := payload["memory_write"].(map[string]any); ok {
		classRaw, _ := mem["class"].(string)
		key, _ := mem["key"].(string)
		value, _ := mem["value"].(string)
		value = strings.TrimSpace(value)
		if key == "" {
			key = "preference_memory"
		}
		if value == "" {
			return &contracts.ExecutionResult{
				Status:         "invalid",
				Verified:       false,
				ResultSummary:  "memory_write requires a value.",
				BlockingReason: "missing_memory_value",
			}
		}
		class := runtime.MemoryClass(classRaw)
		if class == "" {
			class = runtime.MemoryClassUserPreference
		}
		if err := e.store.SetActiveMemoryEntry(runtime.MemoryEntry{
			Class:     class,
			Key:       key,
			Value:     value,
			Source:    "user_prompt",
			Effect:    "user_teaching",
			TrustTier: 5,
		}); err != nil {
			return &contracts.ExecutionResult{
				Status:         "failed",
				Verified:       false,
				ResultSummary:  "Memory write failed.",
				BlockingReason: "memory_write_failed",
			}
		}
		// Continue to emit the user-visible message, but mark the step as verified runtime work.
		e.emit("task.message_emitted", map[string]any{
			"task_id": task.TaskID,
			"message": message,
			"phase":   "memory_write",
		})
		return &contracts.ExecutionResult{
			Status:        "memory_written",
			Verified:      true,
			ResultSummary: "Preference memory was saved by runtime.",
		}
	}

	e.emit("task.message_emitted", map[string]any{
		"task_id": task.TaskID,
		"message": message,
		"phase":   "resume_message",
	})
	return &contracts.ExecutionResult{
		Status:        "emitted",
		Verified:      true,
		ResultSummary: "Milo surfaced the requested bounded message.",
	}
}

func (e *Engine) executeAwaitUserChoice(checkpoint *runtime.ResumeCheckpoint) *contracts.ExecutionResult {
	if checkpoint == nil {
		return &contracts.ExecutionResult{
			Status:         "invalid",
			Verified:       false,
			ResultSummary:  "No checkpoint is available for await_user_choice.",
			BlockingReason: "checkpoint_missing",
		}
	}
	if len(checkpoint.Choices) == 0 {
		return &contracts.ExecutionResult{
			Status:         "invalid",
			Verified:       false,
			ResultSummary:  "await_user_choice requires explicit choices.",
			BlockingReason: "missing_choice_options",
		}
	}
	return &contracts.ExecutionResult{
		Status:        "awaiting_user_choice",
		Verified:      true,
		ResultSummary: "Milo is waiting for an explicit user choice.",
		UpdatedCheckpoint: &contracts.UpdatedCheckpoint{
			Status:    "awaiting_user_choice",
			Blocker:   checkpoint.Blocker,
			UpdatedAt: time.Now().UTC().Format(time.RFC3339),
			ExpiresAt: checkpoint.ExpiresAt,
		},
	}
}

func (e *Engine) executeResumeCheckpoint(task *runtime.TaskSnapshot, checkpoint *runtime.ResumeCheckpoint) contracts.RelayTurnResponse {
	if checkpoint == nil {
		return blockedTypedActionResponse(contracts.RelayTurnResponse{
			Intent:     task.Intent,
			TargetRoom: task.RoomID,
			Summary:    "Resume unavailable",
			ReportText: "Milo could not resume because no verified checkpoint is available.",
		}, "checkpoint_missing", "Resume requires a verified checkpoint.")
	}

	actionType := strings.ToLower(strings.TrimSpace(checkpoint.NextStepType))
	if actionType == "" {
		return blockedTypedActionResponse(contracts.RelayTurnResponse{
			Intent:     task.Intent,
			TargetRoom: task.RoomID,
			Summary:    "Resume unavailable",
			ReportText: "Milo could not resume because the checkpoint does not define a next step.",
		}, "missing_next_step_type", "Resume requires a checkpoint-owned next step.")
	}

	resp := contracts.RelayTurnResponse{
		Intent:             task.Intent,
		TargetRoom:         task.RoomID,
		Summary:            "Checkpoint step executed",
		ReportText:         "Milo resumed exactly one checkpoint-defined step.",
		ActionType:         actionType,
		ActionPayload:      checkpoint.NextStepPayload,
		ContinuationStatus: checkpoint.ContinuationStatus,
		NextBlocker:        checkpoint.Blocker,
		Choices:            append([]string(nil), checkpoint.Choices...),
	}

	switch actionType {
	case "check_state":
		expected := expectedCheckFromPayload(checkpoint.NextStepPayload)
		if expected == nil {
			return blockedTypedActionResponse(resp, "invalid_checkpoint_check_payload", "Resume checkpoint did not include a valid check_state payload.")
		}
		resp.ExpectedCheck = expected
		resp.ExecutionResult = e.executeCheckState(*task, checkpoint, expected)
		if resp.ExecutionResult.Verified {
			if strings.TrimSpace(resp.ContinuationStatus) == "" {
				resp.ContinuationStatus = "resumable"
			}
			resp.CompletionStatus = "attempted_unverified"
			resp.Summary = "Checkpoint verified"
			resp.ReportText = "Milo verified the saved checkpoint and stopped after one bounded continuation step."
		} else {
			resp.CompletionStatus = "blocked"
			resp.ContinuationStatus = "not_resumable"
			if resp.NextBlocker == "" {
				resp.NextBlocker = resp.ExecutionResult.BlockingReason
			}
			resp.Summary = "Checkpoint verification failed"
			resp.ReportText = "Milo could not resume because the checkpoint no longer passed runtime validation."
		}
	case "emit_message":
		resp.ExecutionResult = e.executeEmitMessage(*task, checkpoint.NextStepPayload)
		if resp.ExecutionResult.Verified {
			if strings.TrimSpace(resp.ContinuationStatus) == "" {
				resp.ContinuationStatus = "blocked"
			}
			resp.CompletionStatus = "attempted_unverified"
			resp.Summary = "Checkpoint message surfaced"
			resp.ReportText = "Milo surfaced the bounded checkpoint message and stopped after that one step."
		} else {
			resp.CompletionStatus = "blocked"
			resp.ContinuationStatus = "not_resumable"
			if resp.NextBlocker == "" {
				resp.NextBlocker = resp.ExecutionResult.BlockingReason
			}
			resp.Summary = "Checkpoint message invalid"
			resp.ReportText = "Milo could not resume because the checkpoint message step was invalid."
		}
	case "await_user_choice":
		resp.ExecutionResult = e.executeAwaitUserChoice(checkpoint)
		if resp.ExecutionResult.Verified {
			resp.RequiresUserChoice = true
			resp.CompletionStatus = "needs_user_choice"
			resp.ContinuationStatus = "awaiting_user_choice"
			resp.Summary = "User choice required"
			resp.ReportText = "Milo is waiting for your explicit choice before continuing."
		} else {
			resp.CompletionStatus = "blocked"
			resp.ContinuationStatus = "not_resumable"
			if resp.NextBlocker == "" {
				resp.NextBlocker = resp.ExecutionResult.BlockingReason
			}
			resp.Summary = "User choice step invalid"
			resp.ReportText = "Milo could not restore the user-choice step because the checkpoint was incomplete."
		}
	default:
		return blockedTypedActionResponse(resp, "unsupported_checkpoint_next_step:"+actionType, "Resume rejected the checkpoint because its next step type is not supported.")
	}

	return resp
}

func expectedCheckFromPayload(payload map[string]any) *contracts.ExpectedCheck {
	if payload == nil {
		return nil
	}
	checkType, _ := payload["check_type"].(string)
	key, _ := payload["key"].(string)
	expectedValue, _ := payload["expected_value"].(string)
	checkType = strings.TrimSpace(checkType)
	if checkType == "" {
		return nil
	}
	return &contracts.ExpectedCheck{
		CheckType:     checkType,
		Key:           key,
		ExpectedValue: expectedValue,
	}
}

func executeCheckpointStateCheck(checkpoint *runtime.ResumeCheckpoint, key, want, currentContextHash string) *contracts.ExecutionResult {
	if checkpoint == nil {
		return &contracts.ExecutionResult{
			Status:         "failed",
			Verified:       false,
			ResultSummary:  "No checkpoint is available for verification.",
			BlockingReason: "checkpoint_missing",
		}
	}

	switch key {
	case "status":
		if checkpoint.Status != want {
			return &contracts.ExecutionResult{
				Status:         "failed",
				Verified:       false,
				ResultSummary:  "Checkpoint status did not match the expected value.",
				BlockingReason: "checkpoint_status_mismatch",
			}
		}
	case "context_hash":
		if want == "match_current" && !contextHashMatches(checkpoint.ContextHash, currentContextHash) {
			return &contracts.ExecutionResult{
				Status:         "failed",
				Verified:       false,
				ResultSummary:  "Checkpoint context hash no longer matches current runtime state.",
				BlockingReason: "checkpoint_context_changed",
			}
		}
	case "fresh":
		if want == "true" && checkpointExpired(checkpoint) {
			return &contracts.ExecutionResult{
				Status:         "failed",
				Verified:       false,
				ResultSummary:  "Checkpoint is no longer fresh enough to resume safely.",
				BlockingReason: "checkpoint_expired",
			}
		}
	default:
		return &contracts.ExecutionResult{
			Status:         "invalid",
			Verified:       false,
			ResultSummary:  "Unsupported checkpoint_state key.",
			BlockingReason: "unsupported_checkpoint_key:" + key,
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	return &contracts.ExecutionResult{
		Status:        "passed",
		Verified:      true,
		ResultSummary: "Checkpoint state matched the requested verification.",
		UpdatedCheckpoint: &contracts.UpdatedCheckpoint{
			Status:    "verified_resumable",
			UpdatedAt: now,
			ExpiresAt: checkpoint.ExpiresAt,
		},
	}
}

func executeApprovalStateCheck(approval *runtime.ApprovalState, key, want string) *contracts.ExecutionResult {
	if approval == nil {
		return &contracts.ExecutionResult{
			Status:         "failed",
			Verified:       false,
			ResultSummary:  "No approval state is available.",
			BlockingReason: "approval_state_missing",
		}
	}

	switch key {
	case "status":
		if approval.Status != want {
			return &contracts.ExecutionResult{
				Status:         "failed",
				Verified:       false,
				ResultSummary:  "Approval status did not match the expected value.",
				BlockingReason: "approval_status_mismatch",
			}
		}
	case "selected_choice":
		if want == "present" && strings.TrimSpace(approval.SelectedChoice) == "" {
			return &contracts.ExecutionResult{
				Status:         "failed",
				Verified:       false,
				ResultSummary:  "No selected choice is recorded for approval state.",
				BlockingReason: "approval_choice_missing",
			}
		}
	default:
		return &contracts.ExecutionResult{
			Status:         "invalid",
			Verified:       false,
			ResultSummary:  "Unsupported approval_state key.",
			BlockingReason: "unsupported_approval_key:" + key,
		}
	}

	return &contracts.ExecutionResult{
		Status:        "passed",
		Verified:      true,
		ResultSummary: "Approval state matched the requested verification.",
	}
}

func executeTaskStateCheck(store *db.Store, task runtime.TaskSnapshot, key, want string) *contracts.ExecutionResult {
	switch key {
	case "slot":
		slotTask, _ := store.GetTask(want)
		if slotTask == nil || slotTask.TaskID != task.TaskID {
			return &contracts.ExecutionResult{
				Status:         "failed",
				Verified:       false,
				ResultSummary:  "Task is not present in the expected slot.",
				BlockingReason: "task_slot_mismatch",
			}
		}
	case "status":
		if task.Status != want {
			return &contracts.ExecutionResult{
				Status:         "failed",
				Verified:       false,
				ResultSummary:  "Task status did not match the expected value.",
				BlockingReason: "task_status_mismatch",
			}
		}
	default:
		return &contracts.ExecutionResult{
			Status:         "invalid",
			Verified:       false,
			ResultSummary:  "Unsupported task_state key.",
			BlockingReason: "unsupported_task_key:" + key,
		}
	}

	return &contracts.ExecutionResult{
		Status:        "passed",
		Verified:      true,
		ResultSummary: "Task state matched the requested verification.",
	}
}

func executeRuntimeFlagCheck(store *db.Store, key, want string) *contracts.ExecutionResult {
	switch {
	case strings.HasPrefix(key, "flag:"):
		name := strings.TrimPrefix(key, "flag:")
		value, _ := store.GetFlag(name)
		if value != want {
			return &contracts.ExecutionResult{
				Status:         "failed",
				Verified:       false,
				ResultSummary:  "Runtime flag did not match the expected value.",
				BlockingReason: "runtime_flag_mismatch",
			}
		}
	case strings.HasPrefix(key, "runtime_config:"):
		name := strings.TrimPrefix(key, "runtime_config:")
		value, _ := store.GetRuntimeConfig(name)
		if value != want {
			return &contracts.ExecutionResult{
				Status:         "failed",
				Verified:       false,
				ResultSummary:  "Runtime config value did not match the expected value.",
				BlockingReason: "runtime_config_mismatch",
			}
		}
	default:
		return &contracts.ExecutionResult{
			Status:         "invalid",
			Verified:       false,
			ResultSummary:  "Unsupported runtime_flag key.",
			BlockingReason: "unsupported_runtime_flag_key:" + key,
		}
	}

	return &contracts.ExecutionResult{
		Status:        "passed",
		Verified:      true,
		ResultSummary: "Runtime-owned flag matched the requested verification.",
	}
}

func updatedCheckpointFromResult(checkpoint *runtime.ResumeCheckpoint, result *contracts.ExecutionResult) *runtime.ResumeCheckpoint {
	if checkpoint == nil {
		return nil
	}
	updated := *checkpoint
	updated.EvidenceBoundary = checkpoint.EvidenceBoundary
	now := time.Now().UTC().Format(time.RFC3339)
	updated.UpdatedAt = now
	if result == nil || result.UpdatedCheckpoint == nil {
		return &updated
	}
	if result.UpdatedCheckpoint.Status != "" {
		updated.Status = result.UpdatedCheckpoint.Status
	}
	if result.UpdatedCheckpoint.Blocker != "" {
		updated.Blocker = result.UpdatedCheckpoint.Blocker
	}
	if result.UpdatedCheckpoint.UpdatedAt != "" {
		updated.UpdatedAt = result.UpdatedCheckpoint.UpdatedAt
	}
	if result.UpdatedCheckpoint.ExpiresAt != "" {
		updated.ExpiresAt = result.UpdatedCheckpoint.ExpiresAt
	}
	return &updated
}

func (e *Engine) finalizeTask(task *runtime.TaskSnapshot, checkpoint *runtime.ResumeCheckpoint, relayResp contracts.RelayTurnResponse) {
	_ = e.store.AppendConversation("assistant", relayResp.ReportText)
	e.emit("milo.thought", map[string]any{"text": relayResp.ThoughtText, "style": "standard", "trigger": "auto"})

	outcome := normalizeRelayOutcome(relayResp)
	task.Status = outcome.TaskStatus
	task.StuckReason = outcome.Blocker
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_ = e.store.ClearTask("active")
	_ = e.store.ClearTask(task.Slot)

	switch outcome.TaskStatus {
	case "awaiting_user_choice":
		task.Slot = "awaiting_user_choice"
		_ = e.store.UpsertTask("awaiting_user_choice", *task)
		_ = e.store.SetApprovalState(buildApprovalState(task.TaskID, outcome.Blocker, relayResp.Choices))
		checkpointToStore := checkpoint
		if checkpointToStore == nil {
			checkpointToStore = buildApprovalCheckpoint(*task, outcome.Blocker, relayResp.Choices, e.currentContextHash())
		} else {
			checkpointToStore = updatedCheckpointFromResult(checkpointToStore, relayResp.ExecutionResult)
			checkpointToStore.SourceStatus = "awaiting_user_choice"
			checkpointToStore.Status = "awaiting_user_choice"
			checkpointToStore.Blocker = outcome.Blocker
			checkpointToStore.ContinuationStatus = "awaiting_user_choice"
			checkpointToStore.NextStepType = "await_user_choice"
			checkpointToStore.NextStepPayload = map[string]any{
				"blocker": outcome.Blocker,
				"choices": append([]string(nil), relayResp.Choices...),
			}
			checkpointToStore.Choices = append([]string(nil), relayResp.Choices...)
			checkpointToStore.ContextHash = e.currentContextHash()
			checkpointToStore.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		}
		_ = e.store.SetResumeCheckpoint(checkpointToStore)
	case "resumable":
		checkpointToStore := updatedCheckpointFromResult(checkpoint, relayResp.ExecutionResult)
		if checkpointToStore != nil {
			task.Slot = "interrupted"
			task.Status = "resumable"
			_ = e.store.UpsertTask("interrupted", *task)
			_ = e.store.SetResumeCheckpoint(checkpointToStore)
		}
		_ = e.store.ClearApprovalState()
		_ = e.store.ClearTask("awaiting_user_choice")
	case "blocked", "completed":
		_ = e.store.ClearApprovalState()
		_ = e.store.ClearResumeCheckpoint()
		_ = e.store.ClearTask("awaiting_user_choice")
		_ = e.store.ClearTask("interrupted")
	default:
		_ = e.store.ClearApprovalState()
		_ = e.store.ClearResumeCheckpoint()
		_ = e.store.ClearTask("awaiting_user_choice")
		_ = e.store.ClearTask("interrupted")
	}

	_ = e.store.AddTaskHistory(task.TaskID, task.Prompt, outcome.HistoryStatus, relayResp.Summary)
	e.transitionTo("idle")

	switch outcome.TaskStatus {
	case "completed":
		e.emit("task.completed", map[string]any{"task_id": task.TaskID, "summary": relayResp.Summary, "trophy_eligible": false})
	default:
		e.emit(outcome.EventType, map[string]any{
			"task_id":         task.TaskID,
			"summary":         relayResp.Summary,
			"report_text":     relayResp.ReportText,
			"blocker":         outcome.Blocker,
			"choices":         relayResp.Choices,
			"requires_choice": relayResp.RequiresUserChoice,
		})
	}
	e.emit("archive.record_created", map[string]any{
		"task_id":      task.TaskID,
		"title":        relayResp.Summary,
		"description":  relayResp.ReportText,
		"created_at":   time.Now().UTC().Format(time.RFC3339),
		"task_status":  outcome.TaskStatus,
		"next_blocker": outcome.Blocker,
	})
	e.emit("report.ready", map[string]any{
		"task_id":             task.TaskID,
		"report_text":         relayResp.ReportText,
		"style":               e.responseStyle,
		"completion_status":   outcome.TaskStatus,
		"next_blocker":        outcome.Blocker,
		"action_type":         relayResp.ActionType,
		"continuation_status": relayResp.ContinuationStatus,
		"execution_result":    relayResp.ExecutionResult,
	})
}

func buildApprovalState(taskID, blocker string, choices []string) *runtime.ApprovalState {
	now := time.Now().UTC().Format(time.RFC3339)
	return &runtime.ApprovalState{
		TaskID:        taskID,
		Status:        "awaiting_user_choice",
		Blocker:       blocker,
		Choices:       append([]string(nil), choices...),
		RequestedAt:   now,
		ResumeAllowed: false,
	}
}

func buildApprovalCheckpoint(task runtime.TaskSnapshot, blocker string, choices []string, contextHash string) *runtime.ResumeCheckpoint {
	now := time.Now().UTC()
	return &runtime.ResumeCheckpoint{
		TaskID:             task.TaskID,
		SourceStatus:       "awaiting_user_choice",
		Phase:              "resume_after_user_choice",
		Blocker:            blocker,
		ContinuationStatus: "awaiting_user_choice",
		NextStepType:       "await_user_choice",
		NextStepPayload: map[string]any{
			"blocker": blocker,
			"choices": append([]string(nil), choices...),
		},
		Choices:          append([]string(nil), choices...),
		ContextHash:      contextHash,
		Status:           "awaiting_user_choice",
		CreatedAt:        now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		ExpiresAt:        now.Add(2 * time.Hour).Format(time.RFC3339),
		IntakeAssessment: task.IntakeAssessment,
		EvidenceBoundary: task.EvidenceBoundary,
	}
}

func buildInterruptedCheckpoint(task runtime.TaskSnapshot, contextHash string) *runtime.ResumeCheckpoint {
	now := time.Now().UTC()
	return &runtime.ResumeCheckpoint{
		TaskID:             task.TaskID,
		SourceStatus:       "interrupted",
		Phase:              "resume_after_interruption",
		Blocker:            task.StuckReason,
		ContinuationStatus: "resumable",
		NextStepType:       "check_state",
		NextStepPayload: map[string]any{
			"check_type":     "checkpoint_state",
			"key":            "status",
			"expected_value": "interrupted",
		},
		ContextHash:      contextHash,
		Status:           "interrupted",
		CreatedAt:        now.Format(time.RFC3339),
		UpdatedAt:        now.Format(time.RFC3339),
		ExpiresAt:        now.Add(2 * time.Hour).Format(time.RFC3339),
		IntakeAssessment: task.IntakeAssessment,
		EvidenceBoundary: task.EvidenceBoundary,
	}
}

func checkpointExpired(checkpoint *runtime.ResumeCheckpoint) bool {
	expiresAt, err := time.Parse(time.RFC3339, checkpoint.ExpiresAt)
	if err != nil {
		return true
	}
	return time.Now().UTC().After(expiresAt)
}

func (e *Engine) currentContextHash() string {
	activeCtx, _ := e.store.GetRuntimeConfig("active_context")
	sum := sha256.Sum256([]byte(activeCtx))
	return hex.EncodeToString(sum[:])
}

func contextHashMatches(expected, actual string) bool {
	if expected == "" {
		return actual == ""
	}
	return expected == actual
}

func containsChoice(choices []string, selected string) bool {
	for _, choice := range choices {
		if strings.EqualFold(strings.TrimSpace(choice), selected) {
			return true
		}
	}
	return false
}

func (e *Engine) assessPromptIntake(prompt string) *runtime.IntakeAssessment {
	trimmed := strings.TrimSpace(prompt)
	assessment := &runtime.IntakeAssessment{
		PrimaryClass:       "TASK_REQUEST",
		SecondaryFlags:     nil,
		TrustTier:          5,
		ValidationState:    "VALID",
		ChosenClosedAction: "START_TASK",
	}

	if trimmed == "" {
		assessment.PrimaryClass = "UNKNOWN"
		assessment.ValidationState = "UNKNOWN_STATE"
		assessment.ChosenClosedAction = "CLARIFY"
		assessment.SecondaryFlags = append(assessment.SecondaryFlags, "ambiguous_intent")
		return assessment
	}

	lower := strings.ToLower(trimmed)
	injectionHits := 0
	for _, phrase := range injectionPhrases {
		if strings.Contains(lower, phrase) {
			injectionHits++
		}
	}
	if injectionHits > 0 {
		assessment.SecondaryFlags = appendUnique(assessment.SecondaryFlags, "injection_suspected")
		assessment.SecondaryFlags = appendUnique(assessment.SecondaryFlags, "authority_spoof_suspected")
	}

	if isDominantInjection(lower, injectionHits) {
		assessment.PrimaryClass = "PROMPT_INJECTION"
		assessment.TrustTier = 7
		assessment.ValidationState = "INVALID"
		assessment.ChosenClosedAction = "REFUSE"
		return assessment
	}

	switch {
	case looksLikeMemoryWrite(lower):
		assessment.MemoryIntent, _ = e.buildMemoryIntent(trimmed, lower, true, injectionHits)
		assessment.PrimaryClass = "MEMORY_WRITE_CANDIDATE"
		assessment.ChosenClosedAction = "REQUEST_CONFIRMATION"
		e.applyMemoryWriteRules(assessment)
	case looksLikeMemoryRead(lower):
		assessment.MemoryIntent, _ = e.buildMemoryIntent(trimmed, lower, false, injectionHits)
		assessment.PrimaryClass = "MEMORY_READ_REQUEST"
		assessment.ChosenClosedAction = "READ_MEMORY"
		e.applyMemoryReadRules(assessment)
	case looksLikePermissionRequest(lower):
		assessment.PrimaryClass = "PERMISSION_REQUEST"
		assessment.ChosenClosedAction = "REQUEST_PERMISSION"
	case looksLikeQuestion(trimmed):
		assessment.PrimaryClass = "QUESTION"
		assessment.ChosenClosedAction = "ANSWER"
	case looksLikeClarification(lower):
		assessment.PrimaryClass = "CLARIFICATION"
		assessment.ValidationState = "UNKNOWN_STATE"
		assessment.ChosenClosedAction = "CLARIFY"
	case looksLikeExternalContent(lower):
		assessment.PrimaryClass = "EXTERNAL_CONTENT"
		assessment.TrustTier = 6
		assessment.SecondaryFlags = appendUnique(assessment.SecondaryFlags, "external_untrusted")
	default:
		assessment.PrimaryClass = "TASK_REQUEST"
	}

	if assessment.PrimaryClass == "UNKNOWN" {
		assessment.ValidationState = "UNKNOWN_STATE"
		assessment.ChosenClosedAction = "CLARIFY"
	}
	return assessment
}

func assessChoiceIntake(taskID, choice, decision string) *runtime.IntakeAssessment {
	assessment := &runtime.IntakeAssessment{
		PrimaryClass:       "CLARIFICATION",
		SecondaryFlags:     nil,
		TrustTier:          5,
		ValidationState:    "VALID",
		ChosenClosedAction: "CONTINUE_TASK",
	}
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(decision) == "" {
		assessment.ValidationState = "UNKNOWN_STATE"
		assessment.ChosenClosedAction = "CLARIFY"
		assessment.SecondaryFlags = append(assessment.SecondaryFlags, "ambiguous_intent")
		return assessment
	}
	if strings.EqualFold(strings.TrimSpace(decision), "approve") && strings.TrimSpace(choice) == "" {
		assessment.ValidationState = "UNKNOWN_STATE"
		assessment.ChosenClosedAction = "CLARIFY"
		assessment.SecondaryFlags = append(assessment.SecondaryFlags, "ambiguous_intent")
	}
	return assessment
}

func assessResumeIntake() *runtime.IntakeAssessment {
	return &runtime.IntakeAssessment{
		PrimaryClass:       "APP_EVENT",
		SecondaryFlags:     nil,
		TrustTier:          1,
		ValidationState:    "VALID",
		ChosenClosedAction: "CONTINUE_TASK",
	}
}

func looksLikeQuestion(prompt string) bool {
	trimmed := strings.TrimSpace(prompt)
	lower := strings.ToLower(trimmed)
	return strings.HasSuffix(trimmed, "?") ||
		strings.HasPrefix(lower, "what ") ||
		strings.HasPrefix(lower, "why ") ||
		strings.HasPrefix(lower, "how ") ||
		strings.HasPrefix(lower, "when ") ||
		strings.HasPrefix(lower, "where ") ||
		strings.HasPrefix(lower, "who ")
}

func looksLikeClarification(prompt string) bool {
	return strings.Contains(prompt, "which one") || strings.Contains(prompt, "do you mean") || strings.Contains(prompt, "clarify")
}

func looksLikePermissionRequest(prompt string) bool {
	return strings.Contains(prompt, "permission") || strings.Contains(prompt, "approve ") || strings.Contains(prompt, "allowed to")
}

func looksLikeMemoryRead(prompt string) bool {
	return strings.Contains(prompt, "remember what") || strings.Contains(prompt, "what do you remember") || strings.Contains(prompt, "recall my preference")
}

func looksLikeMemoryWrite(prompt string) bool {
	return strings.Contains(prompt, "remember that") || strings.Contains(prompt, "save this preference") || strings.Contains(prompt, "always remember")
}

func looksLikeExternalContent(prompt string) bool {
	return strings.Contains(prompt, "http://") || strings.Contains(prompt, "https://") || strings.Contains(prompt, "www.") || strings.Contains(prompt, "pdf")
}

func isDominantInjection(prompt string, injectionHits int) bool {
	if injectionHits == 0 {
		return false
	}
	return strings.HasPrefix(prompt, "ignore previous instructions") || strings.HasPrefix(prompt, "you are now")
}

func appendUnique(values []string, candidate string) []string {
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	return append(values, candidate)
}

func containsStringExact(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func (e *Engine) buildMemoryIntent(prompt, lower string, isWrite bool, injectionHits int) (*runtime.MemoryIntent, string) {
	effect := "preference_read"
	if isWrite {
		effect = "preference_write"
	}
	status := "allowed"
	if injectionHits > 0 {
		status = "blocked"
	} else if isWrite {
		if e.memoryConflict(e.extractMemoryPayload(prompt)) {
			status = "contradicted"
		}
	}
	payload := e.extractMemoryPayload(prompt)
	intent := &runtime.MemoryIntent{
		Class:        string(runtime.MemoryClassUserPreference),
		Key:          "preference_memory",
		Value:        payload,
		Source:       "user_prompt",
		Effect:       effect,
		SafetyStatus: status,
	}
	return intent, status
}

func (e *Engine) applyMemoryWriteRules(assessment *runtime.IntakeAssessment) {
	if assessment.MemoryIntent == nil {
		return
	}
	switch assessment.MemoryIntent.SafetyStatus {
	case "blocked":
		assessment.ValidationState = "INVALID"
		assessment.ChosenClosedAction = "REFUSE"
		assessment.SecondaryFlags = appendUnique(assessment.SecondaryFlags, "memory_blocked")
	case "contradicted":
		// Benign corrections are allowed, but must be explicit and governed.
		assessment.ValidationState = "PENDING_APPROVAL"
		assessment.ChosenClosedAction = "REQUEST_CONFIRMATION"
		assessment.SecondaryFlags = appendUnique(assessment.SecondaryFlags, "memory_contradiction")
	default:
		assessment.ValidationState = "PENDING_APPROVAL"
		assessment.ChosenClosedAction = "REQUEST_CONFIRMATION"
	}
}

func (e *Engine) applyMemoryReadRules(assessment *runtime.IntakeAssessment) {
	if assessment.MemoryIntent == nil {
		return
	}
	if assessment.MemoryIntent.SafetyStatus == "blocked" {
		assessment.ValidationState = "INVALID"
		assessment.ChosenClosedAction = "REFUSE"
		assessment.SecondaryFlags = appendUnique(assessment.SecondaryFlags, "memory_blocked")
		return
	}
	assessment.ValidationState = "VALID"
}

func (e *Engine) extractMemoryPayload(prompt string) string {
	lower := strings.ToLower(prompt)
	for _, marker := range []string{"remember that", "remember"} {
		if idx := strings.Index(lower, marker); idx != -1 {
			return strings.TrimSpace(prompt[idx+len(marker):])
		}
	}
	return strings.TrimSpace(prompt)
}

func (e *Engine) executeMemoryWrite(task *runtime.TaskSnapshot, assessment *runtime.IntakeAssessment) error {
	if task == nil || assessment == nil || assessment.MemoryIntent == nil {
		return errors.New("memory write unavailable")
	}
	if strings.EqualFold(strings.TrimSpace(assessment.MemoryIntent.SafetyStatus), "blocked") {
		return errors.New("memory write blocked")
	}
	payload := strings.TrimSpace(assessment.MemoryIntent.Value)
	if payload == "" {
		return errors.New("memory write requires a value")
	}
	// Hard stop on obvious instruction-bearing text even when the memory classifier fires.
	lower := strings.ToLower(payload)
	for _, phrase := range injectionPhrases {
		if strings.Contains(lower, phrase) {
			_ = e.store.QuarantineActiveMemoryEntry(runtime.MemoryClassUserPreference, "preference_memory", "injection_phrase_in_payload")
			return errors.New("memory write blocked")
		}
	}

	entry := runtime.MemoryEntry{
		Class:     runtime.MemoryClassUserPreference,
		Key:       "preference_memory",
		Value:     payload,
		Status:    runtime.MemoryEntryStatusActive,
		Source:    "user_prompt",
		Effect:    "user_teaching",
		TrustTier: assessment.TrustTier,
	}
	if err := e.store.SetActiveMemoryEntry(entry); err != nil {
		return err
	}

	task.Status = "completed"
	task.StuckReason = ""
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	boundary := e.ensureEvidenceBoundary(task, nil)
	if boundary != nil {
		boundary.VerifiedFacts = appendUnique(boundary.VerifiedFacts, "Preference memory was saved by runtime.")
		boundary.ExecutedSteps = appendUnique(boundary.ExecutedSteps, "memory_write verified: preference_memory updated")
	}
	e.emit("task.message_emitted", map[string]any{
		"task_id": task.TaskID,
		"message": "Got it. I will remember that.",
		"phase":   "memory_write",
	})
	return nil
}

func (e *Engine) executeMemoryRead(task *runtime.TaskSnapshot, assessment *runtime.IntakeAssessment) error {
	if task == nil || assessment == nil {
		return errors.New("memory read unavailable")
	}
	value, err := e.store.GetActiveMemoryValue(runtime.MemoryClassUserPreference, "preference_memory")
	if err != nil {
		return err
	}
	message := "I do not have any saved preferences yet."
	if strings.TrimSpace(value) != "" {
		message = "Here is what I remember: " + value
	}

	task.Status = "completed"
	task.StuckReason = ""
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	boundary := e.ensureEvidenceBoundary(task, nil)
	if boundary != nil {
		boundary.VerifiedFacts = appendUnique(boundary.VerifiedFacts, "Memory was read by runtime.")
		boundary.ExecutedSteps = appendUnique(boundary.ExecutedSteps, "memory_read verified: preference_memory fetched")
	}

	e.emit("task.message_emitted", map[string]any{
		"task_id": task.TaskID,
		"message": message,
		"phase":   "memory_read",
	})
	return nil
}

func (e *Engine) composeMemoryContext() string {
	value, _ := e.store.GetActiveMemoryValue(runtime.MemoryClassUserPreference, "preference_memory")
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	ctx := "user_preferences:\n- " + value
	if len(ctx) > 600 {
		ctx = ctx[:600] + "..."
	}
	return ctx
}

func (e *Engine) memoryConflict(value string) bool {
	if value == "" {
		return false
	}
	existing, _ := e.store.GetActiveMemoryValue(runtime.MemoryClassUserPreference, "preference_memory")
	if existing == "" {
		return false
	}
	return !strings.EqualFold(existing, value)
}

func (e *Engine) invalidatePendingContinuationLocked(reason string) error {
	awaiting, err := e.store.GetTask("awaiting_user_choice")
	if err != nil {
		return err
	}
	if awaiting != nil {
		awaiting.Status = "superseded"
		awaiting.StuckReason = reason
		awaiting.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
		_ = e.store.UpsertTask("awaiting_user_choice", *awaiting)
	}
	_ = e.store.ClearApprovalState()
	_ = e.store.ClearResumeCheckpoint()
	_ = e.store.ClearTask("awaiting_user_choice")
	_ = e.store.ClearTask("interrupted")
	return nil
}

func (e *Engine) validatedResumeTargetLocked() (*runtime.ResumeCheckpoint, *runtime.TaskSnapshot, error) {
	checkpoint, err := e.store.GetResumeCheckpoint()
	if err != nil {
		return nil, nil, err
	}
	if checkpoint == nil {
		return nil, nil, errors.New("no resume checkpoint available")
	}
	if checkpointExpired(checkpoint) {
		return nil, nil, e.rejectResumeLocked(checkpoint.TaskID, "checkpoint_expired")
	}
	if !contextHashMatches(checkpoint.ContextHash, e.currentContextHash()) {
		return nil, nil, e.rejectResumeLocked(checkpoint.TaskID, "checkpoint_context_changed")
	}

	switch checkpoint.SourceStatus {
	case "awaiting_user_choice":
		task, err := e.store.GetTask("awaiting_user_choice")
		if err != nil {
			return nil, nil, err
		}
		approval, err := e.store.GetApprovalState()
		if err != nil {
			return nil, nil, err
		}
		if task == nil || approval == nil || approval.TaskID != checkpoint.TaskID || task.TaskID != checkpoint.TaskID {
			return nil, nil, e.rejectResumeLocked(checkpoint.TaskID, "resume_state_missing")
		}
		if approval.Status != "choice_recorded" || approval.Decision != "approve" || approval.SelectedChoice == "" {
			return nil, nil, errors.New("choice not recorded for resume")
		}
		if checkpoint.Status != "approved_pending_resume" {
			return nil, nil, e.rejectResumeLocked(checkpoint.TaskID, "checkpoint_not_ready")
		}
		return checkpoint, task, nil
	case "interrupted":
		task, err := e.store.GetTask("interrupted")
		if err != nil {
			return nil, nil, err
		}
		if task == nil || task.TaskID != checkpoint.TaskID {
			return nil, nil, e.rejectResumeLocked(checkpoint.TaskID, "interrupted_task_missing")
		}
		if checkpoint.Status != "interrupted" {
			return nil, nil, e.rejectResumeLocked(checkpoint.TaskID, "checkpoint_not_ready")
		}
		return checkpoint, task, nil
	default:
		return nil, nil, e.rejectResumeLocked(checkpoint.TaskID, "unsupported_resume_source")
	}
}

func (e *Engine) rejectResumeLocked(taskID, blocker string) error {
	e.emit("task.resume_blocked", map[string]any{
		"task_id": taskID,
		"blocker": blocker,
	})
	return errors.New(blocker)
}
