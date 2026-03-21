// Package game — events.go
// Defines the JSON payload structs for every WebSocket event that PicoClaw emits.
// These match the map[string]any payloads in sidecar-go/internal/tasks/engine.go
// exactly. The renderer only reacts to these; it never initiates state changes.
package game

import "encoding/json"

// Envelope is the top-level wrapper PicoClaw sends for every broadcast.
// Matches ws.Envelope in sidecar-go/internal/ws/hub.go
type Envelope struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// MovementStarted is the payload for "milo.movement_started".
// Emitted by engine.go when a task begins (moving to work room)
// and when a task completes (returning to main_hall).
type MovementStarted struct {
	FromRoom    string `json:"from_room"`
	FromAnchor  string `json:"from_anchor"`
	ToRoom      string `json:"to_room"`
	ToAnchor    string `json:"to_anchor"`
	Reason      string `json:"reason"`      // "task_start" | "report"
	EstimatedMS int    `json:"estimated_ms"` // walk animation duration
}

// RoomChanged is the payload for "milo.room_changed".
// Emitted when Milo has arrived at the destination room.
type RoomChanged struct {
	RoomID   string `json:"room_id"`
	AnchorID string `json:"anchor_id"`
}

// StateChanged is the payload for "milo.state_changed".
// States: "idle" | "moving" | "working"
type StateChanged struct {
	FromState string `json:"from_state"`
	ToState   string `json:"to_state"`
}

// TaskCompleted is the payload for "task.completed".
type TaskCompleted struct {
	TaskID         string `json:"task_id"`
	Summary        string `json:"summary"`
	TrophyEligible bool   `json:"trophy_eligible"`
}

// TaskStuck is the payload for "task.stuck".
type TaskStuck struct {
	TaskID          string   `json:"task_id"`
	Reason          string   `json:"reason"`
	RecoveryOptions []string `json:"recovery_options"`
}

// TaskCancelled is the payload for "task.cancelled".
type TaskCancelled struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason"`
}

// MiloThought is the payload for "milo.thought".
// Used to trigger the speech bubble / thinking cloud above Milo.
type MiloThought struct {
	Text    string `json:"text"`
	Style   string `json:"style"`   // "standard"
	Trigger string `json:"trigger"` // "auto" | "tap"
}
