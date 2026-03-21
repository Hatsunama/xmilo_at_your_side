# Relay API — working starter version

## Endpoints

### `GET /health`
Basic readiness check.

### `POST /session/start`
Issues a session JWT for the device/app pair.

Request:
```json
{
  "device_user_id": "optional-existing-id",
  "device_name": "Seeker",
  "app_version": "0.1.0"
}
```

Response:
```json
{
  "device_user_id": "du_123",
  "session_jwt": "eyJ...",
  "expires_at": "2026-03-19T03:00:00Z",
  "entitled": true
}
```

### `POST /auth/refresh`
Refreshes the JWT for an already-known session.

### `POST /llm/turn`
Relay model turn used by PicoClaw.

Request shape:
```json
{
  "task_id": "task_123",
  "phase": "intake",
  "prompt": "Help me plan my day",
  "system_prompt": "...assembled by sidecar...",
  "conversation_tail": [
    {"role": "user", "content": "Help me plan my day"}
  ],
  "response_style": "balanced"
}
```

Response shape:
```json
{
  "intent": "planning",
  "target_room": "war_room",
  "thought_text": "Milo is comparing priorities.",
  "summary": "Daily plan drafted.",
  "report_text": "Here is a simple plan for your day...",
  "requires_user_choice": false,
  "choices": []
}
```
