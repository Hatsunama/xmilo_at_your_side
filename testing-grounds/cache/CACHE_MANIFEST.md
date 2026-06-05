# Cache Manifest

## Purpose

The prompt-hash cache prevents unnecessary live model spend and makes Testing Grounds reports repeatable.

## Cache Key Inputs

Cache keys are derived from fixture-declared `cache_key_inputs`. For the dry runner, the prompt/content hash is computed from stable fixture fields and does not call a model.

Recommended cache input fields:

- `fixture_id`
- `input_prompt`
- `untrusted_content`
- `memory_state_before`
- `expected_result`
- `forbidden_results`
- `token_policy`

## No-Live-Token Default

Zero-token dry run is the default. A cache miss must not trigger a live model eval by default.

## Cache Hit Behavior

When a cache entry exists and matches the fixture hash, a future approved runner may reuse the cached result for report regeneration.

## Cache Miss Behavior

A cache miss in zero-token mode remains local-only. Fixtures that require live model execution are `SKIPPED` or `BLOCKED`, not executed.

## Live Eval Approval Requirement

Live evals require explicit Main Hub approval. Approval must identify the fixture set, model/provider, budget, and whether reviewed anchors are required.

## Report Regeneration Rule

Report regeneration must not rerun live evals by default. It should reuse cached evidence or generate a zero-token dry-run report.

