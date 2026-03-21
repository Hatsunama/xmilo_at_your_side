package runtime

type HealthResponse struct {
	OK        bool   `json:"ok"`
	Service   string `json:"service"`
	Version   string `json:"version"`
	UptimeSec int64  `json:"uptime_sec"`
}

type ReadyResponse struct {
	OK              bool   `json:"ok"`
	WakeLockActive  bool   `json:"wake_lock_active"`
	DBReady         bool   `json:"db_ready"`
	RelayConfigured bool   `json:"relay_configured"`
	MindLoaded      bool   `json:"mind_loaded"`
	RuntimeID       string `json:"runtime_id"`
}

type TaskSnapshot struct {
	TaskID      string `json:"task_id"`
	Prompt      string `json:"prompt"`
	Intent      string `json:"intent"`
	RoomID      string `json:"room_id"`
	AnchorID    string `json:"anchor_id"`
	Status      string `json:"status"`
	StartedAt   string `json:"started_at"`
	UpdatedAt   string `json:"updated_at"`
	RetryCount  int    `json:"retry_count"`
	MaxRetries  int    `json:"max_retries"`
	FailureType string `json:"failure_type"`
	StuckReason string `json:"stuck_reason"`
	Slot        string `json:"slot"`
}

type RuntimeState struct {
	MiloState                  string        `json:"milo_state"`
	CurrentRoomID              string        `json:"current_room_id"`
	CurrentAnchorID            string        `json:"current_anchor_id"`
	LastMeaningfulUserActionAt string        `json:"last_meaningful_user_action_at"`
	ActiveTask                 *TaskSnapshot `json:"active_task,omitempty"`
	QueuedTask                 *TaskSnapshot `json:"queued_task,omitempty"`
	RuntimeID                  string        `json:"runtime_id"`
}
