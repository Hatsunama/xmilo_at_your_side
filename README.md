# xMilo

xMilo is an Android-first assistant app and runtime. This repository contains the public app shell, local sidecar runtime, hosted relay service, native Castle renderer source, and repo-local development documentation.

## Status

xMilo is under active development and is not release-ready yet.

Current public repository facts:

- Android package identity defaults to `com.hatsunama.xmilo`.
- App package metadata, Expo config, package lock, and Android `versionName` are aligned at `0.1.5`.
- Public relay defaults are configured for hosted relay use rather than a local development relay.
- The active runtime direction is an app-owned runtime host with the local sidecar runtime.
- Legacy external-terminal bootstrap material is not part of the active public setup path.

## Repository Layout

| Path | Purpose |
|---|---|
| `apps/expo-app/` | Expo/React Native Android app, setup flow, bridge client, native Android packaging, and app-side configuration |
| `sidecar-go/` | Local Go sidecar runtime, task/runtime HTTP surface, local state, and relay forwarding |
| `relay-go/` | Hosted Go relay service for sessions, access checks, LLM relay calls, and report ingestion/admin APIs |
| `castle-go/` | Go source for the native Castle renderer used to generate the Android AAR |
| `docs/authority/xMilo_v1/` | Repo-local reference documentation used by development and packaged runtime workflows |
| `scripts/` | Validation and build-support scripts |
| `shared/` | Shared schemas or cross-package support files where applicable |

## Prerequisites

- Node.js and npm
- Go
- Android Studio and Android SDK
- Expo tooling through `npx expo`
- Fly CLI, only when working on hosted relay deployment

## Development Setup

Install app dependencies:

```powershell
cd apps\expo-app
npm install
```

Run the app development server:

```powershell
cd apps\expo-app
npx expo start --lan --clear
```

Run the sidecar locally:

```powershell
cd sidecar-go
Copy-Item config.example.json config.json
go run ./cmd/xmilo_sidecar
```

Run the relay locally when backend relay work is needed:

```powershell
cd relay-go
Copy-Item .env.example .env
go run ./cmd/milo-relay
```

Use example config files as templates only. Do not commit local `.env`, `config.json`, keys, tokens, or generated private configuration.

## Validation

Run the public release hygiene check:

```powershell
.\scripts\android\validate_open_source_release.ps1
```

Allow intentional local working-tree changes:

```powershell
.\scripts\android\validate_open_source_release.ps1 -AllowDirty
```

Require generated native Android artifacts before packaging:

```powershell
.\scripts\android\validate_open_source_release.ps1 -AllowDirty -RequireNativeArtifacts
```

Useful component checks:

```powershell
cd sidecar-go
go test ./...

cd ..\relay-go
go test ./...

cd ..\apps\expo-app
npx tsc --noEmit
```

## Android and Build Notes

- Public Android builds use `com.hatsunama.xmilo` by default.
- Internal Android build variants may add package suffixes for local testing.
- Castle Android packaging depends on a generated `apps/expo-app/android/app/libs/castle.aar`.
- The generated Castle AAR is not committed as source; rebuild or provide it before Android packaging when native Castle support is required.
- See `docs/OPEN_SOURCE_RELEASE.md` for additional public release hygiene details.

## Configuration and Secrets

- Keep local configuration in ignored environment/config files such as `.env`, `.env.local`, or `config.json`.
- Use checked-in example files only as templates.
- Do not commit API keys, access tokens, bearer tokens, auth headers, provider configs, local database credentials, or private payloads.
- User-supplied provider keys for bring-your-own-key flows should remain on the intended local/user-controlled path and must not be embedded in public source.
- Hosted relay secrets belong in server-side environment storage, not in app bundles or client-side code.

## Security and Privacy

- Treat generated model output as untrusted until reviewed.
- Review diagnostics and report payloads before sharing them.
- Redact secrets from logs, screenshots, repro bundles, and support artifacts.
- Do not rely on client-side checks as the only enforcement for hosted access or privileged backend behavior.
- Report submission and backend review paths should handle only explicit user-submitted reports, not background telemetry.

## Known Limitations

- The repository is not release-ready yet.
- Hosted access hardening is separate from bring-your-own-key local development support.
- Native Castle runtime and visual behavior must be validated before promising the Castle experience in release materials.
- Generated native artifacts may be required for Android packaging but are not committed to the repository.

## License

This repository currently includes an MIT license. See `LICENSE`.
