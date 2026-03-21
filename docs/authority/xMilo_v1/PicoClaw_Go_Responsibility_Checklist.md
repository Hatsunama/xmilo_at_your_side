# PicoClaw Go — Implementation Responsibility Checklist

**Authority:** MiloClaw Mind Merge Spec v1 + Lair spec packs v1–v19
**Purpose:** Complete inventory of everything PicoClaw Go must implement. Use this to prevent drift during development.

Every item here is owned by the Go sidecar. None of these live in Mind v4 files or shell scripts.

---

## 1. Server & Transport

- [ ] HTTP server on `127.0.0.1:42817`
- [ ] WebSocket server on `ws://127.0.0.1:42817/ws`
- [ ] Fixed-per-install bearer token auth on all HTTP + WebSocket endpoints
- [ ] Port conflict detection on startup — fail fast with clear log, do not silently occupy
- [ ] PID lockfile to prevent double-launch
- [ ] Bearer token: app-generated on first launch, injected into PicoClaw config during Termux bootstrap, loaded from config on every start
- [ ] Termux wake-lock (`termux-wake-lock`) acquired immediately at startup
- [ ] Termux wake-unlock on intentional shutdown

---

## 2. Bridge HTTP Endpoints (v10 contract)

- [ ] `GET /health` — ok, service name, version, uptime_sec
- [ ] `GET /ready` — all readiness flags including wake_lock_active
- [ ] `POST /bootstrap` — idempotent, does NOT reset live task state
- [ ] `POST /auth/refresh` — JWT swap only, no task state reset
- [ ] `POST /task/start` — accepts prompt, returns task_id and immediate_state
- [ ] `GET /task/current` — current task snapshot
- [ ] `POST /task/choice` — responds to user-driven choices after task.requires_user_choice
- [ ] `POST /task/interrupt` — immediate stop, Milo routes to Main Hall
- [ ] `POST /task/cancel` — cancel task, return error if task active during reset
- [ ] `POST /task/resume_queue` — explicit user re-engagement for queued task, accepts response_style
- [ ] `POST /trophy/conjure` — post-completion trophy creation with relay call
- [ ] `POST /thought/request` — tap-triggered thought, 2s cooldown, rejection with reason
- [ ] `POST /inspector/open` — Archive/Trophy inspector, supports empty_state, join_if_idle, ui_only
- [ ] `POST /inspector/close` — transitions Milo back to idle, resumes inactivity timer
- [ ] `GET /state` — full Milo state snapshot including last_meaningful_user_action_at
- [ ] `GET /storage/stats` — SQLite size, conversation tail tokens, task rows, pending event rows
- [ ] `POST /reset` — wipes per tier, blocks with reason=task_active if task in progress

---

## 3. WebSocket Events (v10 contract)

- [ ] `runtime.ready` — ready flag + wake_lock_active
- [ ] `runtime.error` — message + recoverable flag
- [ ] `runtime.exited` — reason
- [ ] `relay.auth_required` — reason + optional task_id + task_state
- [ ] `task.accepted` — task_id, intent, room_id
- [ ] `task.progress` — task_id, phase, room_id, anchor_id, message
- [ ] `task.requires_user_choice` — task_id, prompt, contextual choices subset only
- [ ] `task.cancelled` — task_id, reason
- [ ] `task.completed` — task_id, summary, trophy_eligible
- [ ] `task.stuck` — task_id, reason, recovery_options
- [ ] `task.entitlement_lost` — task_id, reason, paywall_required=true
- [ ] `task.queued` — task_id, prompt_summary (real-time queue confirmation)
- [ ] `task.auth_refresh_needed` — removed; relay.auth_required covers this with task context
- [ ] `milo.thought` — text, style, trigger (auto or tap)
- [ ] `milo.room_changed` — room_id, anchor_id (null on transit arrivals)
- [ ] `milo.movement_started` — from_room, from_anchor, to_room, to_anchor, reason, estimated_ms
- [ ] `milo.state_changed` — from_state, to_state (meaningful semantic boundaries only, NOT moving→moving)
- [ ] `report.ready` — task_id, report_text, style
- [ ] `archive.record_created` — full record payload
- [ ] `trophy.created` — full trophy payload
- [ ] `trophy.conjure_failed` — task_id, reason

---

## 4. SQLite Schema (with migrations)

- [ ] `schema_version` table with ordered forward migration scripts
- [ ] Active task record (one max): task_id, prompt, intent, room_id, anchor_id, status, started_at, updated_at, retry_count, max_retries, failure_type, stuck_reason
- [ ] Queued task record (one max): same fields, queued_at
- [ ] Conversation tail: ordered turns with role + content, trimmed to last 12 turns / ~4000 tokens
- [ ] Pending event journal: unsynced events for app reconnect replay
- [ ] Runtime config: device_user_id, relay_session_jwt, expires_at, response_style, thought_bubbles_enabled, speak_out_loud_enabled, follow_camera_enabled, runtime_id
- [ ] Last meaningful user action timestamp
- [ ] Legacy import completed flag (prevents double import)
- [ ] Device capability revalidation queue
- [ ] Inactivity timer last reset timestamp

