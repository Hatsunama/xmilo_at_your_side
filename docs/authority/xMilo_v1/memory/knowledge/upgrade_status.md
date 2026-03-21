# Upgrade Status

## Current Canonical Mind Pack
- version: Milo's Mind v4
- merged with: MiloClaw Mind Merge Spec v1
- runtime authority: PicoClaw Go (SQLite, bridge, relay, events)
- policy authority: Mind v4 files (identity, behavior, security, knowledge)

## What changed in v4
- unrelated domain-specific legacy logic removed
- execution-before-reporting hardened
- bounded mission completion rule hardened
- generic assistant identity restored
- shell scripts demoted from runtime authority to maintenance utilities
- Tier 1/2/3 capability gate model added
- secret promotion block added
- conversation tail ownership moved to PicoClaw SQLite
- BOOTSTRAP.md updated with runtime readiness checks
- HEARTBEAT.md updated with policy-first load order
- speak_text_procedure.md updated with on-phone vs app-side speech separation
- reusable_procedures.md updated with consent-gate annotations

## SQLite schema tracking
PicoClaw SQLite uses a schema_version table with ordered forward migrations.
Schema migration is a first-class required feature of the MiloClaw fork.
Current schema version: managed by PicoClaw Go. Not tracked in this file.
