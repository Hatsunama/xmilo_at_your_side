# xMilo v8

v8 is the first feature-complete alpha of the xMilo monorepo. It adds the full
access lifecycle on top of the v7 skeleton: email verification, launch-stage
access code redemption, RevenueCat webhook handling, proactive JWT refresh, the
setup wizard, in-app AI output reporting, account-management compliance work,
and the hidden relay admin page.

## Stack

| Layer | Where | Description |
|---|---|---|
| `apps/expo-app/` | Android (Expo) | App shell, Wizard Lair UI, WebSocket bridge |
| `sidecar-go/` | On-device (Termux) | xMilo local runtime, SQLite, relay proxy |
| `relay-go/` | Hosted (xmiloatyourside.com) | JWT, LLM, email, entitlements, admin |

## What's real in v8

### Relay (`relay-go/`)
- xAI / Grok provider via OpenAI-compatible endpoint
- Postgres-backed migrations (users, email verifications, access codes,
  entitlement grants, turn logs, error reports, AI content reports)
- Email verification via SMTP — dev mode prints links to stdout
- Entitlement service: launch-stage access codes (single-use, 30 days by
  default in `code_only` mode), later public trial/subscription via config
- Hidden admin page at `/admin` with stats, invite batch creation, error log
- All locked product rules enforced in code, not config
- Account deletion endpoint now clears the current verified account's linked
  sessions, entitlements, reports, and future device-linked turn logs

### Sidecar (`sidecar-go/`)
- Bootstraps JWT from relay on first launch
- Proactive JWT refresh goroutine (wakes every minute, refreshes at <10 min TTL)
- `/auth/register`, `/auth/invite`, `/auth/check` proxy endpoints
- `/auth/check` forces immediate relay refresh → entitled=true within ~1s of
  email verification
- `/report/ai`, sign-out, and account-deletion paths are wired through the relay

### App (`apps/expo-app/`)
- Full setup wizard (`app/setup.tsx`):
  - Detects sidecar health, waits and shows Termux install instructions if needed
  - Can copy the Termux bootstrap command and open direct install destinations
  - Generated Android shell now includes a native Termux bootstrap bridge for post-prerequisite handoff
  - Email collection → verification polling → access choice
  - Access-code-only launch path now shown by default
  - Public trial/subscription path remains preserved behind relay config
- Settings now expose privacy, support, sign-out, and delete-account actions
- User-visible AI outputs now include an in-app report action
- `bridge.ts` auth helpers wired to sidecar endpoints

## What's still missing

- RevenueCat in-app purchase in the app (subscribe stub shown, not wired)
- Final Wizard Lair visual world (deferred)
- Play Store build artifacts and EAS production profile
- Gmail / Google OAuth login
- Final xMilo-owned rename pass for legacy internal runtime names in binaries, release assets, and scripts
- Native automatic Termux bootstrap / repair from the app itself is partially in place, but still depends on the user completing prerequisite F-Droid / Termux / Termux:API setup and granting the Termux run-command permission during walkthrough
- Repository license file still needs an explicit project-owner choice before public release hygiene is complete

## Env variables

See `relay-go/.env.example` for the full list with comments.

Minimum required for production:
```
RELAY_POSTGRES_DSN=postgres://...
RELAY_JWT_SECRET=<openssl rand -hex 32>
XAI_API_KEY=<your xAI key>
SMTP_HOST=<smtp host>
SMTP_USERNAME=<user>
SMTP_PASSWORD=<pass>
RELAY_ADMIN_PASSWORD=<openssl rand -hex 16>
REVENUECAT_WEBHOOK_AUTH=<from RevenueCat dashboard>
RELAY_DEV_ENTITLED=false
```

Sidecar (`sidecar-go/config.example.json`):
```json
{
  "bearer_token": "<must match EXPO_PUBLIC_LOCALHOST_TOKEN>",
  "relay_base_url": "https://xmiloatyourside.com"
}
```

## Safe build order (from INTEGRATION_AUTHORITY.md)

1. Bootstrap, SQLite schema, one-time legacy import ✓
2. HTTP bridge + WebSocket server + event journal ✓
3. JWT / session lifecycle and `/auth/refresh` ✓
4. Entitlement lifecycle (launch code mode now default, later public mode preserved) ✓
5. Setup wizard ✓
6. RevenueCat in-app purchase wiring ← next
7. Wizard Lair visual world ← deferred
8. Play Store artifacts and EAS production profile

## Dev quickstart

```bash
# Relay (needs Postgres)
cd relay-go && cp .env.example .env  # fill in values
go run ./cmd/milo-relay

# Sidecar (local)
cd sidecar-go && cp config.example.json config.json  # fill in values
go run ./cmd/picoclaw

# App
cd apps/expo-app && cp .env.example .env.local  # fill in values
npx expo start --lan --clear
```

## Authority

Policy and product locks live in `docs/authority/xMilo_v1/`.
When code and docs conflict, the v1 authority docs win.

## Release hygiene notes

- User-facing naming must stay xMilo-owned even if some internal binaries still
  use legacy placeholder names during development.
- The Play-facing build must keep privacy, deletion, AI reporting, and
  permission disclosures aligned with the real runtime behavior.
