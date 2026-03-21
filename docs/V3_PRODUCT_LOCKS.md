# xMilo v3 — Locked Product Rules

This file exists so the package itself remembers the current product rules without relying on chat history.
It is not meant to replace the authority docs. It is the current build-facing lock sheet.

## First-launch shape

- User lands in Main Hall first.
- If setup is incomplete, show one large primary setup card.
- Setup may be skipped with `Not now`.
- Setup card disappears entirely after setup is complete.
- Main Hall remains usable while setup is incomplete.
- Only open the specific missing setup step when a requested action requires it.

## Setup flow

1. verify/install F-Droid if missing
2. verify/install Termux from F-Droid
3. verify/install Termux:API from F-Droid
4. verify email on one screen
5. show access choices inline
6. perform xMilo sidecar install / verify automatically behind the scenes when needed
7. auto-check xMilo health on each app launch
8. auto-repair before asking the user for anything

## Email verification

- required for everyone before access
- 6-digit code
- 10-minute expiry
- resend allowed after 60 seconds
- SMTP via dedicated no-reply mailbox under the domain
- same screen email + send + code + submit
- remember verified email on-device after success

## Access choices after verification

- start free trial
- redeem code
- subscribe

If the verified email already used the one-time trial:
- keep the trial slot visible
- label it `already used`

Subscribe and redeem-code share one inline panel that switches modes.

## Trial rules

- one-time 12-hour free trial
- starts on first real task/request
- show small non-blocking `trial started` notice
- if the trial ends mid-task:
  - save position
  - stop task
  - return to Main Hall
  - notify according to speech/text toggle rules

## Invite-code rules

- invite-code only beta access in the early rollout
- 20 codes pre-generated in the first batch
- single-use only
- burn permanently after redemption
- redemption available during setup and on the paywall
- each redeemed code stores the verified email and redeemed date
- redeemed code grants 5 days of access, then payment is required
- reinstall/new-device loses invite access in v1

## Payment / subscription rules

- one monthly tier only
- target price: $19.99/month
- Play Billing + RevenueCat entitlement state
- Restore Purchases visible in Settings and on the paywall
- same paywall surface should be reused rather than special one-off subscription screens
- access-ended paywall always offers:
  - go to payment
  - redeem code

After payment or invite redemption following interruption:
- xMilo reports the interrupted saved task
- user chooses resume or start new process
- interrupted task is always archived

## Session / app-normal defaults

- keep the user logged in across launches
- visible Log out action in Settings
- logout returns to email verification/access flow
- local archive/trophies stay unless explicitly reset
- use short-lived access token + refresh token

## Permissions / toggles

- request all relevant permissions during setup
- speech off by default
- notifications off by default
- camera permission requested during setup, but capture remains consent-gated later
- spoken important updates show matching popup/text only if the user's toggle says they should

## Runtime personality / ambient behavior

- ambient idle behavior on
- small movement / room wandering / idle presence
- idle lines only when speech is enabled
- safe idle room subset: Main Hall, Trophy Room, Archive
- speech moments when enabled:
  - start
  - important findings
  - completion
  - interruptions/errors

## Auto-repair / failure posture

- user should do as little as possible
- normal users do not see advanced repair tools
- xMilo tries to fix issues automatically first
- show simple `please wait, solving issues` popup while repairing
- if it still fails, tell the user plainly and send a backend/admin error report
- admin page keeps unresolved and cleared errors in separate lists

## Admin page scope

- small protected admin page served by relay
- hidden / non-obvious public location
- single admin password in env config in v1
- no routine user-content access
- allowed metadata: online/offline, last seen, app version, sidecar version, entitlement state
- invite-code management included
- simple error report list included
