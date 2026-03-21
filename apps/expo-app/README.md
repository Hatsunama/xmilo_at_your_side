# apps/expo-app

Expo Router starter for the xMilo Android app shell.

## What this app is right now

- a fixed-dark-theme starter shell
- localhost bridge client to the sidecar
- basic event feed
- task input form
- setup and settings starter screens
- local archive cache via expo-sqlite
- app-side RevenueCat wiring for paywall open + restore purchases

## Important limitations

- this is not the final Wizard Lair world view
- the setup wizard is still a starter shell, not the finished automated flow
- relay-side entitlement truth still depends on RevenueCat webhook delivery before xMilo access flips fully active
- restore-purchases is wired in the app shell, but broader account/logout/payment management surfaces are still incomplete
- no final icon/splash/art assets are included yet
- this should be treated as the first real app shell, not store-ready polish

## RevenueCat dev note

- use an Expo development build for real purchases; hot reload / Expo Go is not enough for full native billing behavior
- add `EXPO_PUBLIC_RC_ANDROID_API_KEY` before testing the subscribe or restore flows
