#!/data/data/com.termux/files/usr/bin/bash
set -eu

WORKSPACE="${WORKSPACE:-$HOME/.openclaw/workspace}"
TASKS_FILE="$WORKSPACE/tasks/active.json"
FAILED_FILE="$WORKSPACE/tasks/failed.json"
COMPLETED_FILE="$WORKSPACE/tasks/completed.json"
LOG_FILE="$WORKSPACE/logs/recovery.log"

mkdir -p "$WORKSPACE/logs" "$WORKSPACE/tasks"

[ -f "$TASKS_FILE" ] || echo "[]" > "$TASKS_FILE"
[ -f "$FAILED_FILE" ] || echo "[]" > "$FAILED_FILE"
[ -f "$COMPLETED_FILE" ] || echo "[]" > "$COMPLETED_FILE"

python3 <<'PY'
import json, os, datetime

workspace = os.path.expanduser(os.environ.get("WORKSPACE", "~/.openclaw/workspace"))
tasks_file = f"{workspace}/tasks/active.json"
failed_file = f"{workspace}/tasks/failed.json"
completed_file = f"{workspace}/tasks/completed.json"
log_file = f"{workspace}/logs/recovery.log"

def now_iso():
    return datetime.datetime.now(datetime.timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")

def load(path, default):
    try:
        with open(path, "r", encoding="utf-8") as f:
            return json.load(f)
    except Exception:
        return default

def save(path, data):
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2)

tasks = load(tasks_file, [])
failed = load(failed_file, [])
completed = load(completed_file, [])

new_active = []

for task in tasks:
    status = task.get("status", "")
    retry_count = int(task.get("retry_count", 0))
    max_retries = int(task.get("max_retries", 3))
    failure_type = task.get("failure_type", "temporary")
    title = task.get("title", "untitled")

    if status == "completed":
        task["completed_at"] = task.get("completed_at") or now_iso()
        completed.append(task)
        continue

    if status in ("cancelled", "failed"):
        task["failed_at"] = task.get("failed_at") or now_iso()
        failed.append(task)
        continue

    if status == "stalled":
        if failure_type == "permanent":
            task["status"] = "failed"
            task["failed_at"] = now_iso()
            failed.append(task)
            with open(log_file, "a", encoding="utf-8") as log:
                log.write(f"{now_iso()} Marked permanent failure: {title}\n")
            continue

        if retry_count < max_retries:
            task["status"] = "retrying"
            task["retry_count"] = retry_count + 1
            task["updated_at"] = now_iso()
            with open(log_file, "a", encoding="utf-8") as log:
                log.write(f"{now_iso()} Retrying task: {title} (retry {task['retry_count']}/{max_retries})\n")
            new_active.append(task)
            continue

        task["status"] = "failed"
        task["failed_at"] = now_iso()
        failed.append(task)
        with open(log_file, "a", encoding="utf-8") as log:
            log.write(f"{now_iso()} Exhausted retries: {title}\n")
        continue

    new_active.append(task)

save(tasks_file, new_active)
save(failed_file, failed)
save(completed_file, completed)
PY

exit 0
