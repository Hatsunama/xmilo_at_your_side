#!/data/data/com.termux/files/usr/bin/bash
# daily_rollover.sh
# Maintenance utility only. Not runtime authority.
# Creates today's daily log file if it doesn't exist.
set -eu

WORKSPACE="${WORKSPACE:-$HOME/.xMilo/workspace}"
MEMORY_DIR="$WORKSPACE/memory"

mkdir -p "$MEMORY_DIR"

TODAY_FILE="$MEMORY_DIR/$(date +%F).md"

if [ ! -f "$TODAY_FILE" ]; then
cat <<'DOC' > "$TODAY_FILE"
# Session Reflection

## Projects

## Tasks

## Notes

## Errors

## Decisions
DOC
fi

exit 0
