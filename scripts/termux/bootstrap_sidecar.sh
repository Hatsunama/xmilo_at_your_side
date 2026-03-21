#!/data/data/com.termux/files/usr/bin/bash
# xMilo sidecar bootstrap — install.sh
# Served at https://xmiloatyourside.com/install.sh
# Usage: curl -fsSL https://xmiloatyourside.com/install.sh | bash
set -euo pipefail

GITHUB_OWNER="xmilo-app"
GITHUB_REPO="xmilo-releases"
BINARY_NAME="picoclaw"
WORKSPACE="${HOME}/.miloclaw"
BIN_DIR="${WORKSPACE}/bin"
LOG_DIR="${WORKSPACE}/logs"
CONFIG_PATH="${WORKSPACE}/config.json"
PID_FILE="${WORKSPACE}/sidecar.pid"
SIDECAR_PORT="42817"
SIDECAR_HOST="127.0.0.1"
HEALTH_URL="http://${SIDECAR_HOST}:${SIDECAR_PORT}/health"
HEALTH_RETRIES=12
HEALTH_WAIT_SEC=2

bold() { printf '\033[1m%s\033[0m\n' "$*"; }
ok()   { printf '\033[32m✓\033[0m  %s\n' "$*"; }
info() { printf '\033[34m→\033[0m  %s\n' "$*"; }
warn() { printf '\033[33m⚠\033[0m  %s\n' "$*"; }
fail() { printf '\033[31m✗\033[0m  %s\n' "$*" >&2; exit 1; }

bold "xMilo sidecar installer"
echo ""

# 1. Termux check
if [ ! -d "/data/data/com.termux" ]; then
  fail "This script must be run inside Termux. Install Termux from F-Droid first."
fi
ok "Termux detected"

# 2. Termux:API check
if ! command -v termux-info >/dev/null 2>&1; then
  warn "Termux:API not found. Install it from F-Droid, then re-run this script."
  warn "Termux:API is required for device features (wake lock, notifications)."
  echo ""
  echo "  Install from F-Droid: search for 'Termux:API'"
  echo "  After installing, open Termux:API once to grant permissions, then run:"
  echo "    curl -fsSL https://xmiloatyourside.com/install.sh | bash"
  exit 1
fi
ok "Termux:API detected"

# 3. Required tools
for tool in curl jq; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    info "Installing missing tool: ${tool}"
    pkg install -y "$tool" >/dev/null 2>&1 || fail "Could not install ${tool}. Run: pkg install ${tool}"
  fi
done
ok "Required tools available"

# 4. CPU architecture
RAW_ARCH="$(uname -m)"
case "$RAW_ARCH" in
  aarch64|arm64)    ARCH="arm64" ;;
  armv7l|armv7|arm) ARCH="arm"   ;;
  x86_64)           ARCH="amd64" ;;
  *) fail "Unsupported CPU architecture: ${RAW_ARCH}. Supported: arm64, arm, amd64." ;;
esac
ok "CPU architecture: ${ARCH} (from ${RAW_ARCH})"

# 5. Fetch latest release tag
info "Checking latest release..."
LATEST_TAG="$(curl -fsSL "https://api.github.com/repos/${GITHUB_OWNER}/${GITHUB_REPO}/releases/latest" | jq -r '.tag_name')"
if [ -z "$LATEST_TAG" ] || [ "$LATEST_TAG" = "null" ]; then
  fail "Could not fetch latest release from GitHub. Check your internet connection."
fi
ok "Latest release: ${LATEST_TAG}"

# 6. Download URLs
ASSET_NAME="${BINARY_NAME}-${ARCH}"
DOWNLOAD_URL="https://github.com/${GITHUB_OWNER}/${GITHUB_REPO}/releases/download/${LATEST_TAG}/${ASSET_NAME}"
CHECKSUM_URL="${DOWNLOAD_URL}.sha256"

# 7. Create workspace
mkdir -p "${BIN_DIR}" "${LOG_DIR}"
ok "Workspace ready: ${WORKSPACE}"

# 8. Download binary
TARGET_BIN="${BIN_DIR}/${BINARY_NAME}"
info "Downloading ${ASSET_NAME}..."
curl -fsSL --progress-bar -o "${TARGET_BIN}.tmp" "${DOWNLOAD_URL}" \
  || fail "Download failed. URL: ${DOWNLOAD_URL}"

# 9. Verify checksum
if curl -fsSL -o "${TARGET_BIN}.sha256" "${CHECKSUM_URL}" 2>/dev/null; then
  EXPECTED="$(awk '{print $1}' "${TARGET_BIN}.sha256")"
  ACTUAL="$(sha256sum "${TARGET_BIN}.tmp" | awk '{print $1}')"
  if [ "$EXPECTED" != "$ACTUAL" ]; then
    rm -f "${TARGET_BIN}.tmp" "${TARGET_BIN}.sha256"
    fail "Checksum mismatch. Expected: ${EXPECTED}, Got: ${ACTUAL}"
  fi
  ok "Checksum verified"
  rm -f "${TARGET_BIN}.sha256"
else
  warn "No checksum file found — skipping verification."
fi

mv "${TARGET_BIN}.tmp" "${TARGET_BIN}"
chmod +x "${TARGET_BIN}"
ok "Binary installed: ${TARGET_BIN}"

