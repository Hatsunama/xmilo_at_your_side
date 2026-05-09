# Speak Text Procedure

## Current speech authority

xMilo Phase 9 uses the app-owned runtime and app UI speech path.
Speech must not depend on retired external tooling or externally installed wrappers.

## App UI speech

The Wizard Lair app owns Milo's app-visible voice output.
If speech is available, it must be exposed through the app-owned UI/runtime contract.

## Runtime speech

The native sidecar must not assume a shell speech command exists.
If the sidecar needs speech and no approved app-owned speech capability is exposed, it must report `speech_capability_unavailable` instead of attempting an external fallback path.

## Procedure

1. Check whether the current app-owned runtime exposes a speech capability.
2. Use the approved app-owned speech path only if present.
3. If unavailable, report the missing capability truthfully.
4. Log a durable lesson only if it changes future behavior.

## Speech must not derail the mission loop

Use background speech only when the app-owned capability supports it.
Do not block execution to wait for speech completion unless the output is critical.
