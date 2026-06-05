# Testing Grounds Manifest

## Active Purpose

Reconstruct the Phase 19-Memory Testing Grounds scaffold and provide a zero-token dry-run gate for deterministic fixture validation.

## Writable Scope

Only:

```text
C:\xMilo\xmilo_at_your_side\testing-grounds
```

## Non-Writable Scopes

- `apps/expo-app/**`
- `sidecar-go/**`
- `relay-go/**`
- `shared/contracts/**`
- `docs/authority/**`
- `C:\xMilo\Source_Of_Truth_And_Phase_List_Master/**`
- `apps/public-site/**`
- Android Gradle/build files
- package manifests outside Testing Grounds
- scripts outside Testing Grounds

## Current Phase 19-Memory Status

Phase 19M0-M2 scaffold, zero-token dry-run harness, schema/self-test validation, memory fixture expansion, review packet scaffolding, and reviewed schema-only anchors are present. Current dry-run baseline is 25 fixtures PASS, 0 REQUIRES_REVIEW, and 0 FAIL/BLOCKED/SKIPPED. Proof remains schema-only and closure-blocked. This does not prove app, runtime, sidecar, relay, provider, live-model, phone, product, or release readiness. Future work must not treat these anchors as runtime proof.

## Scaffold Components

- `README.md`
- `MANIFEST.md`
- `schemas/fixture.schema.json`
- `schemas/result.schema.json`
- `fixtures/phase19_memory/*.json`
- `runners/dry_run.py`
- `reports/`
- `cache/CACHE_MANIFEST.md`
- `anchors/REVIEWED_ANCHORS.md`
- `docs/README.md`

## Commands To Run

```powershell
python .\testing-grounds\runners\dry_run.py
python .\testing-grounds\runners\dry_run.py --self-test
```

## PASS / FAIL / BLOCKED

- `PASS`: fixture is schema-valid, zero-token compatible, and dry-run expectations are internally coherent.
- `FAIL`: fixture is schema-valid but dry-run validation detects an expectation conflict.
- `BLOCKED`: fixture cannot be evaluated in the current local-only mode because it needs a missing approved capability.
- `REQUIRES_REVIEW`: fixture is schema-valid but needs human review before being used as an anchor or release gate.
- `SKIPPED`: fixture intentionally requires live model or phone proof and zero-token mode is active.

Malformed fixtures or internal runner errors make the runner return nonzero. Expected `SKIPPED` live-model fixtures do not.
