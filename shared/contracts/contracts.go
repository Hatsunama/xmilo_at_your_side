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

type RelayTurnResponse struct {
    Intent             string   `json:"intent"`
    TargetRoom         string   `json:"target_room"`
    ThoughtText        string   `json:"thought_text"`
    Summary            string   `json:"summary"`
    ReportText         string   `json:"report_text"`
    RequiresUserChoice bool     `json:"requires_user_choice"`
    Choices            []string `json:"choices"`
}

type SessionStartRequest struct {
    DeviceUserID string `json:"device_user_id"`
    DeviceName   string `json:"device_name"`
    AppVersion   string `json:"app_version"`
}

type SessionStartResponse struct {
    DeviceUserID string `json:"device_user_id"`
    SessionJWT   string `json:"session_jwt"`
    ExpiresAt    string `json:"expires_at"`
    Entitled     bool   `json:"entitled"`
}
