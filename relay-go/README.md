# relay-go

Hosted relay starter for xMilo.

## What it does now

- boots an HTTP API
- creates / refreshes session JWTs
- stores device users and sessions in Postgres
- accepts `/llm/turn`
- calls xAI/Grok's Responses API over HTTPS when configured

## Intentional constraints

- provider abstraction is kept at the relay boundary
- the Android app and sidecar should never talk to the model provider directly
- full rich user-account auth is deferred
- email verification, invite access, and billing are planned as relay-owned flows
- RevenueCat production integration is still deferred
