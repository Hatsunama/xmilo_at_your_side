# relay-go

Hosted relay starter for xMilo.

## What it does now

- boots an HTTP API
- creates / refreshes session JWTs
- stores device users and sessions in Postgres
- accepts `/llm/turn`
- calls xAI/Grok's Responses API over HTTPS when configured
- stores in-app AI output reports with moderation status
- exposes account-deletion and sign-out paths for the app-side compliance flow
- exposes an admin review surface for invite codes, error reports, and AI report triage

## Intentional constraints

- provider abstraction is kept at the relay boundary
- the Android app and sidecar should never talk to the model provider directly
- full rich user-account auth is deferred
- email verification, access-code redemption, and launch gating are relay-owned flows
- launch defaults to `XMILO_ACCESS_MODE=code_only`
- public trial/subscription paths are preserved behind config for later activation
- Google OAuth and Play-billing identity linkage still require external platform setup before that lane is complete
