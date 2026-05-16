# WebSocket Events — working starter version

WebSocket URL:

`ws://127.0.0.1:42817/ws?token=<localhost_token>`

## Implemented / emitted now

- `archive.record_created`
- `local_provider.diagnostic`
- `milo.movement_started`
- `milo.room_changed`
- `milo.state_changed`
- `milo.thought`
- `report.ready`
- `runtime.capability_degraded`
- `runtime.capability_state`
- `runtime.ready`
- `runtime.error`
- `task.accepted`
- `task.awaiting_user_choice`
- `task.blocked`
- `task.cancelled`
- `task.choice_recorded`
- `task.completed`
- `task.entitlement_lost`
- `task.intake_evaluated`
- `task.message_emitted`
- `task.progress`
- `task.result_unverified`
- `task.resume_blocked`
- `task.resumable`
- `task.stale_active_recovered`
- `task.stuck`
- `maintenance.nightly_deferred`
- `maintenance.nightly_started`
- `maintenance.nightly_completed`

## Deferred but reserved

- `task.requires_user_choice`
- `task.queued`
- `relay.auth_required`
- `trophy.created`
- `trophy.conjure_failed`

## Safety Decision Contract

Phase 14 runtime gates currently expose sanitized safety decisions through existing runtime/task response structures, such as `intake_assessment.safety_decision`. No standalone WebSocket event is emitted for safety decisions in this slice.

Sanitized safety decisions may include only these stable fields:

- `outcome`
- `reason_code`
- `gate_phase`
- `action_family`
- `action_name`
- `evidence_required`
- `source_trust_tier`
- `user_safe_message`
- `safe_summary`
- `created_at`

Approved `outcome` values:

- `allow`
- `block`
- `clarify`
- `confirm`
- `safe_redirect`

Approved `gate_phase` values:

- `pre_task`
- `pre_context_injection`
- `pre_model_action`
- `pre_tool_action`
- `pre_completion`
- `pre_memory_write`

Approved `reason_code` values:

- `none`
- `harmful_request`
- `prompt_injection_authority_spoof`
- `external_content_attempted_command`
- `missing_permission_or_capability`
- `missing_tool_proof`
- `missing_provider_access_route`
- `missing_approval`
- `unsafe_automation`
- `destructive_action`
- `privacy_surveillance_risk`
- `credential_secret_risk`
- `completion_evidence_missing`
- `unknown_malformed_action`
- `unbounded_consumption_risk`

Safety decision and runtime-visible surfaces must not include `raw_prompt`, `system_prompt`, `developer_prompt`, `hidden_prompt`, `chain_of_thought`, `scratchpad`, `internal_detail`, raw model output, raw context payloads, raw memory/archive payloads, raw tool payloads, private tool payloads, secrets, API keys, tokens, auth headers, provider config, request bodies, response bodies, or credentials.

## Source boundaries

- Sidecar WebSocket events are sidecar runtime truth and are tagged as `sidecar_runtime` by the app bridge when they enter the Expo runtime.
- `runtime.error` is reserved for sidecar-origin runtime failures.
- UI-local submit/display failures must use `ui_local.error` with source `ui_local` and truth scope `ui_submit`; they are not sidecar runtime events.
- Android bridge observations remain `android_bridge` proof or `android_bridge_observation` recovery observations. They are not sidecar runtime truth.

## Recovery result boundaries

Recovery result semantics are source-tagged. The app may orchestrate Android bridge recovery because it owns the app-side process bridge, but that budget and restart loop are Android bridge recovery orchestration, not sidecar runtime truth.

- `restart_verified` requires observed post-action readiness such as sidecar ready or the task route surface.
- `restart_failed` means the bridge attempted or checked recovery and observed a failed/non-ready result.
- `restart_rate_limited` means the app-side Android bridge recovery budget is currently in backoff.
- `restart_needs_operator` means the Android bridge recovery path cannot continue without operator action.

Sidecar runtime truth remains sidecar-origin health, readiness, task state, and emitted runtime events. UI-local errors are diagnostics/display failures and do not establish runtime truth.

## Art-facing notes

- `maintenance.nightly_deferred`, `maintenance.nightly_started`, and `maintenance.nightly_completed` are the current truth source for nightly ritual presentation.
- Castle/lair visuals must key off those real maintenance events rather than inventing a separate ritual scheduler.
- If native castle rendering is unavailable or over budget, the Expo shell/lair fallback remains the honest presentation path.

## Event envelope

```json
{
  "type": "task.progress",
  "timestamp": "2026-03-19T02:00:00Z",
  "payload": {
    "task_id": "task_123",
    "attempt_id": "attempt_123",
    "message": "Milo is reasoning."
  }
}
```

UI-local submit/display error example:

```json
{
  "type": "ui_local.error",
  "timestamp": "2026-03-19T02:00:00Z",
  "source": "ui_local",
  "truth_scope": "ui_submit",
  "payload": {
    "message": "Local submit failed before sidecar accepted the task.",
    "recoverable": true,
    "source": "ui_local",
    "truth_scope": "ui_submit"
  }
}
```
