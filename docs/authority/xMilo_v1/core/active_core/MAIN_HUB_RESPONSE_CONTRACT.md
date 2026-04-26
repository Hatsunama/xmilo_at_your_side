# Main Hub Response Contract

Purpose: standardize lane response shape and handoff behavior under already-locked xMilo canon.

## Compatibility / non-override rule

- `LOCKED CANON`: this file is a response-shape contract only.
- `LOCKED CANON`: this file does not silently replace existing lane directives.
- `LOCKED CANON`: this file does not redefine lane ownership, writable scope, phase order, or source-of-truth precedence.
- `LOCKED CANON`: this file does not create a new shadow authority layer.
- `LOCKED CANON`: this file only standardizes response format and handoff behavior under already-locked canon.
- `LOCKED CANON`: if an existing lane directive or Main Hub assignment defines a stricter output requirement, the stricter requirement still wins unless Main Hub explicitly supersedes it.

## Core response policy

- `LOCKED CANON`: lanes should not send routine progress chatter by default.
- `LOCKED CANON`: lanes should respond when:
  - Main Hub assigns work
  - Main Hub requests proof
  - the lane has completed the assigned return artifact
  - the lane is truly blocked
  - the lane needs a specific canon-required proof artifact from another lane
- `LOCKED CANON`: no "just checking in" update is required unless Main Hub explicitly requests interim updates.
- `LOCKED CANON`: lane response shape must match the assignment type; do not improvise a different reply style when a canonical contract shape exists.
- `LOCKED CANON`: no false completion is allowed; if required proof is missing, completion may not be implied.
- `LOCKED CANON`: partial work must be labeled as partial or blocked rather than framed as closure.

## Blocker rule

- `LOCKED CANON`: `BLOCKED` may be used only when continuation is impossible without a missing prerequisite, missing proof artifact, missing canon decision, missing environment access, or out-of-scope dependency.
- `LOCKED CANON`: a blocked response must name the single smallest unblocker whenever possible.
- `LOCKED CANON`: do not say only "waiting on another lane"; name the exact artifact, decision, or access needed.
- `LOCKED CANON`: blocker language must stay narrow, truthful, and execution-relevant.

## Cross-lane handoff rule

- `LOCKED CANON`: if Lane A needs proof from Lane B, Lane A must name the exact artifact needed, not only the lane name.
- `LOCKED CANON`: if Lane B is asked to provide proof for another lane, Lane B must return a reusable handoff packet rather than freeform explanation.
- `LOCKED CANON`: if build identity matters, the handoff packet must include the exact identity tuple and a `DO_NOT_PROCEED_UNLESS` gate.
- `LOCKED CANON`: if literal strings, logs, endpoints, status routes, file paths, commands, or bundle paths are required for continuation, they must be returned explicitly in the handoff packet.
- `LOCKED CANON`: Main Hub should not need to re-translate a compliant handoff packet into another format before the receiving lane can use it.

## No-over-updating rule

- `LOCKED CANON`: lanes should not post intermediate progress updates unless:
  - Main Hub explicitly requested interim updates
  - a newly discovered true blocker exists
  - a material canon conflict exists
  - a safety or release-risk issue must be escalated immediately
- `LOCKED CANON`: default behavior is do the assigned work, then return only the required contract-shaped output.

## Canonical response shapes

### A. Assignment accepted

Use when Main Hub assigns work and the lane can proceed without a blocker.

Required fields:
- `LANE`
- `TASK_ID`
- `STATUS: ACCEPTED`
- `WILL_RETURN`
- `BLOCKERS: NONE` or the immediate blocker if one is discovered at intake

### B. Completion / return package

Use when the lane has completed the exact assigned return.

Required fields:
- `LANE`
- `TASK_ID`
- `STATUS: COMPLETE`
- `RETURNED_ARTIFACTS`
- `EVIDENCE`
- `DECISIVE_RESULT`
- `NOTES` only if needed and kept short

### C. Blocked

Use only when continuation is impossible without something external.

Required fields:
- `LANE`
- `TASK_ID`
- `STATUS: BLOCKED`
- `BLOCKER_TYPE`
- `EXACT_MISSING_ITEM`
- `WHY_BLOCKED`
- `NEXT_SMALLEST_UNBLOCKER`
- `OWNER_OF_UNBLOCKER` if known

### D. Handoff packet

Use when one lane must give another lane proof or an execution-ready handoff.

Required fields:
- `FROM_LANE`
- `TO_LANE`
- `TASK_ID`
- `HANDOFF_TYPE`
- `EXACT_ARTIFACTS`
- `USE_THIS_TO_CONTINUE`
- `DO_NOT_PROCEED_UNLESS` if identity, build, or proof gating is required

### E. Canon / policy question to Hub

Use when the lane cannot safely continue because canon is not explicit enough.

Required fields:
- `LANE`
- `TASK_ID`
- `STATUS: NEEDS_HUB_DECISION`
- `QUESTION`
- `WHY_EXISTING_CANON_IS_INSUFFICIENT`
- `SMALLEST_DECISION_NEEDED`

### F. Closure validation

Use when a lane is validating whether a gate, phase, or item can close.

Required fields:
- `LANE`
- `TASK_ID`
- `STATUS: PASS` or `FAIL`
- `VALIDATED_AGAINST`
- `REQUIRED_PROOF_CHECKED`
- `SMALLEST_FAIL_SEAM` if `FAIL`

## Scope boundary reminder

- `LOCKED CANON`: this contract standardizes lane response shape; it does not widen execution authority.
- `LOCKED CANON`: ownership, writable scope, source-of-truth precedence, and phase order remain governed by Main Hub canon and lane-specific directives.
