# xMiloClaw Skill Registry

Purpose: thin canonical resolver/index for xMiloClaw skills.

Compatibility rules:
- `LOCKED CANON`: this file is the canonical skill registry source of truth for lookup and activation state only.
- `LOCKED CANON`: this file does not replace deeper skill policy/spec files; it resolves approved skill entries against those deeper artifacts.
- `LOCKED CANON`: Main Hub is the registry edit authority for active/canonical entries.

Resolver model:
- `LOCKED CANON`: the registry is index-only and may contain metadata, activation state, validation state, approval state, and implementation references only.
- `LOCKED CANON`: skill implementations live in separate prompt-pack files, declarative skill-spec files interpreted by shipped runtime code, or server-side Go/runtime modules behind approved typed contracts.
- `LOCKED CANON`: Play-unsafe dynamic code loading remains disallowed; runtime resolution may only target approved registered artifacts.
- `LOCKED CANON`: malformed, missing, disabled, oversized, or validation-failed entries must fail closed, not activate, and surface `disabled`, `invalid`, or `review_required` state in the relevant admin/import flow.
- `REJECTED / NOT ADOPTED`: using giant prompt blobs, mixed metadata/logic markdown, or freeform runtime-only registration as the canonical resolver.

Registry entry contract:

| Field | Required meaning |
| --- | --- |
| `skill_id` | Stable canonical skill identifier |
| `display_name` | User/admin-visible skill name |
| `registry_status` | `candidate`, `approved_active`, `approved_disabled`, or `rejected` |
| `implementation_kind` | `prompt_pack`, `declarative_spec`, or `relay_go_module` |
| `implementation_ref` | Canonical file/module/contract reference |
| `source_origin` | `starter_pack`, `converted_openclaw`, `community_import`, or other approved origin |
| `approval_state` | Current approval decision state |
| `validation_state` | Import/validation outcome state |
| `owner_lane` | Lane responsible for the candidate/implementation upkeep |
| `size_class` | Small/medium/large classification or equivalent bounded size label |
| `version` | Skill definition/version identifier |
| `notes` | Compact non-policy notes only |

Import / approval flow:
- `LOCKED CANON`: external or community-added skills enter as `candidate` only.
- `LOCKED CANON`: candidate entries must pass import validation and the basic safety-screening contract before they may be reviewed for activation.
- `LOCKED CANON`: Research Lane and Conversion Lane may contribute candidate skill entries and candidate metadata, but may not canonize active entries without Main Hub approval.
- `LOCKED CANON`: runtime/app/testing lanes may consume or report registry state but may not silently activate candidate skills.

Current population state:
- `PROVISIONAL UNTIL VERIFIED`: this registry file exists as the canonical lookup location, but its initial per-skill population is not complete yet.
- owner: Conversion Lane
- conversion condition: approved starter/candidate skills are entered here with full metadata and implementation references under Main Hub review
