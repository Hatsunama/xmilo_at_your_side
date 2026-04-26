# xMilo Source-of-Truth Pack

Purpose: provide one canonical first-pass lookup location for active source-of-truth files so startup/setup and cross-lane authority do not drift across scattered legacy paths.

Status:
- `LOCKED CANON`: this folder is a repo-local mirror/helper pack for active xMilo authority.
- `LOCKED CANON`: the active canonical authority root is `C:\xMilo\Source_Of_Truth_And_Phase_List_Master\`.
- `LOCKED CANON`: files in this folder must not override the moved canonical root.
- `LOCKED CANON`: legacy file locations remain preserved as mirror/history paths until Main Hub explicitly retires them.

## Folder structure

- `active_core/`
  - mirrored active core authority files used across lanes
- `master/`
  - mirrored master canon / phase-list authority
- `startup/`
  - dedicated startup/setup flow authority files

## First-pass lookup order

1. `C:\xMilo\Source_Of_Truth_And_Phase_List_Master\README.md`
2. `C:\xMilo\Source_Of_Truth_And_Phase_List_Master\XMILO_MASTER_PHASE_LIST_2026-03-24.txt`
3. `C:\xMilo\Source_Of_Truth_And_Phase_List_Master\XMILO_STARTUP_AND_SETUP_FLOW__SOURCE_OF_TRUTH.txt`
4. `source_of_truth/README.md`
5. `source_of_truth/master/XMILO_MASTER_PHASE_LIST_2026-03-24.txt`
6. `source_of_truth/active_core/BOOTSTRAP.md`
7. `source_of_truth/active_core/PACKAGE_MANIFEST.md`
8. `source_of_truth/active_core/INTEGRATION_AUTHORITY.md`

## Conflict rule

- The moved canonical root controls before this folder unless Main Hub explicitly supersedes it.
- Legacy paths outside this folder are preserved as mirrors / historical context and must not silently override the moved canonical version.

## Maintenance rule

- Future startup/setup flow changes must be written to the moved canonical copy first.
- Do not re-scatter startup/setup truth into unrelated phase notes after this pack exists.
- If a mirrored helper copy also needs updating, update the moved canonical file first, then mirror outward deliberately.
