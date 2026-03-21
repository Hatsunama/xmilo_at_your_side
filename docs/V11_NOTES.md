# xMilo v11 Notes

## What Changed from v10

### New: castle-go module
A new top-level Go module at `castle-go/` contains the full Ebiten-based
castle renderer. It is a separate Go module from sidecar-go and relay-go.
It does not change or import from either of them at compile time.
It communicates with PicoClaw exclusively via WebSocket at runtime.

**Files added:**
- `castle-go/go.mod` — module xmilo/castle-go
- `castle-go/mobile/mobile.go` — gomobile bind target, exports `Start(wsURL)`
- `castle-go/cmd/castle-standalone/main.go` — test APK entry point
- `castle-go/internal/game/game.go` — ebiten.Game loop
- `castle-go/internal/game/milo.go` — MiloAnimator sprite state machine
- `castle-go/internal/game/camera.go` — isometric projection + anchor registry
- `castle-go/internal/game/rooms.go` — all 9 Wizard Lair rooms + props
- `castle-go/internal/game/events.go` — PicoClaw WS payload structs
- `castle-go/internal/client/wsclient.go` — WS client, auto-reconnect
- `castle-go/internal/assets/assets.go` — asset loader with placeholder fallback
- `castle-go/tools/manifest/main.go` — prints expected asset filenames
- `castle-go/BUILD.md` — full build sequence

### Modified: sidecar-go/internal/rooms/router.go
Added 4 new intent → room mappings. All existing mappings frozen:
- `creative` → spellbook / spellbook_reading
- `processing` → cauldron / cauldron_stir
- `prediction` → crystal_orb / crystal_orb_stand
- `memory` → archive / archive_lectern

### New: apps/expo-app/app/lair.tsx
The Wizard Lair screen. Full-screen castle view with:
- `CastleView` filling the entire screen
- Top HUD showing current room + Milo state
- Bottom event ticker showing last WS event
- Bottom input bar for giving Milo tasks without leaving the castle view
- `headerShown: false` — no navigation bar, true immersive experience

### Modified: apps/expo-app/app/_layout.tsx
Added `lair` screen with `headerShown: false`.

### Modified: apps/expo-app/app/index.tsx
Added `🏰 Enter the Lair` button above the Setup/Settings footer row.
Styled in the purple/midnight castle palette.

### New: apps/expo-app/src/components/CastleModule.tsx
The `CastleView` component. Currently renders a dark starfield placeholder
with castle emoji. Activates the real Ebiten renderer once `castle.aar`
is built and `CastleModule` NativeModule is registered.

### New: apps/expo-app/android-native/CastleModule.java
The Java NativeModule bridge. Must be manually placed at:
`android/app/src/main/java/com/xmilo/milo/CastleModule.java`
after `npx expo prebuild` generates the android/ directory.

## What Has NOT Changed
- relay-go — untouched
- sidecar-go (everything except rooms/router.go) — untouched
- All auth, subscription, setup wizard, archive, settings logic — untouched
- All product locks from V3_PRODUCT_LOCKS.md — still enforced
- All WS event types the task engine emits — unchanged, castle-go consumes them

## Current State of the Lair Screen
The lair screen is navigable and functional right now without building the .aar.
The `CastleView` shows a dark animated placeholder. All HUD elements are live
because they read from AppContext which is populated by the existing WS bridge.

## Next Steps to Activate Real Castle Rendering
See `castle-go/BUILD.md` for the full sequence. Summary:
1. `cd castle-go && go mod tidy` (needs internet, do this on PC)
2. `go run ./cmd/castle-standalone` — verify on desktop first
3. `gomobile build -target android` — test APK on real device
4. `ebitenmobile bind` — produce castle.aar
5. Place .aar in android/app/libs/
6. Place CastleModule.java in android/app/src/.../milo/
7. Register NativeModule in your ReactPackage
8. Uncomment EbitenView line in CastleModule.tsx
9. `npx expo run:android`

## Art Assets Needed
See `docs/ASSET_SPEC.md` for the complete art specification.
The renderer runs with colored placeholders until real art is dropped in.
