# xMilo_v1 — Package Manifest

Purpose: classify canonical authority, bootstrap-safe thin files, on-demand authority layers, and runtime support.

## Canonical authority files

These remain human-readable source-of-truth and must not be replaced by a search/index backend:
- `C:\xMilo\Source_Of_Truth_And_Phase_List_Master\README.md`
- `C:\xMilo\Source_Of_Truth_And_Phase_List_Master\XMILO_STARTUP_AND_SETUP_FLOW__SOURCE_OF_TRUTH.txt`
- `IDENTITY.md`
- `SOUL.md`
- `SECURITY.md`
- `TOOLS.md`
- `USER.md`
- `HEARTBEAT.md`
- `MEMORY.md`
- `AGENTS.md`
- `BOOTSTRAP.md`
- `INTEGRATION_AUTHORITY.md`
- `LANE_REGISTRY.md`
- `SKILL_REGISTRY.md`
- `MAIN_HUB_RESPONSE_CONTRACT.md`
- `PACKAGE_MANIFEST.md`

## Bootstrap-safe thin files

These are eligible for direct startup/bootstrap loading because they are intended to remain small and policy-critical:
- `source_of_truth/README.md`
- `BOOTSTRAP.md`
- `PACKAGE_MANIFEST.md`
- `INTEGRATION_AUTHORITY.md`
- `IDENTITY.md`
- `SOUL.md`
- `SECURITY.md`
- `TOOLS.md`
- `USER.md`
- `HEARTBEAT.md`
- `MEMORY.md`
- `AGENTS.md`
- `LANE_REGISTRY.md`
- `SKILL_REGISTRY.md`
- `MAIN_HUB_RESPONSE_CONTRACT.md`

## On-demand / search-retrieved authority layers

These remain approved authority/supporting layers, but should be retrieved selectively by relevance rather than loaded wholesale by default:
- `source_of_truth/startup/*`
- `memory/tacit/*`
- `memory/knowledge/*`
- `specs/*`
- `xMilo_Sidecar_Go_Responsibility_Checklist.md`
- research/supporting comparison files

## Operational / supporting folders

These are not co-equal canon with bootstrap-safe thin files:
- `memory/` = supporting/on-demand authority layer
- `tasks/` = operational/supporting layer
- `state/` = operational/supporting layer
- `logs/` = operational/supporting layer
- `scripts/` = operational/supporting or retired-evidence layer

## Runtime/config support

- Governed retrieval/search over approved files is runtime/config support, not source-of-truth.
- Backend implementation name is `PROVISIONAL UNTIL VERIFIED`; `QMD` or any equivalent backend is not canon until the current build/runtime confirms the selected implementation.
- Exact backend config path, indexing defaults, and fallback chain are also `PROVISIONAL UNTIL VERIFIED`.
- `MAIN_HUB_RESPONSE_CONTRACT.md` is the single canonical shared response-contract file; active lane directives and the active Testing Grounds authority surface may point to it, but must not fork a duplicate response-contract authority.

## Historical evidence preserved

The following are intentionally preserved as evidence and should not be read as live runtime authority:
- `scripts/*.retired.sh`
- `tasks/*.migrated.json`
- `state/*.migrated.*`
- `tasks/opportunities.retired.json`

## Archived / worktree / backup copies

Archived, backup, cloned, and lane-worktree copies of authority-like files are `ARCHIVED / NOT ACTIVE AUTHORITY` unless Main Hub explicitly selects them for active use.

## Source-of-truth folder lock

- `LOCKED CANON`: `C:\xMilo\Source_Of_Truth_And_Phase_List_Master\` is the first lookup location for active authority.
- `LOCKED CANON`: `docs/authority/xMilo_v1/source_of_truth/` remains a mirror/helper pack only.
- `LOCKED CANON`: future startup/setup flow updates must be written to the moved canonical copy first and mirrored outward deliberately.
- `LOCKED CANON`: legacy authority file locations outside the moved canonical folder remain preserved as mirror/history paths until Main Hub explicitly retires them.

## Packaging rules

- Do not treat retrieval/search infrastructure as independent policy authority.
- Do not load the full authority stack or full memory tree by default.
- Split large authority or memory content into governed subfiles where useful.
- Silent truncation or partial authority load is not acceptable normal behavior.
