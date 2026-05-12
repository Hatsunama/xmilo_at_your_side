# sidecar-go

Local xMilo sidecar runtime for the app-owned backend terminal/runtime host.

## What it does now

- boots a localhost server on `127.0.0.1:42817`
- persists runtime state in SQLite
- enforces bearer auth
- emits WebSocket events
- starts a simplified task flow against the relay
- loads the Milo system prompt from the authority docs
- reports wake/voice/physical cue capabilities truthfully when the app-owned runtime host does not provide them

## What it does not do yet

- full movement/path registry
- full queue semantics
- trophy flow
- full reset tiers
- full inspector flow
- final xMilo-owned rename pass for any remaining legacy internal binary names
- app-owned runtime-host install/repair proof for public release packaging

## Runtime host truth

The sidecar does not execute external terminal-app commands for wake locks, vibration, or text-to-speech. If the app-owned runtime host does not expose those capabilities, the sidecar emits bounded unsupported/degraded states instead of pretending success or preserving a shell fallback.
