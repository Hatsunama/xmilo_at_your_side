# Planned Flows — xMilo_v3

## First open

- open Main Hall
- if setup incomplete, show large setup card
- allow `Not now`
- allow normal text entry unless the requested action needs missing setup

## Email verification + access flow

- user enters email
- tap `Send code`
- relay generates/stores 6-digit code and sends via SMTP
- user enters code on the same screen
- relay verifies code
- app stores verified email / session locally
- app shows access choices inline:
  - start free trial
  - redeem code
  - subscribe
- if trial already used, show that slot as `already used`

## Free trial path

- first real task/request starts the 12-hour clock
- small non-blocking trial-start notice appears
- relay marks trial used for the verified email
- if trial expires mid-task:
  - sidecar saves task position
  - stops execution
  - routes xMilo back to Main Hall
  - app shows the access-ended surface

## Invite-code path

- user enters code inline during setup or paywall
- relay validates code, verifies unused status, burns it, stores redeemed email/date
- grant 5-day beta access window
- app shows short success popup
- if a task was interrupted, present resume/new-process choice

## Subscription path

- user opens inline subscribe mode
- payment uses Google Play Billing / RevenueCat
- relay updates entitlement state after verification
- app returns to resume/new-process choice if an interrupted task exists
- Restore Purchases available from Settings and paywall

## Sidecar bootstrap path

- app confirms F-Droid / Termux / Termux:API presence
- app generates per-install localhost bearer token
- app detects device CPU ABI
- app downloads the matching xMilo sidecar build from the hidden release source
- app verifies integrity
- app hands bootstrap execution to Termux using the run-command style handoff
- app verifies health / ready state on localhost
- app shows small success popup on success
- if health check fails:
  - auto-retry repair
  - only then show simple failure messaging
  - backend/admin receives error report
