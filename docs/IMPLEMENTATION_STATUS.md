# Implementation Status

This status file is scoped to current open-source release hygiene. Older setup/bootstrap planning notes are archived and are not current public runtime truth.

## Current Release-Hygiene State

- Public app identity defaults to `com.hatsunama.xmilo`.
- App version is aligned across Expo config, package metadata, package lock, and Android `versionName`.
- Public relay defaults no longer point to a local development relay.
- The sidecar release workflow publishes the sidecar binary and checksum only.
- The current runtime host direction is app-owned backend terminal/runtime host plus sidecar.
- Generated Castle AAR provenance is documented in `apps/expo-app/android/app/RELEASE_ARTIFACTS.md`.
- Static release validation lives at `scripts/android/validate_open_source_release.ps1`.

## Still Not Launch-Ready

- The project still needs an explicit license decision before public release packaging.
- Hosted relay privacy/auth hardening remains separate unless hosted access is included in the first release.
- UI/setup/onboarding changes are out of scope for this release-hygiene pass.
- Castle visual/runtime truth remains separate if launch materials promise the native Castle experience.
