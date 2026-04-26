# xMilo Lane Registry

Purpose: thin canonical lane lookup layer for fast routing.

Compatibility rules:
- `LOCKED CANON`: this file is a lookup/index layer only, not a rewrite of deeper lane directives.
- `LOCKED CANON`: deeper source-of-truth files still control scope detail, ownership meaning, and supersession decisions.
- `LOCKED CANON`: Main Hub is the only registry edit authority for lane metadata canonization.
- `LOCKED CANON`: if registry lookup is still ambiguous, escalate to Main Hub rather than guessing from task wording.

Current lane registry:

| Lane | Official lane name | Role | Writable scope | Non-owned areas | Status | Registry edit authority |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | Main Hub | Source-of-truth wording, blocker arbitration, cross-lane integration | Master/source authority files and hub-owned authority docs | All non-hub implementation scopes unless explicitly reassigned | active | Main Hub |
| 2 | Website Build Lane | Website/account/support/bootstrap endpoint ownership | `C:\\xMilo\\xmilo_at_your_side\\__website_access_code_system` and hosted bootstrap endpoint content when explicitly assigned | App, runtime, art, testing, registry canon | parked except bootstrap maintenance | Main Hub |
| 3 | Art Build Lane | Castle visuals and screenshot proof | `castle-go/` | App shell, runtime, testing, skill/runtime authority | active | Main Hub |
| 4 | Testing Grounds | Eval harnesses, fixtures, batteries, dated artifacts | `C:\\xMilo\\xmilo_at_your_side\\legacy_from_xMilo_v14_fix\\xmilo-testing-grounds` | Production/runtime/app code unless explicitly assigned for test instrumentation | active | Main Hub |
| 5 | Mind of X Milo | Runtime contracts, sidecar/relay behavior, auth/entitlement refresh, continuity | `sidecar-go/`, `relay-go/` | App shell, website, art, hub authority docs except explicit assignment | active | Main Hub |
| 6 | App Build Lane | App-shell UX truth, setup/recovery flows, on-device rendering | `apps/expo-app/`, `scripts/android/` when explicitly assigned | Runtime, website, art, hub authority docs except explicit assignment | active | Main Hub |
| 7 | UX Research Lane | Research findings and implementation planning only | Research docs/artifacts only when explicitly assigned; no speculative production code changes | Product/runtime/app writable scopes | research-only | Main Hub |
| 8 | Research Lane | Cross-product research, upstream capability review, candidate research synthesis | `C:\xMilo\xmilo_at_your_side\docs\authority\xMilo_v1\research\` when explicitly assigned | Product/runtime/app writable scopes and final canon wording | research-only | Main Hub |
| 9 | Conversion Lane | xMiloClaw conversion pipeline and candidate skill transformations | `C:\\xMilo\\xmilo_at_your_side\\legacy_from_xMilo_v14_fix\\xmilo-skill-conversion` and candidate conversion artifacts when explicitly assigned | App/runtime ownership, active registry canon, hub authority docs | active | Main Hub |
| 10 | Release Signing and Play Distribution Lane | Release signing, packaging, Play submission readiness | No writable scope locked yet | All scopes until Main Hub explicitly activates and assigns writable boundaries | provisional until verified | Main Hub |

Conflict / naming lock:
- `REJECTED / NOT ADOPTED`: renaming `Lane 7` to `Design Lane` at this time.
- `LOCKED CANON`: `Lane 7` remains `UX Research Lane` unless Main Hub explicitly supersedes that name.

Future-lane procedure:
- `LOCKED CANON`: new lanes may be added only by explicit Main Hub decision.
- `LOCKED CANON`: every new lane must receive a unique lane number, official lane name, role, writable scope, non-owned areas, status, and registry edit authority.
- `LOCKED CANON`: this file must be updated in the same change set that creates or canonizes a new lane.
- `LOCKED CANON`: if a new lane overlaps an existing lane's writable scope, Main Hub must resolve the boundary before the new lane becomes active.
- `LOCKED CANON`: no lane is canonically active until it appears in this file or Main Hub explicitly marks it provisional.
