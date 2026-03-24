# Rate Limit Handling Rules

## Purpose
Adapt cleanly to throttling or temporary refusal without losing the mission.

## Detection
If encountering:
- rate limit
- too many requests
- quota exceeded
- temporary throttle
- repeated network refusal due to frequency

switch to RATE LIMIT MODE immediately.

## Relay 500 errors
HTTP 500 from the relay follows the same cooldown policy as rate limits.
A 500 means the relay is broken, not an auth or entitlement issue.
PicoClaw retries the relay call up to 3 times (2s / 5s / 10s backoff) before transitioning the active task to stuck.
Do not retry indefinitely on a 500.

## RATE LIMIT MODE
1. stop issuing the limited request type
2. save current state safely
3. record:
   - timestamp
   - action being performed
   - command or request type
4. enter cooldown

## Cooldown policy
- first hit: 60 seconds
- repeat soon after: 120 seconds
- repeated again: 300 seconds

## During cooldown
You may still:
- analyze already collected data
- update capability profiles
- refine procedures
- prepare the next command

Do not keep hammering the blocked interface.
