# Build Decisions

This file records current build-facing decisions for the open-source release hygiene pass.

## Current Public Launch Scope

- Source and release delivery are scoped to GitHub, F-Droid packaging work, and website delivery.
- Public build and release docs must not depend on private maintainer paths.
- Public app identity must use xMilo-owned naming and package identity.
- Internal/dev validation may keep separate build settings, but those settings must not be the default public identity.

## Runtime Host Direction

- xMilo uses its app-owned backend terminal/runtime host plus the sidecar runtime.
- Legacy external-terminal bootstrap material is archived/internal-only.
- Public install docs must not present legacy bootstrap as the normal setup path, runtime host, or fallback path.

## Native Artifacts

- Generated native artifacts must either be reproducible from source or explicitly excluded from source truth.
- The Castle AAR remains generated output under `apps/expo-app/android/app/libs/`.
- `apps/expo-app/android/app/RELEASE_ARTIFACTS.md` documents the required rebuild and validation path.

## Release Validation

- `scripts/android/validate_open_source_release.ps1` is the static release-hygiene gate.
- Default validation requires clean git status.
- App packaging validation must use `-RequireNativeArtifacts`.
