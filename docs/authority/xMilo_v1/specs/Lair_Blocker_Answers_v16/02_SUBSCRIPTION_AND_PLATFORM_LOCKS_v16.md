# Subscription, Platform, and Quality Locks — v16

## 12. Trial expiry warning
Locked v1:
- show an in-app banner when trial has **6 hours remaining**
- schedule one local notification at **6 hours remaining** as well

## 13. Manage Subscription button
Locked v1:
- include a **Manage Subscription** button in Settings
- use the platform subscription management URL provided by RevenueCat when available

## 14. Restore Purchases flow
Locked v1 sequence:
1. user taps `Restore Purchases`
2. app calls RevenueCat restore flow
3. app calls relay `/session/start` again
4. if entitlement is restored:
   - app updates unlocked state
   - if xMilo Sidecar is already running, app calls localhost `/auth/refresh`
   - otherwise normal bootstrap/setup continues

## 15. OTA updates
Locked v1:
- **enabled for JS-layer updates only**
- do not rely on OTA for:
  - native module changes
  - xMilo Sidecar binary changes
  - manifest/permission changes

## 16. In-app rating prompt
Locked v1:
- enabled
- ask after the **3rd successful completed task**
- only if the native review API is available
- never show during setup or immediately after a stuck/entitlement-loss flow

## 17. Accessibility baseline
Locked v1 minimum:
- accessibility labels/content descriptions on all interactive controls
- minimum touch target size **48dp × 48dp**
- readable contrast on UI panels/text
- no unlabeled icon-only critical controls

## 18. Theme behavior
Locked v1:
- app uses a **fixed dark theme**
- world and UI panels stay in the magical-night presentation
- do not follow system light mode in v1
- do respect system font scaling where practical
