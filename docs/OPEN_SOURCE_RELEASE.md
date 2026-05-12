# Open-Source Release Hygiene

This document is the current public release truth for the open-source launch path.

## Launch Path

xMilo is launching open source first through:

- GitHub source and release artifacts
- F-Droid packaging work
- website delivery

This document does not define external store policy work. If older docs conflict with this file for public source/package truth, this file and the repo-local authority mirror are the release-hygiene reference for the current open-source pass.

## Public App Identity

- Public Android application id: `com.hatsunama.xmilo`
- Internal/dev builds may append an internal suffix or override `XMILO_ANDROID_PACKAGE`.
- Public release config must not default to a dev package name.
- Public relay config must not default to a local development relay.
- App version must stay consistent across `apps/expo-app/package.json`, `apps/expo-app/package-lock.json`, `apps/expo-app/app.config.ts`, and `apps/expo-app/android/app/build.gradle`.

## Runtime Host Truth

xMilo's current public runtime direction is the app-owned backend terminal/runtime host plus the sidecar runtime. Legacy external-terminal bootstrap scripts are not the active public setup, backend terminal, runtime host, bootstrap path, fallback path, or required user dependency.

Public docs must not direct normal users to install a separate terminal app as the normal launch path. Any legacy bootstrap notes must stay archived/internal-only and must not be published as current install instructions.

## Authority Root Truth

Public source users should rely on the repo-local authority mirror under `docs/authority/xMilo_v1/` and the packaged bootstrap files copied into the Android app assets.

Private absolute authority roots are development context only. Public source/package instructions must not require paths from a maintainer machine.

## Native Artifact Truth

The Android app currently depends on a generated Castle AAR at `apps/expo-app/android/app/libs/castle.aar`. That `libs/` directory is ignored so generated native binaries are not committed as source.

The required rebuild/verification path is documented in `apps/expo-app/android/app/RELEASE_ARTIFACTS.md`. Release validation must fail if app packaging requires generated native artifacts and they are missing.

## Static Release Gate

Run:

```powershell
.\scripts\android\validate_open_source_release.ps1
```

The default gate requires a clean git status. During active repair work, `-AllowDirty` may be used only to validate all non-clean-tree checks before packaging.

For app packaging, also require generated native artifacts:

```powershell
.\scripts\android\validate_open_source_release.ps1 -RequireNativeArtifacts
```

The gate checks release config identity, version consistency, public docs, legacy local artifacts, workflow drift, native artifact provenance, and scope boundaries for this release-hygiene mission.
