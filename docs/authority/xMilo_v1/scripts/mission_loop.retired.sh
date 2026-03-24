#!/data/data/com.termux/files/usr/bin/bash
set -eu

WORKSPACE="${WORKSPACE:-$HOME/.openclaw/workspace}"
QUEUE="$WORKSPACE/tasks/mission_queue.json"
ACTIVE="$WORKSPACE/tasks/active.json"

mkdir -p "$WORKSPACE/tasks"

[ -f "$QUEUE" ] || echo "[]" > "$QUEUE"
[ -f "$ACTIVE" ] || echo "[]" > "$ACTIVE"

python3 <<'PY'
import json, os, datetime

workspace = os.path.expanduser(os.environ.get("WORKSPACE", "~/.openclaw/workspace"))
queue_file = f"{workspace}/tasks/mission_queue.json"
active_file = f"{workspace}/tasks/active.json"

def load_json(path, default):
    try:
        with open(path, "r", encoding="utf-8") as f:
            return json.load(f)
    except Exception:
        return default

def save_json(path, data):
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2)

def score(task):
    priority = int(task.get("priority", 0))
    value = int(task.get("value", 0))
    urgency = int(task.get("urgency", 0))
    difficulty = int(task.get("difficulty", 0))
    return (priority, value, urgency, -difficulty)

queue = load_json(queue_file, [])
active = load_json(active_file, [])

blocking_statuses = {"running", "working", "retrying", "queued", "needs_user_input", "paused"}
if any(t.get("status") in blocking_statuses for t in active):
    raise SystemExit(0)

if not queue:
    raise SystemExit(0)

queue.sort(key=score, reverse=True)
task = dict(queue.pop(0))

now = datetime.datetime.now(datetime.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")
task.setdefault("id", f"task_{int(datetime.datetime.now().timestamp())}")
task.setdefault("title", "Untitled task")
task.setdefault("priority", 5)
task.setdefault("value", 5)
task.setdefault("urgency", 5)
task.setdefault("difficulty", 3)
task.setdefault("retry_count", 0)
task.setdefault("max_retries", 3)
task.setdefault("stale_after_minutes", 45)
task["status"] = "running"
task["started_at"] = now
task["updated_at"] = now

save_json(queue_file, queue)
save_json(active_file, [task])
PY

exit 0
