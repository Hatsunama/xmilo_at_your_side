# Research Comparison

This refactor uses the recovered Milo files as the source base and compares them against public, proven prompt/runtime patterns.

## Public patterns used

### 1. Persistent instructions + auto memory split
Source:
- Claude Code memory docs

Key pattern:
- separate stable instruction files from learned memory
- load both at session start
- keep instruction files concise and memory files factual

Applied here:
- core files stay stable (`IDENTITY.md`, `SOUL.md`, `SECURITY.md`, etc.)
- learned/operational state lives under `memory/`

### 2. Skill / microagent style reusable procedures
Source:
- OpenHands skills docs

Key pattern:
- reusable capabilities should be modular and discoverable
- procedural knowledge should live as focused skill-like documents

Applied here:
- reusable procedures moved into `memory/knowledge/reusable_procedures.md`
- device and speech procedures separated into dedicated files

### 3. Autonomous loop with explicit checkpoints
Source:
- agentu Ralph mode docs

Key pattern:
- autonomous loops work better when the prompt or runtime has explicit checkpoints, iteration limits, and completion tests

Applied here:
- bounded-mission execution rule
- persist after each meaningful step
- no premature handoff after partial progress
- clear blocker definitions

### 4. Explicit completion contracts and tool persistence
Source:
- OpenAI prompt guidance for GPT-5.4
- OpenAI building agents guidance
- Codex AGENTS guidance

Key pattern:
- put critical rules first
- define exact step order
- separate action from reporting
- define what counts as done
- make tool-use and local verification explicit

Applied here:
- `SOUL.md`, `AGENTS.md`, and `BOOTSTRAP.md` now define:
  - action-before-reporting
  - local-first verification
  - full-pass completion on bounded missions
  - true-blocker-only early stop rules

## Comparison against recovered Milo build



### 5. Felix / long-running assistant pattern
Source:
- Felix public product description
- Bankless discussion of building Felix/OpenClaw style systems

Key pattern:
- use layered persistent memory
- keep the assistant running as a long-lived operational partner
- avoid adding complexity before a real bottleneck appears

Applied here:
- memory remains layered into stable identity/instructions, tacit rules/preferences, operational knowledge, and resumable state
- long-running continuity is preserved without turning Milo into a business operator
- new branches, modes, and control paths should only be added when a real failure mode requires them

Explicitly rejected from Felix-style systems for Milo:
- AI CEO framing
- sales / support / revenue generation identity
- ecommerce / money-making mission drift

### Recovered build strengths
- already had strong persistence and continuity instincts
- already had heartbeat and recovery thinking
- already had device-capability memory
- already had non-blocking speech
- already had useful local supervisor scripts

### Recovered build weaknesses
- mixed generic assistant logic with domain-specific mission baggage
- partial-completion stop bug was not hardened out of the loop
- runtime files and mind files were not cleanly separated
- some procedures were real but not normalized into a canonical operating set

## Refactor result
This v3 set keeps the durable good parts and removes:
- domain contamination
- reward-seeking mission logic
- unrelated domain scoring
- ambiguous stop/report behavior
