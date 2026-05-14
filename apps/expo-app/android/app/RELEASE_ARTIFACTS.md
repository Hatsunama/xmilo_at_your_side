# Android Release Artifacts

This file documents generated native artifacts required by the Android app packaging path.

## Sidecar Native Payload

Required generated path:

```text
apps/expo-app/android/app/src/main/jniLibs/arm64-v8a/libxmilo_sidecar.so
```

Source path:

```text
sidecar-go/
```

Tracked expected hash:

```text
apps/expo-app/android/app/SIDECAR_NATIVE_PAYLOAD.sha256
```

Build and verification command:

```powershell
.\scripts\android\build_sidecar_native_payload.ps1
```

Check-only rebuild command:

```powershell
.\scripts\android\build_sidecar_native_payload.ps1 -CheckOnly
```

Hash update command, only after intentional sidecar source changes:

```powershell
.\scripts\android\build_sidecar_native_payload.ps1 -UpdateExpectedHash
```

Notes:

- The generated `.so` must stay ignored and untracked.
- Default build-script behavior verifies the generated payload hash before replacing the active ignored payload.
- Check-only mode rebuilds to a temporary path, compares against the tracked expected hash, and does not replace the active ignored payload.
- Updating the expected hash requires the explicit `-UpdateExpectedHash` flag.
- Open-source release validation must fail if the payload is missing, tracked, not ignored, empty, or hash-mismatched.

## Castle AAR

Required path:

```text
apps/expo-app/android/app/libs/castle.aar
```

Source path:

```text
castle-go/
```

Rebuild and verification command:

```powershell
.\scripts\verify-castle-native-artifacts.ps1 -Variant internalDebug -RebuildAar -SkipBuild
```

Full app-package verification command:

```powershell
.\scripts\android\validate_open_source_release.ps1 -RequireNativeArtifacts
```

Notes:

- `apps/expo-app/android/app/libs/` is ignored because the AAR is generated output, not source.
- Public source release truth must not imply that this ignored AAR is committed.
- App packaging must fail validation if this AAR is required and absent.
- Castle renderer polish and visual truth are not changed by this release-hygiene file.
