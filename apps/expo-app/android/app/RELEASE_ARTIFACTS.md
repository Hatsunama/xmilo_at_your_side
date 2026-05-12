# Android Release Artifacts

This file documents generated native artifacts required by the Android app packaging path.

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
