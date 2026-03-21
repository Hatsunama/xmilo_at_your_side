# WebSocket Events — working starter version

WebSocket URL:

`ws://127.0.0.1:42817/ws?token=<localhost_token>`

## Implemented / emitted now

- `runtime.ready`
- `runtime.error`
- `task.accepted`
- `task.progress`
- `task.completed`
- `task.stuck`
- `task.cancelled`
- `report.ready`
- `archive.record_created`
- `milo.state_changed`
- `milo.room_changed`
- `milo.movement_started`
- `milo.thought`

## Deferred but reserved

- `task.requires_user_choice`
- `task.entitlement_lost`
- `task.queued`
- `relay.auth_required`
- `trophy.created`
- `trophy.conjure_failed`

## Event envelope

```json
{
  "type": "task.progress",
  "timestamp": "2026-03-19T02:00:00Z",
  "payload": {
    "task_id": "task_123",
    "message": "Milo is reasoning."
  }
}
```
