# xMilo v9 notes

## What changed

This is a minimal-delta RevenueCat pass focused only on the Expo app shell.

### Expo app
- Added `expo-dev-client`, `react-native-purchases`, and `react-native-purchases-ui` dependencies.
- Added RevenueCat env surface to `app.config.ts` and `.env.example`.
- Added Android billing permission in Expo config.
- Added `src/lib/revenuecat.ts` helper for:
  - configure once per current app user ID
  - paywall presentation
  - entitlement check
  - restore purchases
- Updated `app/setup.tsx`:
  - syncs RevenueCat when runtime ID becomes available
  - repurposes the old subscribe stub into a real paywall entry
  - waits for relay entitlement after purchase/restore instead of assuming immediate access
- Updated `app/settings.tsx`:
  - Restore Purchases is now wired through RevenueCat
- Updated `apps/expo-app/README.md` to reflect the new app-side billing state.

## Intentionally not changed

- Relay DB schema
- RevenueCat webhook handling in relay
- device-based entitlement model already present in relay
- sidecar auth/session behavior
- install/bootstrap flow
- Customer Center UI
- final store UI / Wizard Lair visuals

## Important remaining note

Current relay subscription grants still key off the existing device/runtime-style app user ID path.
That keeps this patch compatible with v8 without rewriting the backend.
For real cross-device subscription restore, production should later move RevenueCat app user identity to a stable relay-owned user ID tied to the verified user record.

## Before testing

1. Add `EXPO_PUBLIC_RC_ANDROID_API_KEY` to the Expo app env.
2. Make sure RevenueCat has:
   - an entitlement matching `EXPO_PUBLIC_RC_ENTITLEMENT_ID` (default `xmilo_pro`)
   - a current offering / paywall with the monthly product
3. Run dependency install in `apps/expo-app`.
4. Build a development client before real purchase testing.

## Expected behavior now

- Setup -> Subscribe opens the RevenueCat paywall.
- Successful purchase or restore does **not** instantly force access locally.
- The app polls relay auth briefly and only continues once relay entitlement catches up.
- If RevenueCat succeeded but relay has not caught up yet, the user gets a plain "finalizing access" message instead of a silent failure.
