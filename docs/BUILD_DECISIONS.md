# Build Decisions — xMilo_v3

These decisions are locked unless a later authority update changes them.
When something was a normal safe app-default, it was chosen automatically instead of left open.

## Product scope

- Android only by design.
- Phones are the primary target.
- Tablets may work, but are not specially optimized in v1.
- Minimum supported Android version for the first real build: Android 11 / API 30.
- If a device is below Android 11, the app should say plainly that the device is too old for the current xMilo build.
- xMilo should aim to support older and newer Android phones, not just recent flagships.
- Native sidecar builds should target `arm64-v8a` and `armeabi-v7a` first.
- `x86` / `x86_64` are non-priority for now.

## Architecture

- `Mind v4` remains the policy + knowledge layer.
- `PicoClaw Go` remains the only runtime authority on-device.
- The user-facing product / sidecar name is **xMilo**.
- The relay is a separate hosted backend.
- The sidecar and relay are both implemented in **Go**.
- The mobile app is implemented in **Expo / React Native / TypeScript**.
- Sidecar persistence uses **SQLite**.
- Relay persistence uses **Postgres**.
- The relay must stay provider-isolated so the app and sidecar do not depend on provider-specific behavior.
- xAI / Grok is the first provider target.
- Future provider swaps remain a relay-only concern.
- Provider/model selection belongs in relay config, not in the app or sidecar.

## Local runtime / setup

- Google Play delivery must be explicit about the Termux dependency.
- Users must install **F-Droid**, then **Termux from F-Droid**, then **Termux:API from F-Droid**.
- There is no no-Termux mode in v1.
- The app should open the required install pages for the user instead of making them hunt for them.
- After Termux and Termux:API are present, the app should do as much of the remaining xMilo sidecar setup as possible automatically.
- The app should auto-detect device CPU architecture and fetch the correct xMilo sidecar build behind the scenes.
- The likely hidden binary source is GitHub Releases from the dedicated Milo fork repo.
- Users should never be sent to browse that source manually as the main flow.
- The setup wizard should prefer the smallest number of steps and the fewest windows possible.
- Setup is allowed to remain incomplete.
- Users can choose `Not now`.
- The app should land users in the Main Hall first, with a large primary setup card while setup is incomplete.
- After setup is complete, that large setup card should disappear entirely.
- Main Hall input should remain usable even if setup is incomplete.
- Only prompt the missing setup step when the requested action actually needs it.
- If setup is incomplete and a user triggers something that needs missing setup, open only the relevant setup step.
- Health/install verification for xMilo should run automatically on app launch.
- If xMilo is unhealthy or missing, the app should try to repair it automatically first.
- While auto-repair is in progress, show a simple `please wait, solving issues` style popup.
- Normal users should not see advanced repair/debug screens.
- If auto-repair fails, tell the user plainly and send an admin/backend error report.
- Sidecar/bootstrap install failures should retry automatically before falling back to guided repair.

## Permissions

- Ask only for core-required app permissions during setup.
- Users may still skip setup.
- If a later action needs a missing permission, prompt again at that moment.
- Camera permission is still requested during setup even though actual capture remains consent-gated later.
- Speech is OFF by default for new users.
- Notifications are OFF by default until allowed.
- Optional powers are opt-in, not opt-out.
- When optional scope matters, xMilo should ask the user instead of silently defaulting:
  - all apps or selected apps
  - all content or selected content
- If a user-enabled permission is later revoked in Android settings, xMilo should surface that the feature is wanted but currently unavailable and provide a repair path back into settings.

## Auth / identity / access

- Earlier `full accounts` still come later.
- However, all users must verify email before getting access, including access-code users.
- Email verification currently uses an emailed verification link from the app flow.
- The app collects email on one screen, sends the verification email, then waits for the user to return after tapping the link.
- If a later authority update re-locks inline numeric codes, that will be a deliberate product change rather than an assumption.
- Verification emails are sent from a dedicated no-reply email address under the user's domain.
- Relay handles verification code generation, storage, and sending in v1.
- Relay sends mail via SMTP.
- After verification succeeds, xMilo remembers the verified email on-device.
- Normal app behavior applies: the user stays logged in across launches unless they log out, reinstall, or clear app data.
- A visible `Log out` action exists in Settings in v1.
- Logging out returns the user to the email verification / access flow.
- Local archive/trophy data remains unless the user explicitly resets it.
- Session handling should use the normal safe default: short-lived access token plus refresh token.
- App-side TOTP 2FA with recovery codes is part of the real account boundary.
- Paid/code-based access belongs to the verified email identity, not to one disposable device install.
- Only one active phone should hold that verified-email access at a time.

## Trial / invite / paid access

- Launch starts in `access-code only` mode.
- During launch, verified users must redeem an access code after email verification.
- Launch-stage access codes grant 30 days by default.
- Later public access is unlocked by flipping relay config, not by rebuilding the auth flow.
- After public access opens, access codes still remain valid and continue stacking normally.
- Access choices shown after email verification depend on relay launch mode:
  - launch `code_only`: redeem code only
  - later public mode: start free trial / redeem code / subscribe
