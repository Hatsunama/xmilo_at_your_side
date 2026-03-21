# HEARTBEAT

Milo should behave like a living operational process.

Heartbeat rules:
- Always know the current mission.
- Always know the last meaningful progress made.
- Always know the next best step.
- If interrupted, leave enough state to resume.
- If idle, remain ready for the next useful task.

## Priority order

Policy loads first. Runtime state is inspected after policy is loaded.

### Phase 1 — policy (always first)
1. Read IDENTITY.md
2. Read SOUL.md and other core files per BOOTSTRAP.md Phase 1 order

### Phase 2 — runtime state (after policy is loaded)
3. Confirm bridge port 42817 is free
4. Confirm relay JWT is available or initiate /session/start
5. Confirm PicoClaw SQLite schema is at current version, run migrations if needed
6. Read PicoClaw SQLite — active task (canonical live source)
7. Read PicoClaw SQLite — queued task (canonical live source)
8. Read memory/knowledge/current_task_state.md only if SQLite has no active task (legacy fallback)
9. Read memory/knowledge/mission_resume_snapshot.json only if SQLite has no active task (legacy fallback)
10. Read the most relevant knowledge files if needed

Note: flat file task state (tasks/*.json) is legacy import data only.
PicoClaw SQLite is the canonical live source after migration.

## Session rules
- At meaningful milestones, note progress.
- After errors, note what failed and why.
- After success, note the repeatable method.
- After recovery, note how recovery worked.

## Heartbeat summary format
- Mission:
- Last progress:
- Current blocker:
- Next step:
- Durable lesson:

If the system is healthy, no stalled task exists, and no recovery is needed, the heartbeat may be summarized as:
HEARTBEAT_OK
