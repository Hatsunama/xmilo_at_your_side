#!/data/data/com.termux/files/usr/bin/bash
# nightly_consolidate.sh
# Maintenance utility only. Not runtime authority.
# Wire into PicoClaw's nightly maintenance scheduler or run via cron.
set -eu

WORKSPACE="${WORKSPACE:-$HOME/.miloclaw/workspace}"
MEMORY_DIR="$WORKSPACE/memory"
KNOWLEDGE_DIR="$MEMORY_DIR/knowledge"
LOG_FILE="$WORKSPACE/logs/recovery.log"

mkdir -p "$KNOWLEDGE_DIR" "$WORKSPACE/logs"

touch "$KNOWLEDGE_DIR/lessons.md" "$KNOWLEDGE_DIR/tools.md" "$KNOWLEDGE_DIR/projects.md"

python3 <<'PY'
import os, datetime

workspace = os.path.expanduser(os.environ.get("WORKSPACE", os.path.expanduser("~/.miloclaw/workspace")))
today_file = f"{workspace}/memory/{datetime.datetime.now().strftime('%Y-%m-%d')}.md"
knowledge_dir = f"{workspace}/memory/knowledge"
lessons_file = f"{knowledge_dir}/lessons.md"
tools_file = f"{knowledge_dir}/tools.md"
projects_file = f"{knowledge_dir}/projects.md"
log_file = f"{workspace}/logs/recovery.log"

# Secret scrub — never promote lines containing sensitive patterns
SECRET_PATTERNS = [
    "token", "secret", "key", "password", "bearer",
    "jwt", "credential", "api_key", "auth", "private"
]

def is_safe_to_promote(line):
    lower = line.lower()
    return not any(pattern in lower for pattern in SECRET_PATTERNS)

def append_unique(path, header, items):
    if not items:
        return
    try:
        with open(path, "r", encoding="utf-8") as f:
            existing = f.read()
    except Exception:
        existing = ""
    new_items = [item for item in items if item and item not in existing]
    if not new_items:
        return
    with open(path, "a", encoding="utf-8") as f:
        f.write(f"\n## {header} - {datetime.datetime.now(datetime.timezone.utc).replace(microsecond=0).isoformat().replace('+00:00','Z')}\n")
        for item in new_items:
            f.write(f"- {item}\n")

try:
    with open(today_file, "r", encoding="utf-8") as f:
        text = f.read()
except Exception:
    text = ""

lines = [line.strip().lstrip("- ").strip() for line in text.splitlines() if line.strip()]

lessons = []
tools = []
projects = []

for line in lines:
    lower = line.lower()
    if any(k in lower for k in ["learned", "lesson", "worked", "failed", "fix", "issue", "warning", "recovery"]):
        lessons.append(line)
    if any(k in lower for k in ["tool", "script", "termux", "api", "json", "bridge", "runtime"]):
        tools.append(line)
    if any(k in lower for k in ["project", "mission", "task", "workflow", "build"]):
        projects.append(line)

# Apply secret scrub before promotion
lessons = [l for l in lessons if is_safe_to_promote(l)]
tools = [t for t in tools if is_safe_to_promote(t)]
projects = [p for p in projects if is_safe_to_promote(p)]

append_unique(lessons_file, "Lessons", lessons)
append_unique(tools_file, "Tools and Systems", tools)
append_unique(projects_file, "Projects and Workflows", projects)

with open(log_file, "a", encoding="utf-8") as log:
    log.write(f"{datetime.datetime.now(datetime.timezone.utc).replace(microsecond=0).isoformat().replace('+00:00','Z')} Nightly consolidation completed.\n")

PY

# Log rotation — keep logs directory under control
# Delete log files older than 30 days
find "$WORKSPACE/logs" -name "*.log" -mtime +30 -delete 2>/dev/null || true

exit 0
