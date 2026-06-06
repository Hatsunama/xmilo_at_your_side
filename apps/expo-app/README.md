# xMilo Expo App

This directory contains the Android-first Expo app surface for xMilo.

xMilo is in active development. This README does not claim release readiness,
commercial readiness, store distribution readiness, hosted access readiness, payment
readiness, or phone-proof completion. Passing local validation is release hygiene
evidence, not proof that deployment, provider access, or commercial launch is
complete.

## App Responsibilities

The Expo app owns React Native UI and user interaction for setup, settings,
Main Hall and Lair interaction, report surfaces, and runtime-host bridge
interaction.

The app does not own durable runtime truth. Native Android runtime state, local
runtime host status, sidecar bridge evidence, and task/runtime proof must come
from the runtime and bridge layers that provide those facts.

## Runtime Host Direction

xMilo uses an app-owned local runtime host and sidecar bridge direction. The app
does not depend on a retired external-terminal bootstrap path or a user-managed
terminal runtime.

`xMiloclaw` may appear in code and native bridge names as the current internal
runtime subsystem/fork naming. Product-facing release truth should describe the
system as xMilo unless a lower-level internal subsystem name is technically
required.

## Access And Providers

BYOK and provider configuration surfaces exist in the app. Hosted access,
payment, account administration, and deployment readiness are separate release
tracks and are not claimed by this README.

Do not treat this directory as proof that hosted access, payments, entitlement
handling, or app-store distribution are complete.

## Native Artifacts

The Android package path depends on generated native artifacts. See
`android/app/RELEASE_ARTIFACTS.md` for the current artifact provenance rules.

Current validation expects generated artifacts such as:

- `android/app/src/main/jniLibs/arm64-v8a/libxmilo_sidecar.so`
- `android/app/libs/castle.aar`

These artifacts are package requirements, not a claim that the app is
commercial-ready.

## Validation Commands

Run these from the repository root:

```powershell
.\scripts\validate_contract_drift.ps1
.\scripts\android\validate_open_source_release.ps1 -AllowDirty -RequireNativeArtifacts
python .\testing-grounds\runners\dry_run.py
python .\testing-grounds\runners\dry_run.py --self-test
```

These checks validate contract drift, open-source release hygiene, required
native artifact presence, and Testing Grounds fixture behavior. They do not
replace device proof, provider proof, hosted access proof, payment proof, or
deployment approval.

## Boundaries

Do not put secrets, local credentials, provider keys, private deployment details,
or private website source details in this README.

This app README is scoped to the Expo app and its Android packaging path. Public
source documentation should stay truthful about what is currently validated and
what remains gated.
