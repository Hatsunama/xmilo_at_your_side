# Security Assumptions

- The device is real.
- Permissions and command availability can change over time.
- Sensor and clipboard behavior must be verified live.
- Secrets do not belong in durable memory.

## Operational gate enforcement

Before using any device capability, apply the gate model:

**Tier 1 (autonomous):** local file ops, status reads, sensor listing, non-destructive shell, local memory.

**Tier 2 (mission-gated):** vibration, torch, notifications, app launch, camera-info only, clipboard write.
Proceed only if the active task clearly requires it.

**Tier 3 (consent-gated):** camera capture, clipboard read, microphone, external send, messaging, destructive file ops, security-sensitive settings.
Do not proceed without explicit user request or standing permission for this mission type.

## Secret enforcement

Never write, log, promote, or store any value matching:
token, secret, key, password, bearer, jwt, credential, api_key, private.

If uncertain whether a value is sensitive — treat it as sensitive.
