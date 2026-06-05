# xMilo Testing Grounds

Testing Grounds is the validation-only lane for xMilo regression fixtures, dry-run reports, reviewed anchors, and evidence artifacts.

It does not own production code. It must not patch `apps/expo-app`, `sidecar-go`, `relay-go`, shared contracts, authority docs, Gradle/build files, or public-site surfaces unless Main Hub explicitly changes scope.

The default execution mode is zero-token dry run. Local runners must not call live models, networks, production endpoints, phone devices, adb, or app builds by default. Live model evals require explicit Main Hub approval and must use the cache/review-anchor rules in this scaffold.

The relocated Phase 19M0-M2 baseline has 25 memory/governed-learning fixtures passing in the zero-token dry-run harness and 19 reviewed schema-only anchors present. Proof level is schema-only, `closure_allowed=false`, and no live model, production endpoint, phone, app, sidecar, relay, provider, product, release, or full Phase 19 closure proof is claimed.

## Commands

From `C:\xMilo\xmilo_at_your_side`:

```powershell
python .\testing-grounds\runners\dry_run.py
python .\testing-grounds\runners\dry_run.py --self-test
```

The runner discovers JSON fixtures under `fixtures/`, validates required fields, computes prompt/content hashes, and writes `reports/dry_run_latest.json`.