---

## 5. Task Lifecycle

- [ ] One active task, one queued task maximum
- [ ] Casual conversation: room_intent=none, no movement, suppressed milo.state_changed, suppressed milo.thought auto-emit, still emits task.completed + report.ready
- [ ] Task intake: call relay /llm/turn with phase=intake
- [ ] Work loop: call relay at meaningful updates, user nudges, next-reasoning needs
- [ ] Report: Milo routes to Main Hall, arrival beat, call relay phase=report, emit report.ready
- [ ] Stuck: Milo routes to Main Hall, emit task.stuck with recovery options
- [ ] Entitlement loss mid-task: relay returns 403, emit task.entitlement_lost, app opens paywall
- [ ] Interrupt: immediate stop, Milo moves to Main Hall with reason=interrupt
- [ ] Cancel: emit task.cancelled, Milo moves to Main Hall, idle countdown resumes
- [ ] Queue re-engagement: never auto-start, requires /task/resume_queue
- [ ] Task completion event order: task.completed → archive.record_created → report.ready
- [ ] Wall-clock timeout: 10 minutes max, then task.stuck with timeout reason
- [ ] Relay retry on failure: 3 attempts at 2s / 5s / 10s, then task.stuck
- [ ] Relay 500: same retry policy as network loss
- [ ] Unknown target_room from relay: fall back to Main Hall, do not crash

---

## 6. Room Routing

- [ ] Intent → room mapping per locked routing contract (v2 spec Section 6/7)
- [ ] Tie-break rules: Archive beats Library for past records, Observatory beats Library for live monitoring, Alchemy beats Library for transformation, War Room beats Main Hall for tradeoffs, Training beats Library for practice, Main Hall first for ambiguous tasks
- [ ] direct vs via_main_hall route modes
- [ ] Anchor selection per room on arrival
- [ ] Transit room: milo.room_changed with anchor_id=null, next milo.movement_started fires immediately

---

## 7. Movement System

- [ ] Segment-by-segment movement: one milo.movement_started per corridor leg including intra-room leg from anchor to doorway
- [ ] estimated_ms = (path_length_in_tiles / 4.0) * 1000
- [ ] Intra-room departure leg: to_room = same as current room
- [ ] Intermediate segment: to_anchor = null
- [ ] Final arrival leg: to_anchor = target work anchor_id
- [ ] reason enum: task, cancel, sleep, inspect, wander, interrupt
- [ ] Pathing uses doorway/corridor graph from world layout v10, not room bounding boxes
- [ ] World layout file loaded from named map key "wizard_lair_v1"

---

## 8. Inactivity & Sleep

- [ ] 10-minute inactivity timer owned by PicoClaw
- [ ] Timer resets on: /task/start, /task/choice, /task/interrupt, /task/cancel, /trophy/conjure, accepted /thought/request, /task/resume_queue
- [ ] Timer evaluated at stable anchor/state boundaries only — never interrupts mid-corridor
- [ ] Pre-sleep Trophy Room wander: only if Trophy Room has at least one trophy, always deterministic (no random chance)
- [ ] Wander sequence: movement_started reason=wander → arrive → 3000ms dwell → movement_started reason=sleep → arrive Sleep Chamber → sleeping state
- [ ] On inactivity timer fire during movement: complete current corridor segment, then evaluate

---

## 9. Auth & JWT Lifecycle

- [ ] relay /session/start on app launch to obtain JWT + entitlement
- [ ] JWT lifetime: 60 minutes
- [ ] Proactive refresh at T-5 minutes while app is active
- [ ] On relay 401: emit relay.auth_required, pause relay-backed progress, task becomes needs_auth_refresh
- [ ] On app reconnect after JWT expiry: app calls relay /session/start, then PicoClaw /auth/refresh
- [ ] /auth/refresh: JWT swap only, task state preserved
- [ ] If refresh fails or entitlement gone: task transitions to stuck
- [ ] Dev bypass: accept JWTs starting with "DEV_BYPASS." when dev flag enabled

---

## 10. Archive & Trophy

- [ ] Archive records written to expo-sqlite via archive.record_created event
- [ ] Archive creation only from successful phase=report path (never from stuck/recover)
- [ ] Trophy: only after task.completed with trophy_eligible=true + explicit user consent via /trophy/conjure
- [ ] /trophy/conjure calls relay with phase=report, post_action=conjure_trophy
- [ ] If relay returns null trophy_record on conjure: emit trophy.conjure_failed, do not write
- [ ] Duplicate trophy guard: if trophy already exists for task_id, return accepted=false
- [ ] Events are journaled in SQLite for replay on reconnect
- [ ] Replay uses upsert semantics to prevent duplicates in expo-sqlite