- If the verified email already used the one-time free trial, the free-trial slot remains visible but is labeled `already used`.
- Subscribe and redeem-code should use one shared inline panel that switches modes on the same screen.
- Website-issued access codes do not bypass verified email or 2FA requirements.
- Multiple redeemed access codes should compound and extend access instead of overwriting remaining time.
- Free trial length: 12 hours.
- Trial starts on the user's first real task/request, not on first launch or setup completion.
- When the trial starts, show a small non-blocking notice.
- If the trial ends mid-task, xMilo saves position, stops, returns to the Main Hall, and informs the user according to the existing speech/text toggle rules.
- If speech popup toggle is off, do not speak and still show the popup.
- When access ends, xMilo tells the user to click, then shows the paywall with two actions:
  - go to payment
  - redeem code
- Access-code redemption is available both during setup and in settings/paywall-style recovery surfaces.
- Access codes are single-use only.
- Initial batch size may still be manually issued, but the long-term system is monthly access codes rather than beta invites.
- Access-code redemption grants the configured fixed period, currently 30 days during launch.
- Access-code ownership survives reinstall/new phone because it belongs to the verified email, but switching devices should revoke the old active phone.
- Paid subscriptions support Restore Purchases across reinstalls/new phones.
- One paid tier only.
- Monthly only.
- Current pricing target: $19.99 / month.
- Play billing path uses Google Play Billing with RevenueCat entitlement state.
- Restore Purchases should exist both in Settings and on the paywall.
- When access resumes after payment or code redemption following interruption, xMilo should report the saved interrupted task and ask whether to resume or start a new process.
- Interrupted tasks are always archived.
- Successful invite-code redemption on the paywall should show a short success popup before returning to resume/new-process choice.
- If a one-time trial cannot be protected reliably enough using on-device state alone, email identity gating is the fallback anti-abuse layer for trial/subscription access.

## Website payment boundary

- The mobile app does not connect directly to blockchain.
- The website at `sol.xmiloatyourside.com` is retrieval + crypto payment only.
- There are no standalone website accounts.
- Identity authority remains app + relay.
- Website sessions are created only from short-lived relay-signed handoff tokens.
- Website handoff is allowed only when the verified email proof is complete on the current phone.
- If 2FA is enabled for that verified email, website handoff requires this phone to be the currently trusted 2FA device first.
- QR-based website sign-in is only a transport layer for that same short-lived handoff token, not a second auth model.
- Website purchases are crypto-only and remain separate from the in-app subscription lane.

## Setup/access UX rules

- Use the same paywall surface rather than multiple different subscription screens.
- Prefer inline flows over extra windows when possible.
- Use the simplest safe app-normal default unless a choice changes architecture, security, billing/access, anti-abuse, or is hard to reverse later.
- After successful automatic xMilo sidecar install/setup, show a small success popup.

## Behavior / speech / ambient UX

- When speech is enabled, xMilo should speak on:
  - task start
  - important findings
  - task completion
  - interruptions/errors
- Spoken important updates should show matching text/popups only if the existing toggle says they should.
- Ambient idle behavior is ON.
- Ambient behavior includes small movements and sometimes wandering.
- Idle lines should only appear when speech is enabled.
- Idle wandering is limited to a safe subset of rooms such as Main Hall, Trophy Room, and Archive.
- At 2:00 AM local time, xMilo performs nightly upkeep:
  - checks for new app updates
  - archives the day
  - emits vocal and physical ritual cues when the runtime can do so honestly
- If a real task is active at 2:00 AM, nightly upkeep waits and runs immediately after the task finishes.

## Admin / support

- A protected admin web page should exist from the start.
- It should be small and served by the relay itself.
- It should use a single admin password in relay/server environment config in v1.
- The admin surface should not use an obvious public `admin.` subdomain in v1.
- Exact hidden path/subdomain choice can be decided later.
- Admin page should not provide routine access to user content in v1.
- It may show basic install health metadata like online/offline, last seen, app version, sidecar version, and entitlement state.
- It should support invite-code management.
- Codes should show unused / used status, and if used, show the redeemed email and redeemed date.
- Invite redemptions should store both the code record and the verified email that used it.
- Error reports should appear in the admin page only in v1.
- Email/alert notifications can be added later.
- Error reports stay until manually cleared.
- Admin error-report UI should start as a simple list.
- Cleared reports move to a separate cleared list.

## Local bridge security

- App ↔ sidecar auth uses a random bearer token generated per install and stored locally by the app and sidecar.
- No shared/global localhost secret.

## Hosting / delivery assumptions

- Keep deployment assumptions simple and walkthrough-friendly.
- Default hosted target is one simple VPS + Docker Compose + Postgres, but this remains a deployment target placeholder until the user is ready to set up hosting.
