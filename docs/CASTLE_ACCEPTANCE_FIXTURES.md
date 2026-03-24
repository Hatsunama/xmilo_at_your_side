# Castle Acceptance Fixtures

The first embedded handoff for `castle-go` is validated against a locked, deterministic acceptance suite.

Current acceptance authority:
- `main_hall_arrival`
- `lair_work_cycle`
- `nightly_ritual_cycle`

Why this suite is locked first:
- it covers the first shipped renderer scope only
- it exercises room presence, task-state expression, and nightly ritual truth
- it keeps transition tuning deterministic before Android runtime noise enters the loop

## What each fixture proves

### `main_hall_arrival`

Checks:
- Main Hall reads clearly as the home surface
- report-return movement intent is legible
- settled idle state is readable after return

### `lair_work_cycle`

Checks:
- departure from Main Hall is readable
- the working chamber reads as an active lair surface
- working-state emphasis remains truthful and lightweight

### `nightly_ritual_cycle`

Checks:
- deferred, started, and completed upkeep states remain distinct
- ritual visuals key off maintenance truth rather than invented scheduler state
- the ritual remains lightweight enough for conservative launch budgets

## Current workflow

Render the full acceptance suite:

```powershell
go run ./tools/render-fixture -fixture acceptance -outdir fixture-previews
```

Render one fixture only:

```powershell
go run ./tools/render-fixture -fixture main_hall_arrival -outdir fixture-previews
```

This suite remains the first art-truth authority until the native embedded handoff is validated.
