# Termux Quickstart — xMilo (version-agnostic)

Minimum steps to get the sidecar running on your Android phone for Expo Go testing.
This is the personal-device dev loop, not the final setup wizard flow.

---

## Prerequisites

- Termux from F-Droid (not Play Store — the Play Store build is abandoned)
- Termux:API from F-Droid
- Go installed in Termux: `pkg install golang`
- Git (optional but useful): `pkg install git`

---

## Step 1 — Copy the sidecar source into Termux

Option A — download the zip from your computer over USB/local network and unzip in Termux:

```bash
mkdir -p ~/xmilo_v6 && cd ~/xmilo_v6
# copy xMilo_v6.zip to ~/storage/shared/Download/ on the phone first
cp ~/storage/shared/Download/xMilo_v6.zip .
unzip xMilo_v6.zip
cd xMilo_v6/sidecar-go
```

Option B — copy via `adb push`:

```bash
adb push xMilo_v6/ /sdcard/Download/xMilo_v6
```

Then in Termux:

```bash
cp -r ~/storage/shared/Download/xMilo_v6 ~/xmilo_v6
cd ~/xmilo_v6/sidecar-go
```

---

## Step 2 — Generate a localhost token

Pick any random string. This value must match in both the sidecar env and your Expo `.env`.

```bash
TOKEN=$(head -c 32 /dev/urandom | base64 | tr -dc 'a-zA-Z0-9' | head -c 40)
echo "Your token: $TOKEN"
# Copy this value — you will need it in Step 3 and Step 4.
```

---

## Step 3 — Configure the sidecar

```bash
cd ~/xmilo_v6/sidecar-go

export PICOCLAW_BEARER_TOKEN="<your token from Step 2>"
export PICOCLAW_HOST="127.0.0.1"
export PICOCLAW_PORT="42817"
export PICOCLAW_DB_PATH="$HOME/.miloclaw/picoclaw.sqlite"
export PICOCLAW_MIND_ROOT="$HOME/xmilo_v6/xMilo_v6/docs/authority/xMilo_v1"
export PICOCLAW_RELAY_BASE_URL="http://127.0.0.1:8080"
```

For a relay you actually have running (even localhost), set `PICOCLAW_RELAY_BASE_URL` to its real address.
If no relay is running, tasks will enter the `stuck` state with a relay error — the sidecar itself will still start and the bridge/WebSocket will work.

---

## Step 4 — Build and run the sidecar

```bash
cd ~/xmilo_v6/sidecar-go

# First run: download deps (requires internet)
go mod download

# Build
go build -o ~/bin/picoclaw ./cmd/picoclaw

# Run
mkdir -p ~/.miloclaw
~/bin/picoclaw
```

Expected output:

```
2026/... load config: ok
2026/... starting picoclaw-sidecar on 127.0.0.1:42817
```

Verify it is alive:

```bash
curl -s -H "Authorization: Bearer <your token>" http://127.0.0.1:42817/health
# → {"ok":true,"service":"picoclaw-sidecar",...}
```

---

## Step 5 — Set the Expo app env

In `apps/expo-app/.env` (copy from `.env.example` first):

```
EXPO_PUBLIC_SIDECAR_BASE_URL=http://127.0.0.1:42817
EXPO_PUBLIC_LOCALHOST_TOKEN=<your token from Step 2>
EXPO_PUBLIC_RELAY_BASE_URL=http://127.0.0.1:8080
XMILO_ANDROID_PACKAGE=com.hatsunama.xmilo.dev
```

---

## Step 6 — Start the Expo app

On your computer (in `apps/expo-app/`):

```bash
npm install
npx expo start --lan --clear
```

Open in Expo Go on the same phone. The Bridge pill in Main Hall should show `picoclaw-sidecar`.

---

## Keeping the sidecar alive

Termux will kill background processes when the screen is off by default.
Acquire a wake lock before running:

```bash
termux-wake-lock
~/bin/picoclaw
```

Or run in a Termux session you keep open. The sidecar also calls `termux-wake-lock` on startup automatically — but the Termux:API app must be installed for that to work.

---

## Relay for dev

The relay requires Postgres. Fastest local option for dev:

1. Skip it entirely — the sidecar handles all non-LLM events without the relay. Tasks will fail with a relay error but the WebSocket, state, and archive all work.
2. Run Postgres + relay on your computer and point `PICOCLAW_RELAY_BASE_URL` at your computer's LAN IP (e.g. `http://192.168.x.x:8080`).

The relay itself is in `relay-go/`. Its required env vars are in `relay-go/.env.example`.

---

## Rebuild after code changes

```bash
cd ~/xmilo_v6/sidecar-go
go build -o ~/bin/picoclaw ./cmd/picoclaw && ~/bin/picoclaw
```

No need to re-run `go mod download` unless `go.mod` changed.
