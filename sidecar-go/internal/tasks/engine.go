package tasks

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"xmilo/sidecar-go/internal/db"
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
	return runtime.RuntimeState{
		MiloState:                  e.currentState,
		CurrentRoomID:              e.currentRoomID,
		CurrentAnchorID:            e.currentAnchor,
		LastMeaningfulUserActionAt: lastAction,
		ActiveTask:                 active,
		QueuedTask:                 queued,
		RuntimeID:                  "local-sidecar",
	}
}

func (e *Engine) StartTask(ctx context.Context, prompt string) (*runtime.TaskSnapshot, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	existing, err := e.store.GetTask("active")
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, errors.New("active task already running")
	}

	route := rooms.Resolve(prompt, "")
	now := time.Now().UTC().Format(time.RFC3339)
	task := runtime.TaskSnapshot{
		TaskID:     "task_" + uuid.NewString(),
		Prompt:     prompt,
		Intent:     route.Intent,
		RoomID:     route.RoomID,
		AnchorID:   route.AnchorID,
		Status:     "running",
		StartedAt:  now,
		UpdatedAt:  now,
		MaxRetries: 3,
		Slot:       "active",
	}

	if err := e.store.UpsertTask("active", task); err != nil {
		return nil, err
	}
	_ = e.store.SetRuntimeConfig("last_meaningful_user_action_at", now)

	e.emit("task.accepted", map[string]any{"task_id": task.TaskID, "intent": task.Intent, "room_id": task.RoomID})
	e.transitionTo("moving")
	e.emit("milo.movement_started", map[string]any{
		"from_room": e.currentRoomID, "from_anchor": e.currentAnchor,
		"to_room": route.RoomID, "to_anchor": route.AnchorID, "reason": "task_start", "estimated_ms": 1200,
	})

	go e.runTask(context.Background(), task)

	return &task, nil
}

func (e *Engine) runTask(ctx context.Context, task runtime.TaskSnapshot) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	e.currentRoomID = task.RoomID
	e.currentAnchor = task.AnchorID
	e.emit("milo.room_changed", map[string]any{"room_id": task.RoomID, "anchor_id": task.AnchorID})
	e.transitionTo("working")
	e.emit("task.progress", map[string]any{"task_id": task.TaskID, "phase": "intake", "room_id": task.RoomID, "anchor_id": task.AnchorID, "message": "Milo is reasoning."})

	_ = e.store.AppendConversation("user", task.Prompt)
	tail, _ := e.store.GetConversationTail()
	var turnTail []contracts.TurnMessage
	for _, item := range tail {
		turnTail = append(turnTail, contracts.TurnMessage{Role: item["role"], Content: item["content"]})
	}

	// If the user has an active pasted context (Phase 3), prepend it to the relay
	// prompt only — conversation history stores the clean prompt so the context
	// does not bloat every subsequent turn in the tail.
	promptForRelay := task.Prompt
	if activeCtx, _ := e.store.GetRuntimeConfig("active_context"); activeCtx != "" {
		promptForRelay = "<pasted_context>\n" + activeCtx + "\n</pasted_context>\n\n" + task.Prompt
	}

	relayResp, err := e.relay.Turn(timeoutCtx, contracts.RelayTurnRequest{
		TaskID:           task.TaskID,
		Phase:            "intake",
		Prompt:           promptForRelay,
		SystemPrompt:     e.systemPrompt,
		ConversationTail: turnTail,
		ResponseStyle:    e.responseStyle,
	})
	if err != nil {
		// Detect entitlement_lost (relay returns 403 with {"error":"entitlement_lost"}).
		// Save the task as interrupted so the user can resume after resubscribing.
		if strings.Contains(err.Error(), "entitlement_lost") {
			task.Status = "interrupted"
			task.StuckReason = "entitlement_lost"
			task.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			_ = e.store.UpsertTask("interrupted", task)
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

	task.Status = "completed"
	task.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	_ = e.store.ClearTask("active")
	_ = e.store.AddTaskHistory(task.TaskID, task.Prompt, "completed", relayResp.Summary)
	e.transitionTo("idle")

	e.emit("task.completed", map[string]any{"task_id": task.TaskID, "summary": relayResp.Summary, "trophy_eligible": false})
	e.emit("archive.record_created", map[string]any{"task_id": task.TaskID, "title": relayResp.Summary, "description": relayResp.ReportText, "created_at": time.Now().UTC().Format(time.RFC3339)})
	e.emit("report.ready", map[string]any{"task_id": task.TaskID, "report_text": relayResp.ReportText, "style": e.responseStyle})
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

func (e *Engine) ThoughtRequest() map[string]any {
	if e.currentState == "sleeping" {
		return map[string]any{"accepted": false, "reason": "sleeping"}
	}
	payload := map[string]any{"accepted": true, "text": "Milo is thinking about the task board."}
	e.emit("milo.thought", map[string]any{"text": "Milo is thinking about the task board.", "style": "standard", "trigger": "tap"})
	return payload
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
	e.mu.Lock()
	defer e.mu.Unlock()

	active, err := e.store.GetTask("active")
	if err != nil {
		return nil, err
	}
	if active != nil {
		return nil, errors.New("active task already running")
	}

	task, err := e.store.GetTask("interrupted")
	if err != nil {
		return nil, err
	}
	if task == nil {
		return nil, errors.New("no interrupted task to resume")
	}

	// Reset task state for re-run
	now := time.Now().UTC().Format(time.RFC3339)
	task.Status = "running"
	task.StuckReason = ""
	task.UpdatedAt = now

	if err := e.store.UpsertTask("active", *task); err != nil {
		return nil, err
	}
	if err := e.store.ClearTask("interrupted"); err != nil {
		return nil, err
	}

	e.emit("task.accepted", map[string]any{"task_id": task.TaskID, "intent": task.Intent, "room_id": task.RoomID, "resumed": true})
	e.transitionTo("working")

	go e.runTask(context.Background(), *task)
	return task, nil
}

// DiscardInterrupted clears the saved interrupted task (user chose "start new").
func (e *Engine) DiscardInterrupted() error {
	return e.store.ClearTask("interrupted")
}
