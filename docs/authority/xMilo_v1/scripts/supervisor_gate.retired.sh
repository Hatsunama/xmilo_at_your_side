#!/data/data/com.termux/files/usr/bin/bash
set -eu

WORKSPACE="${WORKSPACE:-$HOME/.openclaw/workspace}"
STATE_FILE="$WORKSPACE/state/heartbeat_state.json"

if [ ! -f "$STATE_FILE" ]; then
  exit 1
fi

STATUS="$(python3 - <<'PY'
import json, os
path = os.path.expanduser(os.environ.get("WORKSPACE", "~/.openclaw/workspace")) + "/state/heartbeat_state.json"
try:
    with open(path, "r", encoding="utf-8") as f:
        data = json.load(f)
        print(data.get("status", ""))
except Exception:
    print("")
PY
)"

if [ "$STATUS" = "healthy" ] || [ "$STATUS" = "recovery_needed" ]; then
  exit 0
fi

exit 1