# 10. Bearer token
TOKEN_FILE="${WORKSPACE}/bearer_token"
BEARER_TOKEN=""
[ -f "${TOKEN_FILE}" ] && BEARER_TOKEN="$(cat "${TOKEN_FILE}")"
[ -z "$BEARER_TOKEN" ] && [ -n "${XMILO_BEARER_TOKEN:-}" ] && BEARER_TOKEN="${XMILO_BEARER_TOKEN}"

if [ -z "$BEARER_TOKEN" ]; then
  echo ""
  warn "No bearer token found at ${TOKEN_FILE}."
  echo "  The xMilo app writes this file automatically during setup."
  echo "  Return to the xMilo app and follow the setup screen, then re-run:"
  echo "    curl -fsSL https://xmiloatyourside.com/install.sh | bash"
  echo ""
  printf "  Or paste a token now (blank to skip): "
  read -r BEARER_TOKEN
fi

[ -z "$BEARER_TOKEN" ] && BEARER_TOKEN="REPLACE_WITH_TOKEN_FROM_APP" && \
  warn "No token set. Re-run after the xMilo app generates one."

# 11. Relay URL
RELAY_URL_FILE="${WORKSPACE}/relay_url"
RELAY_URL=""
[ -f "${RELAY_URL_FILE}" ] && RELAY_URL="$(cat "${RELAY_URL_FILE}")"
[ -z "$RELAY_URL" ] && [ -n "${XMILO_RELAY_URL:-}" ] && RELAY_URL="${XMILO_RELAY_URL}"
[ -z "$RELAY_URL" ] && RELAY_URL="https://relay.xmiloatyourside.com" && \
  info "Using default relay URL: ${RELAY_URL}"

# 12. Write config
MIND_ROOT="${WORKSPACE}/mind"
mkdir -p "${MIND_ROOT}"

if [ ! -f "${MIND_ROOT}/system_prompt.md" ]; then
  cat > "${MIND_ROOT}/system_prompt.md" << 'PROMPT_EOF'
You are Milo — a focused, direct, and thoughtful AI companion running locally on the user's Android phone. You help the user think through tasks, write, plan, summarize, and reason. You are precise and avoid padding. When you have enough information to answer, answer. When you need to ask, ask one question.
PROMPT_EOF
fi

RUNTIME_ID="xmilo-$(cat /proc/sys/kernel/random/uuid 2>/dev/null | tr -d '-' | head -c 16 || date +%s)"

cat > "${CONFIG_PATH}" << CONFIG_EOF
{
  "host": "${SIDECAR_HOST}",
  "port": ${SIDECAR_PORT},
  "bearer_token": "${BEARER_TOKEN}",
  "relay_base_url": "${RELAY_URL}",
  "db_path": "${WORKSPACE}/xmilo.db",
  "mind_root": "${MIND_ROOT}",
  "runtime_id": "${RUNTIME_ID}"
}
CONFIG_EOF
ok "Config written: ${CONFIG_PATH}"

# 13. Stop existing instance
if [ -f "${PID_FILE}" ]; then
  OLD_PID="$(cat "${PID_FILE}")"
  if kill -0 "${OLD_PID}" 2>/dev/null; then
    info "Stopping existing sidecar (PID ${OLD_PID})..."
    kill "${OLD_PID}" 2>/dev/null || true
    sleep 1
  fi
  rm -f "${PID_FILE}"
fi
pkill -x "${BINARY_NAME}" 2>/dev/null || true
sleep 1

# 14. Start sidecar
info "Starting xMilo sidecar..."
nohup "${TARGET_BIN}" --config "${CONFIG_PATH}" > "${LOG_DIR}/sidecar.log" 2>&1 &
SIDECAR_PID=$!
echo "${SIDECAR_PID}" > "${PID_FILE}"
info "Sidecar started (PID ${SIDECAR_PID})"

# 15. Health check
echo ""
info "Waiting for sidecar to become healthy..."
HEALTHY=false
for i in $(seq 1 "${HEALTH_RETRIES}"); do
  sleep "${HEALTH_WAIT_SEC}"
  HTTP_CODE="$(curl -fsSL -H "Authorization: Bearer ${BEARER_TOKEN}" -o /dev/null -w "%{http_code}" "${HEALTH_URL}" 2>/dev/null || echo "000")"
  if [ "${HTTP_CODE}" = "200" ]; then
    HEALTHY=true
    break
  fi
  printf "  Attempt %d/%d (HTTP %s)...\n" "$i" "${HEALTH_RETRIES}" "${HTTP_CODE}"
done

echo ""
if [ "${HEALTHY}" = "true" ]; then
  ok "Sidecar is healthy at ${HEALTH_URL}"
  echo ""
  bold "xMilo is ready."
  echo ""
  echo "  Return to the xMilo app — it will continue automatically."
  echo ""
  echo "  To view logs:  tail -f ${LOG_DIR}/sidecar.log"
  echo "  To stop:       kill \$(cat ${PID_FILE})"
else
  fail "Sidecar did not become healthy after $((HEALTH_RETRIES * HEALTH_WAIT_SEC)) seconds.
  Check logs: tail -f ${LOG_DIR}/sidecar.log
  Health URL: ${HEALTH_URL}"
fi
