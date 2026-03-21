# xMilo v10 notes

## What changed (patch on v9)

This is a minimal correctness pass on the v9 RevenueCat wiring.
No new features, no relay/sidecar changes.

### Bug fixes

**Fix 1 — Critical: paywall was unreachable**
`setup.tsx` `access_choice` Subscribe card was still routing to the old
`subscribe_stub` placeholder step instead of calling `handleOpenSubscription`.
`handleOpenSubscription` was fully implemented in v9 but was dead code.
Changed the card's `onPress` to call `handleOpenSubscription` directly.

**Fix 2 — Minor: no loading state on Subscribe card during paywall open**
`ChoiceCard` had no `busy` prop.  Added optional `busy?: boolean` —
when true: card is `disabled`, opacity drops to 0.65, and an
`ActivityIndicator` appears inline beside the title.  The Subscribe card
now passes `busy={busy}` so tapping Subscribe shows a spinner while
the native paywall sheet is opening.

**Fix 3 — Cleanup: `subscribe_stub` step removed**
`subscribe_stub` entry removed from the `Step` union type and its render
block deleted.  No live path reached it after Fix 1.

### Files changed

- `apps/expo-app/app/setup.tsx`

### Files unchanged

Everything else is identical to v9.

## Before testing

Same checklist as v9:

1. Add `EXPO_PUBLIC_RC_ANDROID_API_KEY` to the Expo app env.
2. RevenueCat dashboard: entitlement `xmilo_pro`, active offering with monthly product.
3. `npm install` in `apps/expo-app`.
4. Build a development client — RevenueCat purchase flow requires a dev build.

## Expected behavior

- Setup → email → verified → Subscribe card tap → RC paywall sheet opens immediately.
- Subscribe card shows spinner while paywall is loading.
- Purchase/restore → relay entitlement poll (12s window) → home screen.
- Settings → Restore purchases → unchanged from v9.
