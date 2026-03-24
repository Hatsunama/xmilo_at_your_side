# Castle Renderer Budgets

These are the locked conservative launch budgets for the first production `castle-go` pass.

Intent:
- prioritize mid-range Android phones
- prefer the Expo fallback over a slow native boot
- fail early and honestly if the native renderer is unavailable or unhealthy
- keep nightly ritual visuals lightweight during background-sensitive periods

## Locked launch envelope

- Health check deadline: `180ms`
- Fallback decision deadline: `220ms`
- Cold-start ready budget: `450ms`
- Active-scene FPS floor: `30`
- Nightly-ritual FPS floor: `24`
- Renderer memory ceiling: `96 MB`
- Preferred warm renderer memory target: `64 MB`
- Allowed nightly-ritual extra memory over warm state: `12 MB`

## Fallback rule

If the native renderer cannot prove availability and healthy startup inside the early check window, the app should stay on the current Expo shell/lair presentation and surface the degradation reason honestly.

## Scope rule

The first production `castle-go` slice replaces only:
- Main Hall
- Lair
- nightly ritual scenes

All other castle ambitions remain deferred until this slice is stable inside the launch envelope.
