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
	TaskID           string            `json:"task_id"`
	Prompt           string            `json:"prompt"`
	Intent           string            `json:"intent"`
	RoomID           string            `json:"room_id"`
	AnchorID         string            `json:"anchor_id"`
	Status           string            `json:"status"`
	StartedAt        string            `json:"started_at"`
	UpdatedAt        string            `json:"updated_at"`
	RetryCount       int               `json:"retry_count"`
	MaxRetries       int               `json:"max_retries"`
	FailureType      string            `json:"failure_type"`
	StuckReason      string            `json:"stuck_reason"`
	Slot             string            `json:"slot"`
	IntakeAssessment *IntakeAssessment `json:"intake_assessment,omitempty"`
	EvidenceBoundary *EvidenceBoundary `json:"evidence_boundary,omitempty"`
}

type ApprovalState struct {
	TaskID         string   `json:"task_id"`
	Status         string   `json:"status"`
	Blocker        string   `json:"blocker"`
	Choices        []string `json:"choices"`
	SelectedChoice string   `json:"selected_choice,omitempty"`
	Decision       string   `json:"decision,omitempty"`
	RequestedAt    string   `json:"requested_at"`
	RecordedAt     string   `json:"recorded_at,omitempty"`
	ResumeAllowed  bool     `json:"resume_allowed"`
}

type ResumeCheckpoint struct {
	TaskID             string            `json:"task_id"`
	SourceStatus       string            `json:"source_status"`
	Phase              string            `json:"phase"`
	Blocker            string            `json:"blocker"`
	ContinuationStatus string            `json:"continuation_status"`
	NextStepType       string            `json:"next_step_type"`
	NextStepPayload    map[string]any    `json:"next_step_payload,omitempty"`
	Choices            []string          `json:"choices,omitempty"`
	SelectedChoice     string            `json:"selected_choice,omitempty"`
	ContextHash        string            `json:"context_hash"`
	Status             string            `json:"status"`
	CreatedAt          string            `json:"created_at"`
	UpdatedAt          string            `json:"updated_at"`
	ExpiresAt          string            `json:"expires_at"`
	IntakeAssessment   *IntakeAssessment `json:"intake_assessment,omitempty"`
	EvidenceBoundary   *EvidenceBoundary `json:"evidence_boundary,omitempty"`
}

type IntakeAssessment struct {
	PrimaryClass       string   `json:"primary_class"`
	SecondaryFlags     []string `json:"secondary_flags"`
	TrustTier          int      `json:"trust_tier"`
	ValidationState    string   `json:"validation_state"`
	ChosenClosedAction string   `json:"chosen_closed_action"`
	MemoryIntent       *MemoryIntent `json:"memory_intent,omitempty"`
}

type MemoryIntent struct {
	Class        string `json:"class"`
	Key          string `json:"key,omitempty"`
	Value        string `json:"value,omitempty"`
	Source       string `json:"source"`
	Effect       string `json:"effect"`
	SafetyStatus string `json:"safety_status"`
}

type MemoryClass string

const (
	MemoryClassUserPreference     MemoryClass = "user_preference"
	MemoryClassUserProfile        MemoryClass = "user_profile"
	MemoryClassTaskContinuity     MemoryClass = "task_continuity"
	MemoryClassApprovedSummary    MemoryClass = "approved_summary"
	MemoryClassRuntimeObservation MemoryClass = "runtime_observation"
	MemoryClassQuarantined        MemoryClass = "quarantined"
)

type MemoryEntryStatus string

const (
	MemoryEntryStatusActive     MemoryEntryStatus = "active"
	MemoryEntryStatusSuperseded MemoryEntryStatus = "superseded"
	MemoryEntryStatusQuarantined MemoryEntryStatus = "quarantined"
)

type MemoryEntry struct {
	ID               int64             `json:"id"`
	Class            MemoryClass        `json:"class"`
	Key              string            `json:"key"`
	Value            string            `json:"value"`
	Status           MemoryEntryStatus  `json:"status"`
	Source           string            `json:"source"`
	Effect           string            `json:"effect"`
	TrustTier        int               `json:"trust_tier"`
	QuarantineReason string            `json:"quarantine_reason,omitempty"`
	CreatedAt        string            `json:"created_at"`
	UpdatedAt        string            `json:"updated_at"`
}

type VerificationStep struct {
	Description string `json:"description"`
	Status      string `json:"status"`
}

type EvidenceBoundary struct {
	VerifiedFacts        []string          `json:"verified_facts"`
	ExecutedSteps        []string          `json:"executed_steps"`
	UnverifiedClaims     []string          `json:"unverified_claims"`
	BlockedReasons       []string          `json:"blocked_reasons"`
	NextVerificationStep *VerificationStep `json:"next_verification_step,omitempty"`
}

type RuntimeState struct {
	MiloState                  string            `json:"milo_state"`
	CurrentRoomID              string            `json:"current_room_id"`
	CurrentAnchorID            string            `json:"current_anchor_id"`
	LastMeaningfulUserActionAt string            `json:"last_meaningful_user_action_at"`
	ActiveTask                 *TaskSnapshot     `json:"active_task,omitempty"`
	QueuedTask                 *TaskSnapshot     `json:"queued_task,omitempty"`
	PendingApproval            *ApprovalState    `json:"pending_approval,omitempty"`
	ResumeCheckpoint           *ResumeCheckpoint `json:"resume_checkpoint,omitempty"`
	RuntimeID                  string            `json:"runtime_id"`
}