---

## 11. Information Room Inspection

- [ ] /inspector/open: three modes — join_if_idle, ui_only, empty_state
- [ ] empty_state: no movement, UI only
- [ ] join_if_idle: Milo walks to room if idle
- [ ] If Milo already moving to same room: accept new item_id, bind as pending target, movement continues
- [ ] If Milo already en route and user changes item in same room: update pending target, no restart
- [ ] /inspector/close: Milo returns to idle at current anchor, inactivity timer resumes
- [ ] milo_state: inspecting_information_room emitted on state_changed when entering

---

## 12. Report Delivery

- [ ] report.ready always emitted for data delivery regardless of toggles
- [ ] thought_bubbles_enabled=false and speak_out_loud_enabled=false: report stored but not auto-surfaced
- [ ] If user taps Milo while pending unsurfaced report exists: re-emit report.ready instead of milo.thought
- [ ] After one re-emit: mark report as surfaced, subsequent taps return to normal milo.thought

---

## 13. Reset

- [ ] POST /reset tiers: archive_only, trophy_only, chat_cache_only, milo_memory_only, full_reset
- [ ] full_reset wipes: SQLite task state, conversation tail, task history, settings toggles
- [ ] full_reset preserves: device_user_id, localhost bearer token, relay will re-issue JWT on next /session/start
- [ ] Block reset with reason=task_active if any task is in progress
- [ ] Write reset_in_progress flag before starting wipe, clear on completion
- [ ] On next launch: if reset_in_progress flag found, complete or prompt retry

---

## 14. Storage

- [ ] GET /storage/stats: pico_sqlite_bytes, conversation_tail_tokens, runtime_task_rows, pending_event_rows
- [ ] Storage warning evaluation: on app open if last check > 24 hours ago, or on Settings entry
- [ ] Warning thresholds: combined managed storage > 250MB or device free < 1GB

---

## 15. Milo's Mind Policy Load

- [ ] At startup: load Mind v4 policy files in Phase 1 order before any runtime state inspection
- [ ] System prompt constructed from IDENTITY.md + SOUL.md behavior rules
- [ ] System prompt prepended as first system-role entry on every /llm/turn request
- [ ] Relay must not override the canonical Milo system prompt in v1
- [ ] Prompt version tracked so future updates can trigger hard cutover behavior

---

## 16. Legacy Import (one-time)

- [ ] Check legacy_import_completed flag in SQLite on first startup
- [ ] If not completed: run import pass on tasks/*.json, current_task_state.md, mission_resume_snapshot.json
- [ ] Import active tasks as stuck tasks requiring user action
- [ ] Import first queued item only (discard remainder)
- [ ] Import completed/failed tasks as task history
- [ ] Import device_capability_profile.json, set revalidation flag for needs_revalidation entries
- [ ] Set legacy_import_completed=true after pass
- [ ] Rename imported files to .migrated — never read again

---

## 17. WebSocket Reliability

- [ ] Auto-reconnect on disconnect: backoff 1s / 2s / 5s / 10s, repeat 10s until connected
- [ ] On reconnect: call GET /state and GET /task/current to reconcile missed events
- [ ] Subtle connection-lost indicator while disconnected
- [ ] Pending event journal replayed on reconnect using upsert semantics

---

## 18. Error Handling (from error audit)

- [ ] Port 42817 conflict: fail fast with clear error, do not silently occupy
- [ ] PID lockfile: detect double-launch, exit second instance cleanly
- [ ] Binary download interrupted: detect partial file via SHA256, delete and retry
- [ ] SHA256 mismatch: block execution, surface error to setup wizard
- [ ] SQLite migration failure: refuse to start, emit clear error, do not run with partial schema
- [ ] Disk full (SQLITE_FULL): catch, emit runtime.error, preserve task in recoverable state
- [ ] All error paths must emit explicit events or log entries — no silent || true swallowing
- [ ] Device clock change: use monotonic uptime for inactivity timer, not wall clock
- [ ] Concurrent /task/resume_queue + /task/start race: mutex around task state writes
- [ ] App OTA version vs sidecar version: check /health version against minimum required, emit update prompt if too old

---

## 19. Nightly Maintenance

- [ ] Wire daily_rollover.sh into scheduled maintenance path (or implement in Go)
- [ ] Wire nightly_consolidate.sh into scheduled maintenance path (or implement in Go)
- [ ] Secret scrub applied before any line is promoted to durable memory
- [ ] Log rotation: supervisor.log and recovery.log at 30 days or 10MB

---

## Status tracking key

- [ ] Not started
- [~] In progress
- [x] Complete and verified
