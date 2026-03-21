# BOOTSTRAP

When Milo starts, follow this order:

1. Confirm the environment is alive.
2. Read the core control files.
3. Check runtime state before re-reasoning.
4. Identify whether there is unfinished work or an active mission.
5. Review durable memory relevant to the current mission.
6. Identify the actual current user goal.
7. Act carefully.
8. Preserve state after meaningful progress.

## Startup checklist

### Phase 1 — policy load
- confirm working directory and file access
- read IDENTITY.md
- read SOUL.md
- read SECURITY.md
- read TOOLS.md
- read USER.md
- read HEARTBEAT.md
- read MEMORY.md
- read AGENTS.md
- read memory/tacit/rules.md
- read memory/tacit/preferences.md
- read memory/tacit/goals.md
- read memory/tacit/security.md

### Phase 2 — runtime readiness checks
- confirm bridge port 42817 is free (fail fast with clear error if occupied)
- confirm relay JWT is available or initiate relay /session/start to obtain one
- confirm PicoClaw SQLite schema_version is current; run forward migrations if needed
- read memory/knowledge/device_capability_profile.json if device work may be relevant
- run legacy import pass if not yet completed (tasks/*.json, mission snapshots → SQLite)
- review PicoClaw SQLite active task (canonical source)
- review PicoClaw SQLite queued task (canonical source)

### Phase 3 — action
- emit runtime.ready when all checks pass
- resume active task or await next user input

## Recovery checklist
If recovering from rate limit, crash, interruption, disconnect, or Termux kill:
- identify what was in progress (read SQLite)
- identify what was completed
- identify what remains
- identify whether the next step is still valid
- emit task.stuck if the active task cannot safely continue without user input
- write a recovery note to memory if needed
- resume cleanly only if safe to do so without user confirmation

## Operating principle
- Start grounded.
- Continue deliberately.
- End with preserved state.

## Autonomy rule
If the mission is clearly bounded and the next steps are known, continue without asking again.
If a real blocker exists, stop and explain it clearly.

## Anti-drift startup rule
At startup, prefer the simplest interpretation that matches the real evidence.

Do not revive:
- stale domain baggage
- stale queues from unrelated past missions
- old external-operations logic

unless the current user goal explicitly requires it.

## Port conflict rule
If port 42817 is occupied on startup:
- log a clear error
- exit cleanly with a descriptive message
- do not attempt to kill the occupying process silently
- surface the port conflict to the app setup/recovery flow
