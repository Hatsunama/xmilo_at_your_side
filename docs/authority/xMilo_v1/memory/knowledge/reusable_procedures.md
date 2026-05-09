# Reusable Procedures

These procedures describe current xMilo-owned runtime expectations only.
They must not route work through retired external tooling.

## Device capability requests

Device capabilities are allowed only through approved xMilo app-owned capabilities or typed runtime contracts.
If a capability is not exposed by the current app-owned runtime, Milo must report the missing capability as a runtime blocker instead of suggesting an external workaround.

## Camera

**CONSENT GATE: Tier 3**
Camera capture requires explicit user request or standing user-granted permission for the active mission type.
Do not capture without it.

### Capture image
- Use only an approved xMilo app-owned camera capability.
- If no approved capability is available, report `camera_capability_unavailable`.

### Validation
- confirm the app-owned capability returned a successful result
- confirm the output reference is present and task-scoped
- do not infer camera capability from authority text alone

---

## Clipboard

### Read
**CONSENT GATE: Tier 3**
Clipboard read requires explicit user request.
Clipboard may contain passwords, 2FA codes, banking details, and tokens.
Never read clipboard autonomously.

- Use only an approved xMilo app-owned clipboard-read capability.
- If no approved capability is available, report `clipboard_read_unavailable`.

### Write
**CONSENT GATE: Tier 2 (mission-gated)**
Clipboard write is allowed only when the task output was clearly intended for clipboard delivery.

- Use only an approved xMilo app-owned clipboard-write capability.
- If no approved capability is available, report `clipboard_write_unavailable`.

### Validation
- only report clipboard success after the app-owned capability confirms it
- never infer clipboard availability from authority text alone

---

## Torch

**CONSENT GATE: Tier 2 (mission-gated)**
Torch is allowed only when clearly required by the active task.

- Use only an approved xMilo app-owned torch capability.
- If no approved capability is available, report `torch_capability_unavailable`.

---

## Sensor testing

**CONSENT GATE: Tier 1 for listing; Tier 2 for active reads**
Listing sensors is low-risk, but active reads should be mission-relevant.

- Use only approved xMilo app-owned sensor capabilities.
- If no approved capability is available, report `sensor_capability_unavailable`.

### Validation
- use exact sensor names returned by the live app-owned capability
- empty output is not enough to prove absence
