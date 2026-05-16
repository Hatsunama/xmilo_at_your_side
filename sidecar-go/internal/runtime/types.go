package runtime

import "xmilo/sidecar-go/internal/runtimegate"

type HealthResponse struct {
	OK        bool   `json:"ok"`
	Service   string `json:"service"`
	Version   string `json:"version"`
	UptimeSec int64  `json:"uptime_sec"`
}

type ReadyResponse struct {
	OK                    bool   `json:"ok"`
	WakeLockActive        bool   `json:"wake_lock_active"`
	WakeLockStatus        string `json:"wake_lock_status"`
	RuntimeHost           string `json:"runtime_host"`
	DBReady               bool   `json:"db_ready"`
	RelayConfigured       bool   `json:"relay_configured"`
	MindLoaded            bool   `json:"mind_loaded"`
	RuntimeID             string `json:"runtime_id"`
	LLMMode               string `json:"llm_mode"`
	BYOKProvider          string `json:"byok_provider,omitempty"`
	SubscriptionEntitled  bool   `json:"subscription_entitled"`
	BringYourOwnKeyActive bool   `json:"bring_your_own_key_active"`
	Phase9APIKeyAccess    bool   `json:"phase9_api_key_access"`
	FirstTaskEligible     bool   `json:"first_task_eligible"`
	RelayLLMTurnAllowed   bool   `json:"relay_llm_turn_allowed"`
	LocalLLMTurnAllowed   bool   `json:"local_llm_turn_allowed"`
}

type TaskSnapshot struct {
	TaskID           string            `json:"task_id"`
	AttemptID        string            `json:"attempt_id"`
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
	AttemptID          string            `json:"attempt_id"`
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
	PrimaryClass       string                         `json:"primary_class"`
	SecondaryFlags     []string                       `json:"secondary_flags"`
	TrustTier          int                            `json:"trust_tier"`
	ValidationState    string                         `json:"validation_state"`
	ChosenClosedAction string                         `json:"chosen_closed_action"`
	MemoryIntent       *MemoryIntent                  `json:"memory_intent,omitempty"`
	SafetyDecision     *runtimegate.SanitizedDecision `json:"safety_decision,omitempty"`
}

type MemoryIntent struct {
	Class        string `json:"class"`
	Source       string `json:"source"`
	Effect       string `json:"effect"`
	SafetyStatus string `json:"safety_status"`
}

type VerificationStep struct {
	Description string `json:"description"`
	Status      string `json:"status"`
}

type EvidenceBoundary struct {
	VerifiedFacts        []string            `json:"verified_facts"`
	ExecutedSteps        []string            `json:"executed_steps"`
	UnverifiedClaims     []string            `json:"unverified_claims"`
	BlockedReasons       []string            `json:"blocked_reasons"`
	NextVerificationStep *VerificationStep   `json:"next_verification_step,omitempty"`
	CompletionEvidence   *CompletionEvidence `json:"completion_evidence,omitempty"`
	AppBridgeEvidence    []AppBridgeEvidence `json:"app_bridge_evidence,omitempty"`
}

type CompletionEvidence struct {
	ProofClass            string `json:"proof_class"`
	RequiredForCompletion bool   `json:"required_for_completion"`
	Verified              bool   `json:"verified"`
	Source                string `json:"source"`
	Summary               string `json:"summary"`
	BlockingReason        string `json:"blocking_reason,omitempty"`
	CheckedAt             string `json:"checked_at"`
}

type AppBridgeEvidence struct {
	ProofClass     string         `json:"proof_class"`
	Verified       bool           `json:"verified"`
	Source         string         `json:"source"`
	Operation      string         `json:"operation"`
	CheckedAt      string         `json:"checked_at"`
	Summary        string         `json:"summary"`
	BlockingReason string         `json:"blocking_reason,omitempty"`
	EvidenceID     string         `json:"evidence_id,omitempty"`
	AttemptID      string         `json:"attempt_id,omitempty"`
	TaskID         string         `json:"task_id,omitempty"`
	Details        map[string]any `json:"details,omitempty"`
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
	CapabilityState            map[string]any    `json:"capability_state,omitempty"`
	RuntimeID                  string            `json:"runtime_id"`
}
