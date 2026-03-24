package contracts

type EventEnvelope struct {
	Type      string      `json:"type"`
	Timestamp string      `json:"timestamp"`
	Payload   interface{} `json:"payload"`
}

type TurnMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type RelayTurnRequest struct {
	TaskID           string        `json:"task_id"`
	Phase            string        `json:"phase"`
	Prompt           string        `json:"prompt"`
	SystemPrompt     string        `json:"system_prompt"`
	ConversationTail []TurnMessage `json:"conversation_tail"`
	ResponseStyle    string        `json:"response_style"`
}

type ExpectedCheck struct {
	CheckType     string `json:"check_type"`
	Key           string `json:"key,omitempty"`
	ExpectedValue string `json:"expected_value,omitempty"`
}

type UpdatedCheckpoint struct {
	Status    string `json:"status"`
	Blocker   string `json:"blocker,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

type ExecutionResult struct {
	Status            string             `json:"status"`
	Verified          bool               `json:"verified"`
	ResultSummary     string             `json:"result_summary"`
	BlockingReason    string             `json:"blocking_reason,omitempty"`
	UpdatedCheckpoint *UpdatedCheckpoint `json:"updated_checkpoint,omitempty"`
}

type RelayTurnResponse struct {
	Intent             string           `json:"intent"`
	TargetRoom         string           `json:"target_room"`
	ThoughtText        string           `json:"thought_text"`
	Summary            string           `json:"summary"`
	ReportText         string           `json:"report_text"`
	CompletionStatus   string           `json:"completion_status"`
	ContinuationStatus string           `json:"continuation_status,omitempty"`
	NextBlocker        string           `json:"next_blocker,omitempty"`
	ActionType         string           `json:"action_type,omitempty"`
	ActionPayload      map[string]any   `json:"action_payload,omitempty"`
	ExpectedCheck      *ExpectedCheck   `json:"expected_check,omitempty"`
	ExecutionResult    *ExecutionResult `json:"execution_result,omitempty"`
	RequiresUserChoice bool             `json:"requires_user_choice"`
	Choices            []string         `json:"choices"`
}

type SessionStartRequest struct {
	DeviceUserID string `json:"device_user_id"`
	DeviceName   string `json:"device_name"`
	AppVersion   string `json:"app_version"`
}

type SessionStartResponse struct {
	DeviceUserID        string `json:"device_user_id"`
	SessionJWT          string `json:"session_jwt"`
	ExpiresAt           string `json:"expires_at"`
	Entitled            bool   `json:"entitled"`
	AccessMode          string `json:"access_mode"`
	AccessCodeOnly      bool   `json:"access_code_only"`
	TrialAllowed        bool   `json:"trial_allowed"`
	SubscriptionAllowed bool   `json:"subscription_allowed"`
	AccessCodeGrantDays int    `json:"access_code_grant_days"`
	VerifiedEmail       string `json:"verified_email,omitempty"`
	EmailVerified       bool   `json:"email_verified"`
	TwoFactorEnabled    bool   `json:"two_factor_enabled"`
	TwoFactorOK         bool   `json:"two_factor_ok"`
	WebsiteHandoffReady bool   `json:"website_handoff_ready"`
}

type CueDescriptor struct {
	VoiceCue    string `json:"voice_cue,omitempty"`
	PhysicalCue string `json:"physical_cue,omitempty"`
	Description string `json:"description,omitempty"`
}

type RitualArtState struct {
	Status      string        `json:"status"`
	Title       string        `json:"title"`
	Chamber     string        `json:"chamber"`
	Description string        `json:"description"`
	Cues        CueDescriptor `json:"cues"`
}

type RoomPresenceState struct {
	RoomID    string `json:"room_id"`
	AnchorID  string `json:"anchor_id"`
	MiloState string `json:"milo_state"`
}

type MovementIntentState struct {
	FromRoom    string `json:"from_room,omitempty"`
	FromAnchor  string `json:"from_anchor,omitempty"`
	ToRoom      string `json:"to_room,omitempty"`
	ToAnchor    string `json:"to_anchor,omitempty"`
	Reason      string `json:"reason,omitempty"`
	EstimatedMS int    `json:"estimated_ms,omitempty"`
}

type ArtDegradationState struct {
	Reason  string `json:"reason,omitempty"`
	Surface string `json:"surface,omitempty"`
	Message string `json:"message,omitempty"`
}

type ArtPresentationState struct {
	TaskState         string               `json:"task_state"`
	RoomPresence      RoomPresenceState    `json:"room_presence"`
	MovementIntent    *MovementIntentState `json:"movement_intent,omitempty"`
	RitualState       *RitualArtState      `json:"ritual_state,omitempty"`
	DegradationReason *ArtDegradationState `json:"degradation_reason,omitempty"`
}
