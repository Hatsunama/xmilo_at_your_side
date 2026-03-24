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

## Canonical authority order

When two documents touch the same area, use this order:

1. Root policy files in this package (`IDENTITY.md`, `SOUL.md`, `SECURITY.md`, `TOOLS.md`, `USER.md`, `HEARTBEAT.md`, `MEMORY.md`, `AGENTS.md`, `BOOTSTRAP.md`)
2. `xMilo_Sidecar_Go_Responsibility_Checklist.md` for engine/runtime implementation scope
3. `specs/Lair_Blocker_Answers_v16/*` for app-shell, network-loss, subscription, platform, and UX locks
4. Knowledge-layer files under `memory/knowledge/*` for durable procedures and operational memory
5. Retired or migrated evidence files only as historical reference, never as live authority

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

- Root files = policy and startup authority
- `memory/` = tacit and knowledge layers
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

## Package naming

Root folder and zip label for this integrated pack:
- `xMilo_v1`
