#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../sidecar-go"
export PICOCLAW_CONFIG="${PICOCLAW_CONFIG:-./config.example.json}"
go run ./cmd/picoclaw
