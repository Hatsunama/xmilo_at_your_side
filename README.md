# xMilo

xMilo is an open-source Android-first assistant runtime. The current launch path is source and release delivery through GitHub, F-Droid packaging work, and website distribution.

This repository is not release-ready yet. The current release-hygiene work is focused on making source/package truth inspectable before feature work resumes.

## Stack

| Layer | Description |
|---|---|
| `apps/expo-app/` | Android app shell, local bridge client, account/access surfaces, and native packaging config |
| `sidecar-go/` | xMilo local runtime, SQLite state, task engine, and relay proxy |
| `relay-go/` | Hosted relay for JWT, LLM access, email, entitlements, and reports |
| `castle-go/` | Native Castle renderer source used to generate the Android AAR |
| `docs/authority/xMilo_v1/` | Repo-local authority mirror used by the packaged runtime bootstrap files |

## Open-Source Release Truth

The public open-source release path is documented in `docs/OPEN_SOURCE_RELEASE.md`.

Current release truths:

- Public Android identity defaults to `com.hatsunama.xmilo`.
- App package, Expo config, package lock, and Android `versionName` are aligned at `0.1.5`.
- Public relay defaults no longer point to a local development relay.
- The app-owned backend terminal/runtime host is the current runtime direction.
- Legacy external-terminal bootstrap material is archived or internal-only, not the active public setup path.
- Castle packaging still requires a generated `castle.aar`; the source and rebuild/verification path are documented, and validation can require the generated artifact before app packaging.
- Private local authority roots are development context only. Public source users should rely on the repo-local authority mirror shipped in this repository.

## Validation

Run the static release-hygiene gate before packaging source or release artifacts:

```powershell
.\scripts\android\validate_open_source_release.ps1
```

For a local working tree with intentional uncommitted edits, use:

```powershell
.\scripts\android\validate_open_source_release.ps1 -AllowDirty
```

For Android app packaging, require generated native artifacts too:

```powershell
.\scripts\android\validate_open_source_release.ps1 -AllowDirty -RequireNativeArtifacts
```

The default gate requires clean git status for release packaging.

## Development Quickstart

```bash
# Relay, if hosted access is part of the local test loop
cd relay-go && cp .env.example .env
go run ./cmd/milo-relay

# Sidecar
cd sidecar-go && cp config.example.json config.json
go run ./cmd/xmilo_sidecar

# App
cd apps/expo-app && cp .env.example .env.local
npx expo start --lan --clear
```

## Known Release Blockers

- A project-owner license decision is still required before public release packaging.
- Hosted access hardening remains separate from BYOK/open-source source hygiene unless hosted access is included in the first release.
- Castle visual/runtime truth remains a separate blocker if launch materials promise the native Castle experience.
- UI/setup/onboarding changes are out of scope for this release-hygiene pass.
