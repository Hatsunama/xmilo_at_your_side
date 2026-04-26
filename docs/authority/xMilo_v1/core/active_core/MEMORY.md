# MEMORY

This file defines durable long-term memory behavior.

## What belongs in memory
Store:
- durable procedures
- device capability facts
- user preferences that change future behavior
- repeatable fixes
- recovery notes
- important project decisions
- mission-resume state that meaningfully reduces restart confusion

## What does not belong in memory
Do not store:
- raw noise
- long repetitive logs
- temporary emotional fluff
- secrets, tokens, credentials, or keys of any kind
- speculative conclusions
- stale domain baggage once it is known to be irrelevant

## Memory writing rule
Write memory only when the note would change future behavior.

## Memory structure
Memory is layered.

### Core instruction files at root
- stable identity and control law
- not casual scratch space

### memory/tacit/
- rules, preferences, goals, security assumptions
- slower-changing behavioral guidance

### memory/knowledge/
- procedures, device capability profile, runtime knowledge
- explicit reusable knowledge

### memory/YYYY-MM-DD.md
- daily factual operational notes
- short-lived operational trail, not the source of truth for identity

## Resume rule
After each meaningful step in a multi-step mission:
- update the resumable state in xMilo Sidecar SQLite (canonical)
- preserve the next step
- preserve blocker state if any
- do not wait until the end if interruption risk is high

## Conversation tail rule
Conversation tail is NOT stored in flat memory files.
Conversation tail lives in xMilo Sidecar SQLite exclusively.
xMilo Sidecar trims it to the last 12 turns or ~4000 input tokens, whichever is smaller.
The system prompt (IDENTITY + SOUL behavior rules) is always prepended as the first system-role entry on every relay call.

## Secret rule
Secrets, tokens, credentials, API keys, bearer tokens, JWTs, and passwords do not belong in any memory file — ever.
This applies to: direct writes, lesson promotion, nightly consolidation output, task notes, and session reflections.
