# Milo's Mind v4

This folder is the canonical Milo policy and knowledge layer for the Wizard Lair / xMilo Sidecar era.

## What this is

Milo's Mind v4 is NOT a runtime.
It is the policy and durable knowledge layer loaded by the xMilo Sidecar Go sidecar.

xMilo Sidecar Go is the only runtime authority for:
- task state
- conversation tail
- relay calls
- WebSocket events
- room routing
- JWT / auth lifecycle
- inactivity timer
- bridge endpoints
- archive / trophy emission

Mind v4 is authoritative for:
- identity
- behavior rules
- security policy
- bootstrap order
- capability gate model
- recovery philosophy
- device capability knowledge
- reusable procedures
- durable lessons and preferences

## Load order

xMilo Sidecar reads files in this order:

### Phase 1 — Policy load
1. IDENTITY.md
2. SOUL.md
3. SECURITY.md
4. TOOLS.md
5. USER.md
6. HEARTBEAT.md
7. MEMORY.md
8. AGENTS.md
9. BOOTSTRAP.md
10. memory/tacit/rules.md
11. memory/tacit/preferences.md
12. memory/tacit/goals.md
13. memory/tacit/security.md

### Phase 2 — Runtime state inspection (after policy load)
14. memory/knowledge/device_capability_profile.json (if device work may be relevant)
15. memory/knowledge/current_task_state.md (legacy import only, one-time)
16. memory/knowledge/mission_resume_snapshot.json (legacy import only, one-time)
17. xMilo Sidecar SQLite — active task (canonical live source)
18. xMilo Sidecar SQLite — queued task (canonical live source)

### Phase 3 — Knowledge (on demand)
19. memory/knowledge/* as relevant to current mission

## Non-goals
- Mind v4 does not drive task execution
- Mind v4 does not own heartbeat loop or recovery loop
- Mind v4 does not route to rooms
- Mind v4 does not call the relay
- Mind v4 does not manage the conversation tail
- Mind v4 shell scripts are maintenance utilities only, not runtime authority

## Build provenance
Derived from the recovered Milo/OpenClaw workspace, validated device-capability scan structure, Lair spec behavior decisions v1–v19, and the xMilo Mind Merge Spec v1.

## Integrated package additions
This `xMilo_v1` package also includes:
- `INTEGRATION_AUTHORITY.md` — the precedence and anti-drift map for the full package
- `PACKAGE_MANIFEST.md` — package contents and intended use
- `specs/Lair_Blocker_Answers_v16/*` — app/product/platform locks kept alongside the Mind pack
- `xMilo_Sidecar_Go_Responsibility_Checklist.md` — engine implementation checklist, including the separate WebSocket reliability rules
