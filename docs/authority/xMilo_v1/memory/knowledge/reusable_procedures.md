# Reusable Procedures

## Camera

**CONSENT GATE: Tier 3**
Camera capture requires explicit user request or standing user-granted permission for the active mission type.
Do not capture without it. Camera metadata checks (termux-camera-info) are Tier 2 / mission-gated.

### Capture front image
```bash
termux-camera-photo -c 0 ~/photo_front.jpg
```

### Capture back image
```bash
termux-camera-photo -c 1 ~/photo_back.jpg
```

### Validation
- confirm output file exists
- confirm file size is non-zero

---

## Clipboard

### Read
**CONSENT GATE: Tier 3**
Clipboard read requires explicit user request.
Clipboard may contain passwords, 2FA codes, banking details, and tokens.
Never read clipboard autonomously.

```bash
termux-clipboard-get
```

### Write
**CONSENT GATE: Tier 2 (mission-gated)**
Clipboard write is allowed only when the task output was clearly intended for clipboard delivery.

```bash
printf 'output_value\n' | termux-clipboard-set
```

### Validation
- only call clipboard broken after set/get was tested together
- never infer clipboard availability from get alone

---

## Torch

**CONSENT GATE: Tier 2 (mission-gated)**
Torch is allowed only when clearly required by the active task.

### Verify command exists
```bash
command -v termux-torch
```

### Test torch
```bash
termux-torch on
sleep 2
termux-torch off
```

---

## Sensor testing

**CONSENT GATE: Tier 1 for listing; Tier 2 for active reads**
Listing sensors is always safe. Active continuous reads should be mission-relevant.

### List sensors
```bash
termux-sensor -l
```

### Single reading
```bash
termux-sensor -s "Exact Sensor Name" -n 1
```

### Validation
- use exact sensor names from the live list
- empty output is not enough to prove absence
