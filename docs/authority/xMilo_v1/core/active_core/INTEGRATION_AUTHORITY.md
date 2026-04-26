# xMilo_v1 — Integration Authority

This package is the single integrated authority pack for the current xMilo / xMilo Sidecar rebuild.

## Purpose

This package exists to stop architectural drift.
It keeps the patched **Mind v4** pack, the **xMilo Sidecar Go runtime checklist**, and the **Wizard Lair blocker/product locks** together in one place.

## Core runtime model

- **Mind v4** is the policy and durable knowledge layer.
- **xMilo Sidecar Go** is the only runtime authority.
- **SQLite** is the canonical live task/state store after migration.
- **Legacy JSON/flat files** are import-once or retired.
- **Shell scripts** are optional maintenance utilities only, unless explicitly marked retired evidence.
- **Governed retrieval/search** over approved authority and memory files is runtime retrieval infrastructure only, never independent canon.

## Retrieval/search authority model

- Markdown authority files remain the canonical human-readable truth layer.
- Retrieval/search backends only retrieve the minimum relevant approved material; they do not replace canon.
- Retrieval/search must preserve provenance, type, trust boundaries, and canonical authority order.
- Retrieval/search must fail conservatively when authority or memory retrieval is missing, quarantined, or suspicious.
- Search/index infrastructure must never promote external content into durable policy by implication.
- Backend implementation name is `PROVISIONAL UNTIL VERIFIED` until the current build/runtime confirms the selected implementation, config path, indexing defaults, and fallback chain.

## Canonical authority order

When two documents touch the same area, use this order:

1. Root policy files in this package (`IDENTITY.md`, `SOUL.md`, `SECURITY.md`, `TOOLS.md`, `USER.md`, `HEARTBEAT.md`, `MEMORY.md`, `AGENTS.md`, `BOOTSTRAP.md`)
2. `xMilo_Sidecar_Go_Responsibility_Checklist.md` for engine/runtime implementation scope
3. `specs/Lair_Blocker_Answers_v16/*` for app-shell, network-loss, subscription, platform, and UX locks
4. Verified runtime/config/state
5. Governed retrieval/search over approved files
6. Knowledge-layer files under `memory/knowledge/*` and other approved on-demand authority files retrieved selectively by relevance
7. Retired or migrated evidence files only as historical reference, never as live authority

## Retry mechanisms — do not merge these accidentally

There are **two different retry systems** and they are both locked:

### 1. Relay/network call retry (v16 product lock)
Source: `specs/Lair_Blocker_Answers_v16/01_APP_SHELL_AND_NETWORK_LOCKS_v16.md`

- xMilo Sidecar retries a failed relay call **3 times** with backoff:
  - 2s
  - 5s
  - 10s
- if all fail:
  - emit `runtime.error` with `recoverable = true`
  - transition active task to `task.stuck`

### 2. WebSocket reconnect backoff (v17 locked behavior, captured in checklist)
Source: `xMilo_Sidecar_Go_Responsibility_Checklist.md` → `## 17. WebSocket Reliability`

- WebSocket reconnect backoff is separate from relay retry
- sequence:
  - 1s
  - 2s
  - 5s
  - 10s
  - then repeat 10s until reconnected or a higher-level stop condition applies

Do **not** collapse these into one generic retry rule.
They solve different failure modes.

## Package layout

- Root files = thin policy and startup authority
- `memory/` = canonical memory policy plus tacit/knowledge subfiles retrieved selectively by relevance
- `scripts/` = maintenance utilities or retired evidence
- `tasks/`, `state/` = migrated evidence only, not runtime authority
- `specs/Lair_Blocker_Answers_v16/` = app/product/platform locks
- `xMilo_Sidecar_Go_Responsibility_Checklist.md` = engine implementation checklist

## Safe implementation order

1. Bootstrap, SQLite schema, and one-time legacy import
2. HTTP bridge + WebSocket server + event journal
3. JWT/session lifecycle and `/auth/refresh`
4. Task lifecycle + timeout + stuck/queue rules
5. Relay retry + WebSocket reconnect reliability
6. Room routing + movement events
7. Archive, trophy, report, reset, storage, and inactivity polish

## Non-goals

- Do not revive shell scripts as runtime authority
- Do not move conversation tail back into flat files
- Do not reintroduce multi-source live task truth
- Do not mix app-shell locks into policy files unless a lock is intentionally promoted

## Minimal Permission Grant Rule

- Main Hub owns cross-lane decisions, blocker resolution, scope boundaries, and final integration authority.
- Lanes do not self-authorize beyond their lane.
- When a lane reports:
  1. one concrete blocker,
  2. the exact fail seam,
  3. the next smallest unblocker,
  4. and that next step stays inside the lane’s proper scope,
  then Main Hub should explicitly grant the smallest permissioned next step required to continue.

### Required Main Hub behavior

When the above conditions are met, Main Hub must choose one of these outcomes explicitly:

1. **GRANT**
   - authorize the narrow next step
   - name the lane
   - name the exact allowed action
   - state the scope boundary
   - state the proof required after execution
2. **OPERATOR-ONLY**
   - state that the next step cannot be delegated to the lane
   - state the exact operator action required
   - state what proof the lane should collect immediately after operator action
3. **DENY / REDIRECT**
   - only if the requested step is outside lane scope, conflicts with canon, or requires a different lane
   - state the single exact reason

### Guardrails

- Grant only the smallest necessary step, not broad standing permission.
- Do not grant permissions that rewrite canon, safety, approval doctrine, payment/entitlement truth, trust/consent flows, or other protected surfaces.
- Do not let lanes widen their own mission through blocker handling.
- Do not require user involvement when Main Hub can safely grant a narrow next step itself.
- If a blocker is external, classify it plainly instead of letting lanes loop.

### Required packet shape for each permission grant

- `FROM_HUB: Main Hub`
- `TO_LANE: <lane name>`
- `PERMISSION_TYPE: <minimal delegated step | operator-only | deny/redirect>`
- `TASK_ID: <task id>`
- `ALLOWED_ACTION:`
- `SCOPE_LIMITS:`
- `REQUIRED_PROOF_AFTER_ACTION:`
- `IF_FAIL_REPORT:`
- `NEXT_HUB_DECISION_POINT:`

### Special runtime/device rule

- For verified runtime/device blockers:
  - if a lane has already proven the blocker is below its current available remote control surface,
  - and the next step is a narrow runtime recovery or truth-check step,
  - Main Hub should either:
    - explicitly grant that exact recovery/verification step to the owning lane, or
    - explicitly mark it operator-only.
- Never leave the lane in repeat limbo.

### Acceptance criteria

- A lane with a verified narrow blocker no longer has to keep asking the user for routine permission escalation.
- Main Hub becomes responsible for issuing the smallest proper permission grant when the next step is clear.
- Permissions remain narrow, explicit, reviewable, and task-bounded.
- Lanes still cannot self-expand scope.
- External/operator-only dependencies are classified explicitly instead of causing repeat loops.

## Package naming

Root folder and zip label for this integrated pack:
- `xMilo_v1`
