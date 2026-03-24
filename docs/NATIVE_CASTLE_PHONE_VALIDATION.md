# Native Castle Phone Validation

This is the production app-builder validation path for the embedded `castle.aar` renderer.

## What this phase is for

- prove whether the native castle renderer really starts on a physical Android phone
- capture the exact failure class if it does not
- keep the Expo fallback behavior untouched while evidence is gathered

## Current gate

- emulator evidence is useful for smoke checks
- emulator evidence is not enough to close native phone-readiness
- a physical Android phone must be attached over `adb`

## One-command validation

From the repo root:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\android\validate_native_castle.ps1
```

If multiple phones are attached:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\android\validate_native_castle.ps1 -Serial <device-serial>
```

If you intentionally want emulator-only evidence:

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\android\validate_native_castle.ps1 -AllowEmulator
```

## What it captures

- device identity and ABI
- whether the attached target is an emulator
- `am start -W` launch output for `NativeCastleActivity`
- full post-launch `logcat`
- a screenshot
- a summary of whether the native library loaded, layout initialized, and first draw was observed

Artifacts are written to:

```text
C:\xMilo\xmilo_at_your_side\validation-artifacts\native-castle\<timestamp>\
```

## Interpretation

- `outcome: native_renderer_drew`
  - the renderer loaded and produced a confirmed draw on the attached target
- `outcome: native_renderer_started_but_no_confirmed_draw`
  - the native path started but did not provide enough evidence to call the draw path healthy
- `outcome: native_renderer_not_confirmed`
  - the native renderer did not even provide early boot evidence

If `ws_connection_refused` is true, that is a sidecar-availability issue, not automatically a renderer failure. The native renderer can still count as booting if `first_draw` is present.
