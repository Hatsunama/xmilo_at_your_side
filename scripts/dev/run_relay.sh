#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../../relay-go"
set -a
[ -f .env ] && source .env
set +a
go run ./cmd/milo-relay
