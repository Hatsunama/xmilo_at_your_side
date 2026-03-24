# Implementation Status — xMilo_v7

## Fixed in v7 (minimum diff from v6)

### Blocker 1 — Sidecar relay client timeout: 60s
**File:** `sidecar-go/internal/relay/client.go`
**Root cause:** Sidecar HTTP client had `Timeout: 60 * time.Second`. Relay's xAI client timeout is `10 * time.Minute`. Grok 4 reasoning regularly exceeds 60s. Sidecar fires `context deadline exceeded` before xAI responds → every non-trivial task goes to `stuck`.
**Fix:** `Timeout: 12 * time.Minute` (relay timeout + buffer).

### Blocker 2 — JWT never bootstrapped into sidecar on first launch
**File:** `sidecar-go/internal/http/router.go`
**Root cause:** `getJWT` reads `relay_session_jwt` from SQLite. Fresh install → empty string → Authorization header omitted → relay `/llm/turn` returns 401 → treated as non-retryable 4xx → every task goes to `stuck` immediately. No code path called `POST /session/start` to obtain the initial JWT.
**Fix:** Added `bootstrapRelaySession()` called in `NewApp()` when `relay_session_jwt` is empty. Calls `POST {relayBaseURL}/session/start`, stores the returned JWT and expiry. Soft-fails (logs, continues) if relay is unreachable at boot — sidecar still starts and serves local events.

### Doc fix — TERMUX_QUICKSTART stale v5 paths
**File:** `docs/TERMUX_QUICKSTART.md`
**Fix:** Updated all `xmilo_v5`/`xMilo_v5` references to `xmilo_v6`/`xMilo_v6`. Title made version-agnostic going forward.

---

## Fixed in v6 (minimum diff from v5)

### Bug — relay HTTP client timeout too short for Grok 4 reasoning
**File:** `relay-go/internal/openai/client.go`
**Root cause:** `http.Client{Timeout: 90 * time.Second}`. xAI's docs flag that Grok 4 reasoning models require a longer timeout and use `3600s` in their examples. Any task that causes Grok 4 to reason beyond 90s hits `context deadline exceeded`, which the relay wraps and the sidecar surfaces as `task.stuck`.
**Fix:** `Timeout: 10 * time.Minute`. Raise to 30m if you hit longer reasoning runs.

---

## Fixed in v5 (minimum diff from v4)

### Bug 1 — WebSocket reconnect storm (Critical)
**File:** `apps/expo-app/src/state/AppContext.tsx`
**Root cause:** `pushEvent` was defined inline inside `useMemo`, making it a new reference on every `setEvents` call. `useEffect([pushEvent, setState])` in `index.tsx` treats any new reference as a dependency change, so it fired cleanup (closed the WebSocket) and re-ran `boot()` (re-opened it) on every single received event.
**Fix:** `pushEvent` extracted as `useCallback(fn, [])` above the `useMemo`. `setEvents` from `useState` is always stable, so empty deps is correct.

### Bug 2 — Babel bundle failure at SDK 55
**File:** `apps/expo-app/babel.config.js`
**Root cause:** `expo-router/babel` was merged into `babel-preset-expo` at Expo Router v3 (SDK 50) and is not resolvable at SDK 55. The plugin entry throws `Cannot find module 'expo-router/babel'` at bundle time.
**Fix:** Removed the `plugins` entry.

### Bug 3 — Malformed `go.mod` in relay
**File:** `relay-go/go.mod`
**Root cause:** `github.com/jackc/pgx/v5/pgxpool` is a subdirectory package inside the `pgx/v5` module, not an independent Go module with its own `go.mod`. Listing it as a separate `require` entry causes `go build` to fail.
**Fix:** Removed that line. The `pgx/v5` require entry already provides access to all subpackages including `pgxpool`.


### Bug 4 — xAI/Grok runtime request-body risk
**Files:** `relay-go/.env.example`, `relay-go/internal/config/config.go`, `relay-go/internal/http/router.go`, `relay-go/internal/openai/client.go`, `relay-go/internal/turns/service.go`
**Root cause:** the relay was still defaulting to OpenAI-specific environment names and base URL, which would force the wrong provider target after the project decision moved to xAI/Grok. More importantly, unsupported reasoning-model fields must be kept out of the JSON request body or xAI returns a runtime 400 during `/llm/turn`.
**Fix:** switched the relay defaults to `XAI_API_KEY`, `XAI_BASE_URL=https://api.x.ai/v1`, and `XAI_MODEL=grok-4`; made the responses base URL configurable; and added an explicit request-body sanitizer that strips unsupported reasoning-model fields before send.

### Added
- `docs/TERMUX_QUICKSTART.md` — step-by-step dev loop for building and running the sidecar in Termux on a personal Android phone.

---

This file exists so the package stays honest about what is real now versus what is still missing.

## Included now

### Expo app
- fixed dark theme shell
- intro / setup / main hall / settings / archive starter routes
- localhost bridge client
- WebSocket reconnect backoff scaffold
- starter prompt chips
- basic message rendering
- local archive cache schema in expo-sqlite
- long-press copy support for Milo messages

