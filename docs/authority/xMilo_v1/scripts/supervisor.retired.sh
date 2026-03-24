#!/data/data/com.termux/files/usr/bin/bash
set -eu

WORKSPACE="${WORKSPACE:-$HOME/.openclaw/workspace}"
TASKS_FILE="$WORKSPACE/tasks/active.json"
LAST_RECOVERY_FILE="$WORKSPACE/state/last_recovery.txt"
HEARTBEAT_STATE_FILE="$WORKSPACE/state/heartbeat_state.json"
SUPERVISOR_LOG="$WORKSPACE/logs/supervisor.log"

mkdir -p "$WORKSPACE/logs" "$WORKSPACE/state" "$WORKSPACE/tasks"

[ -f "$TASKS_FILE" ] || echo "[]" > "$TASKS_FILE"
[ -f "$LAST_RECOVERY_FILE" ] || echo "0" > "$LAST_RECOVERY_FILE"
[ -f "$HEARTBEAT_STATE_FILE" ] || echo "{}" > "$HEARTBEAT_STATE_FILE"

log_line() {
  printf '%s %s\n' "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" "$1" >> "$SUPERVISOR_LOG"
}

bash "$WORKSPACE/scripts/daily_rollover.sh" || true

NOW_EPOCH="$(date -u +%s)"
LAST_RECOVERY="$(cat "$LAST_RECOVERY_FILE" 2>/dev/null || echo 0)"

STALLED_COUNT="$(
python3 - <<'PY'
import json, os, time, datetime
path = os.path.expanduser(os.environ.get("WORKSPACE", "~/.openclaw/workspace")) + "/tasks/active.json"
now = int(time.time())

def to_epoch(value):
    if not value:
        return 0
    try:
        value = value.replace("Z", "+00:00")
        return int(datetime.datetime.fromisoformat(value).timestamp())
    except Exception:
        return 0

try:
    with open(path, "r", encoding="utf-8") as f:
        data = json.load(f)
except Exception:
    data = []

count = 0
for task in data:
    status = task.get("status", "")
    stale_after = int(task.get("stale_after_minutes", 45))
    retry_count = int(task.get("retry_count", 0))
    max_retries = int(task.get("max_retries", 3))
    updated_at = task.get("updated_at") or task.get("started_at") or ""
    updated_epoch = to_epoch(updated_at)
    age_minutes = 999999 if updated_epoch == 0 else int((now - updated_epoch) / 60)
    if status in ("running", "working", "retrying") and age_minutes >= stale_after and retry_count < max_retries:
        count += 1
print(count)
PY
)"

if [ "${STALLED_COUNT:-0}" -eq 0 ]; then
cat <<JSON > "$HEARTBEAT_STATE_FILE"
{
  "status": "healthy",
  "timestamp_utc": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
  "stalled_count": 0
}
JSON
  log_line "Heartbeat clean."
  bash "$WORKSPACE/scripts/mission_loop.sh" || true
  exit 0
fi

if [ $((NOW_EPOCH - LAST_RECOVERY)) -lt 1200 ]; then
cat <<JSON > "$HEARTBEAT_STATE_FILE"
{
  "status": "cooldown",
  "timestamp_utc": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
  "stalled_count": $STALLED_COUNT
}
JSON
  log_line "Stalled tasks detected but recovery suppressed due to cooldown."
  exit 0
fi

echo "$NOW_EPOCH" > "$LAST_RECOVERY_FILE"
cat <<JSON > "$HEARTBEAT_STATE_FILE"
{
  "status": "recovery_needed",
  "timestamp_utc": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
  "stalled_count": $STALLED_COUNT
}
JSON

log_line "Detected ${STALLED_COUNT} stalled task(s). Starting recovery flow."
bash "$WORKSPACE/scripts/recover_tasks.sh" || true
exit 0
