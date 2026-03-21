# sidecar-go

Local xMilo sidecar starter for Android / Termux.

## What it does now

- boots a localhost server on `127.0.0.1:42817`
- persists runtime state in SQLite
- enforces bearer auth
- emits WebSocket events
- starts a simplified task flow against the relay
- loads the Milo system prompt from the authority docs

## What it does not do yet

- full movement/path registry
- full queue semantics
- trophy flow
- full reset tiers
- full inspector flow
- polished automated Termux bootstrap / repair flow