### Sidecar Go
- localhost HTTP server on `127.0.0.1:42817`
- bearer auth middleware
- WebSocket hub route
- SQLite migrations
- runtime config persistence
- task start / current / interrupt / cancel / state / ready / storage routes
- legacy import scaffold
- simplified task engine with relay call + report flow
- event journal writes
- basic room routing scaffold
- wake-lock attempt on startup for Termux

### Relay Go
- HTTP server
- Postgres migrations
- session bootstrap endpoint
- auth refresh endpoint
- `/llm/turn` endpoint
- JWT issue / verify
- xAI/Grok Responses API client scaffold
- provider-isolated relay boundary

## Critical blockers still missing

### Setup / first-run
- real setup wizard flow remains placeholder UI
- F-Droid / Termux / Termux:API install handoff remains unbuilt
- GitHub Releases + `.sha256` is now the locked sidecar artifact authority in setup copy
- fallback installer script now exists at `scripts/termux/install.sh`
- generated Android shell now includes a native Termux bootstrap bridge that can invoke the installer command after prerequisites and permission are granted
- full automatic app-driven bootstrap / repair is still incomplete because prerequisite-app install and permission walkthrough remain user-facing
- guided repair flow remains unbuilt

### Access / auth / billing
- email verification code flow is not wired yet
- SMTP verification send path is not wired yet
- invite-code generation / redeem / burn path is not wired yet
- trial state enforcement is not wired end-to-end
- RevenueCat is not wired
- Restore Purchases is not wired end-to-end
- `task.entitlement_lost` path is not wired end-to-end

## Later authority updates beyond this older snapshot

- launch now defaults to `access-code only`
- launch-stage access codes currently grant 30 days by default
- relay/app contract can later flip to public mode by config instead of rebuilding setup
- setup now points at GitHub Releases assets/checksums instead of treating `curl install.sh` as the desired long-term contract
- app-side file/image staging and local notification test surfaces now exist in the Expo shell
- relay/session/auth now includes real verified-email claims, TOTP 2FA with recovery codes, trusted-phone verification, and a short-lived website handoff token path
- setup now pauses for 2FA when a verified email already requires this phone to prove itself
- settings now includes starter 2FA management and website handoff launch actions
- sidecar nightly upkeep now runs at 2:00 AM local time, defers if a task is active, checks for new releases, archives the day, and emits ritual-start / deferred / complete events
- app shell now mirrors nightly upkeep with local vibration/notification cues and optional spoken cues when `Speak aloud` is enabled
- settings now includes a QR-based website sign-in transport path in addition to direct website handoff launch
- the GitHub Releases bootstrap chain is code-complete, but `releases/latest/download/install.sh` will remain 404 until a new tagged release is actually published from the updated workflow
- Android native castle packaging is now code-complete enough to produce both `app-debug.apk` and `app-release.apk`
- `expo-splash-screen` is now installed and the earlier Dev Launcher splash crash is fixed
- the remaining Android native-validation focus is the release-style embedded path, because the debug APK is still an Expo development build and can surface Dev Launcher behavior by design

### Sidecar engine
- `/task/choice` remains stubbed
- `/task/resume_queue` remains stubbed
- `/trophy/conjure` remains stubbed
- `/inspector/open` and `/inspector/close` remain stubbed
- one-queued-task semantics are not fully implemented
- relay retry policy (2s / 5s / 10s) is not fully implemented in the engine path
- proactive JWT refresh at T-5 is not implemented
- pending-event journal replay on reconnect is not fully implemented
- inactivity timer / sleep / ambient pathing are not implemented
- segment-by-segment movement is not implemented
- full reset tiers are not implemented
- log rotation is not implemented

### App experience
- Main Hall is still a starter shell, not the final Wizard Lair world view
- settings flow does not yet expose the full locked account/payment/reset surfaces
- archive/trophy inspector is incomplete
- notification tap routing is incomplete
- back-button behavior is not fully locked in code yet
- soft 6000-char warning is not implemented yet

### App-store readiness
- no final bundle IDs / signing config
- no final app icons / splash / store assets
- no hosted privacy policy / terms URLs
- no production `eas.json` profile
- no final OTA scoping policy in config yet

## Priority order from root blocker analysis

1. setup wizard + automatic xMilo install/repair flow
2. email verification + invite/trial/payment access control path
3. relay retry + entitlement-loss handling + queue/resume semantics
4. trophy / inspector / reset stub replacement
5. inactivity timer + sleep / ambient runtime behavior
6. RevenueCat wiring + restore purchases
7. Wizard Lair world view / character UI
8. signing / store assets / store submission requirements

## Sandbox verification note

- `go test ./...` currently passes in both `relay-go/` and `sidecar-go/`.
- Expo app TypeScript currently passes via direct compiler invocation.
- Bash syntax verification for the Termux installer script could not be run here because WSL has no installed Linux distribution in this Windows environment.
