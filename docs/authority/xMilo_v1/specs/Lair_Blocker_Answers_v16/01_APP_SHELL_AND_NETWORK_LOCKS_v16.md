# App Shell, Network, and Chat Locks — v16

These are product-ready implementation locks only.
They do not change the approved core architecture.

## 1. Android system back button
Locked behavior by surface:

- **World view, nothing open**
  - default Android back behavior: app backgrounds / exits the current activity
- **Hamburger menu open**
  - closes the menu
- **Archive/Trophy inspector open**
  - closes the inspector
- **Settings → Storage & Reset**
  - goes back one level
- **Setup wizard mid-flow**
  - goes back one step, not full-cancel
- **Setup wizard first step**
  - back exits to the pre-setup intro screen
- **Pre-setup intro screen**
  - default Android back behavior

## 2. Mid-session network loss while Milo is working
Locked v1 behavior:
- PicoClaw retries the failed relay call **3 times** with backoff:
  - 2s
  - 5s
  - 10s
- if all retries fail:
  - PicoClaw emits `runtime.error` with `recoverable = true`
  - active task transitions to `task.stuck`
  - stuck reason should indicate network/connection loss
- no special paused state is added in v1

## 3. Brand-new user before setup wizard
Locked v1 first-launch screen:
- a simple intro screen before the setup wizard
- includes:
  - what Milo is
  - why local runtime setup is needed
  - one primary button: `Begin Setup`

## 4. Guided first task after setup
Locked v1:
- after setup completes, show a lightweight first-task helper state
- include 3–5 starter prompt chips, e.g.:
  - `Summarize what you can do`
  - `Help me plan my day`
  - `Organize my priorities`
  - `What should I know about this app?`

## 5. Notification tap behavior
Locked v1:
- tap **Task complete** notification:
  - open app to Main Hall context
  - surface the completed report in the menu/chat flow
- tap **Task stuck** notification:
  - open app to Main Hall context
  - surface the stuck recovery UI/menu state

## 6. Chat rendering
Locked v1:
- support **basic markdown rendering only**
- supported:
  - paragraphs
  - bullet/numbered lists
  - bold
  - inline code
- not supported:
  - raw HTML
  - complex tables
  - embedded media

## 7. Copy to clipboard
Locked v1:
- long-press on a Milo message copies the plain-text message content

## 8. Prompt input limit
Locked v1:
- soft warning at **6000 characters**
- hard cap at **8000 characters**
- file input remains deferred from v1

## 9. Tapping sleeping Milo
Locked v1 UX:
- if `POST /thought/request` returns `accepted = false` with `reason = sleeping`
- app shows a brief sleeping bubble/status, e.g.:
  - `Zzz... Milo is sleeping.`
- no wake is triggered from a simple tap

## 10. Archive inspector at scale
Locked v1:
- simple list UI
- newest first
- infinite scroll
- simple search bar over title + short description
- no advanced filters in v1

## 11. Long task timeout
Locked v1:
- maximum wall-clock task duration = **10 minutes**
- if exceeded:
  - PicoClaw transitions task to `task.stuck`
  - stuck reason should indicate timeout
