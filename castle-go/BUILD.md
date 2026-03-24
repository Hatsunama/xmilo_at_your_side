# castle-go Build Notes

`castle-go` is the Go/Ebiten renderer for the first production castle slice.

Current locked scope:
- Main Hall
- Lair
- nightly ritual scenes

Current locked rules:
- conservative Android launch budgets
- fallback to the Expo shell/lair must win over a slow native boot
- nightly ritual visuals must key off real sidecar maintenance events
- the renderer must not outrun runtime truth

## Local Go validation

From this folder:

```powershell
go mod tidy
go build ./...
```

## Standalone desktop smoke test

```powershell
$env:CASTLE_WS_URL='ws://127.0.0.1:42817/ws'
go run ./cmd/castle-standalone
```

If the sidecar WebSocket is unavailable, the renderer should still open and remain visually safe.

## Offline preview export

You can render a static preview without Android packaging:

```powershell
go run ./tools/render-preview -room main_hall -state idle -out main-hall-preview.png
go run ./tools/render-preview -room archive -ritual started -state working -out archive-ritual-preview.png
go run ./tools/render-preview -room crystal_orb -ritual completed -state working -out ritual-observatory-preview.png
```

This is the preferred scene-lane validation path before native embedding is ready.

## Fixture-driven transition validation

You can also render deterministic fixture sequences that stand in for real sidecar traces:

```powershell
go run ./tools/render-fixture -fixture list
go run ./tools/render-fixture -fixture acceptance -outdir fixture-previews
go run ./tools/render-fixture -fixture main_hall_arrival -outdir fixture-previews
go run ./tools/render-fixture -fixture lair_work_cycle -outdir fixture-previews
go run ./tools/render-fixture -fixture nightly_ritual_cycle -outdir fixture-previews
```

This is the current art-truth authority for first-pass transition and timing tuning.
The acceptance suite itself is described in `../docs/CASTLE_ACCEPTANCE_FIXTURES.md`.

## Android AAR build target

The intended Android artifact is `castle.aar`.

Expected bind command:

```powershell
ebitenmobile bind -target android -androidapi 21 -javapkg com.xmilo.castle -o castle.aar ./mobile
```

Alternative path if `ebitenmobile` is not on `PATH`:

```powershell
go run github.com/hajimehoshi/ebiten/v2/cmd/ebitenmobile@v2.9.9 bind -target android -androidapi 21 -javapkg com.xmilo.castle -o castle.aar ./mobile
```

On this workspace, ensure `javac` from `JAVA_HOME` is on `PATH` before binding:

```powershell
$env:PATH="$env:JAVA_HOME\bin;$env:PATH"
go run github.com/hajimehoshi/ebiten/v2/cmd/ebitenmobile@v2.9.9 bind -target android -androidapi 21 -javapkg com.xmilo.castle -o C:\xMilo\xmilo_at_your_side\apps\expo-app\android\app\libs\castle.aar ./mobile
```

## Expected Android integration path

1. Build `castle.aar`.
2. Place it in:

```text
apps/expo-app/android/app/libs/castle.aar
```

3. Add the corresponding dependency wiring in the Android app module if it is not already present.
4. Register the native bridge package only when the AAR is present on classpath.
5. Keep the current Expo fallback path active until native startup is proven healthy within the early budget window.

Current verified Android app packaging results:

```text
apps/expo-app/android/app/build/outputs/apk/debug/app-debug.apk
apps/expo-app/android/app/build/outputs/apk/release/app-release.apk
```

Important runtime note:
- the debug APK is an Expo development build and can route into Dev Launcher behavior when Metro/dev-client expectations are active
- the release APK is the correct embedded validation target for first native castle handoff testing

## Current integration blocker

In this workspace today:
- `castle-go` builds successfully as Go code
- static preview export now works for Main Hall and nightly ritual scenes
- `castle.aar` packaging and Android app registration are a separate workstream

So the remaining native-renderer step outside this folder is Android packaging/integration, not scene truth or procedural renderer validation.
