// Package game implements the Ebiten game loop for the Milo Wizard Lair castle scene.
// The Game struct is the single point of integration between:
//   - xMilo sidecar WebSocket events (received via client.Connect)
//   - Milo's visual state (MiloAnimator)
//   - Room scene rendering (RoomScene)
//   - Isometric camera projection (Camera)
package game

import (
	"encoding/json"
	"log"
	"math"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
	"xmilo/castle-go/internal/assets"
	"xmilo/castle-go/internal/client"
)

const resumeProbeLogLimit = 20

// Game implements the ebiten.Game interface.
// It is constructed by NewGame() and passed to ebiten.RunGame() (standalone)
// or mobile.SetGame() (gomobile bind for React Native integration).
type Game struct {
	cam     *Camera
	milo    *MiloAnimator
	scene   *RoomScene
	idle    *IdleDirector
	eventCh chan client.RawEvent

	// current known room and anchor from the sidecar
	currentRoomID string
	currentAnchor string
	currentState  string
	currentRoute  []RouteStep
	routeLast     map[string]int

	// screen dimensions — updated in Layout()
	screenW int
	screenH int

	initialized  bool
	loggedLayout bool
	loggedDraw   bool

	cameraTouchCount int
	cameraTouchLast  map[ebiten.TouchID]touchPoint
	cameraPinchMidX  float64
	cameraPinchMidY  float64
	cameraPinchDist  float64

	assetsRefreshed bool

	diagFrame        int
	diagPixels       []byte
	probeLogged      bool

	mainHallFallbackChecked bool
	mainHallFallbackApplied bool

	resumeProbeUpdateLogs int
	resumeProbeDrawLogs   int
}

func newGameWithChannel(ch chan client.RawEvent) *Game {
	return &Game{
		eventCh:       ch,
		currentRoomID: "main_hall",
		currentAnchor: "main_hall_center",
		currentState:  "idle",
		routeLast:     map[string]int{},
	}
}

// NewGame constructs the game and starts the sidecar WebSocket connection.
// wsURL is typically "ws://127.0.0.1:42817/ws".
func NewGame(wsURL string) *Game {
	ch := make(chan client.RawEvent, 128)
	go client.Connect(wsURL, ch)

	return newGameWithChannel(ch)
}

// NewOfflineGame constructs the game without opening a WebSocket connection.
// This is used for deterministic fixture playback and local visual validation.
func NewOfflineGame() *Game {
	return newGameWithChannel(make(chan client.RawEvent, 128))
}

// Layout implements ebiten.Game. Called before the first Update/Draw.
// Sets the logical screen size and initializes camera/scene/milo on first call.
func (g *Game) Layout(outsideW, outsideH int) (int, int) {
	// Ebiten mobile can report a transient 0×0 (or 1×1) size during view attach/layout.
	// If we accept it, we can initialize the renderer at an invisible size and stay black.
	if outsideW < 2 || outsideH < 2 {
		if g.screenW >= 2 && g.screenH >= 2 {
			return g.screenW, g.screenH
		}
		return 1, 1
	}
	if !g.initialized || g.screenW != outsideW || g.screenH != outsideH {
		g.screenW = outsideW
		g.screenH = outsideH
		g.cam = NewCamera(outsideW, outsideH)
		g.milo = NewMiloAnimator(g.cam)
		g.scene = NewRoomScene(g.cam)
		g.idle = NewIdleDirector()
		g.scene.SetActiveRoom(g.currentRoomID)
		g.scene.SetMiloState(g.currentState)
		g.scene.SetRoute(nil)
		g.initializeCameraView()
		g.initialized = true
		log.Printf("game: layout initialized outside=%dx%d room=%s state=%s", outsideW, outsideH, g.currentRoomID, g.currentState)
		g.loggedLayout = true
		g.loggedDraw = false
	}
	return outsideW, outsideH
}

func (g *Game) cameraWorldBounds() WorldBounds {
	return CastleWorldBounds(g.cam)
}

func (g *Game) initializeCameraView() {
	if g.cam == nil {
		return
	}
	worldBounds := g.cameraWorldBounds()
	g.cam.FitToBounds(worldBounds, 36, 96)

	mainHallLayout, ok := RoomWorldLayoutFor("main_hall")
	if !ok {
		return
	}
	mainHallBounds := mainHallLayout.Bounds().Normalized()
	if !mainHallBounds.Valid() {
		return
	}

	targetRoomWidth := float64(g.cam.ScreenW) * 0.70
	roomZoom := clamp(targetRoomWidth/mainHallBounds.Width(), g.cam.View.MinZoom, g.cam.View.MaxZoom)
	g.cam.View.Zoom = roomZoom

	mainHallCenterX := (mainHallBounds.MinX + mainHallBounds.MaxX) / 2
	mainHallCenterY := (mainHallBounds.MinY + mainHallBounds.MaxY) / 2
	screenCenterX := float64(g.cam.ScreenW) / 2
	screenCenterY := float64(g.cam.ScreenH) / 2

	g.cam.View.PanX = screenCenterX - g.cam.OffsetX - (mainHallCenterX-g.cam.OffsetX)*g.cam.View.Zoom
	g.cam.View.PanY = screenCenterY - g.cam.OffsetY - (mainHallCenterY-g.cam.OffsetY)*g.cam.View.Zoom
	g.cam.ClampToBounds(worldBounds, 36, 96)
}

func (g *Game) clampCameraToWorld() {
	if g.cam == nil {
		return
	}
	g.cam.ClampToBounds(g.cameraWorldBounds(), 36, 96)
}

// Update implements ebiten.Game. Called 60 times per second.
// Drains all pending sidecar events and advances animations.
func (g *Game) Update() error {
	if !g.initialized {
		return nil
	}
	g.logResumeProbeUpdate()

	g.consumeCameraTouches()

	// Drain all pending events — process everything that arrived since last tick
drain:
	for {
		select {
		case ev := <-g.eventCh:
			g.handleEvent(ev)
		default:
			break drain
		}
	}

	g.milo.Tick(g.cam)
	g.idle.Tick(g)
	g.scene.Tick()
	return nil
}

type touchPoint struct {
	x float64
	y float64
}

func (g *Game) consumeCameraTouches() {
	if g.cam == nil {
		return
	}

	touchIDs := ebiten.AppendTouchIDs(nil)
	if len(touchIDs) == 0 {
		g.cameraTouchCount = 0
		g.cameraTouchLast = nil
		g.cameraPinchMidX = 0
		g.cameraPinchMidY = 0
		g.cameraPinchDist = 0
		return
	}

	sort.Slice(touchIDs, func(i, j int) bool { return touchIDs[i] < touchIDs[j] })

	current := make(map[ebiten.TouchID]touchPoint, len(touchIDs))
	for _, id := range touchIDs {
		x, y := ebiten.TouchPosition(id)
		current[id] = touchPoint{x: float64(x), y: float64(y)}
	}

	if g.cameraTouchCount != len(touchIDs) {
		g.cameraTouchCount = len(touchIDs)
		g.cameraTouchLast = current
		g.cameraPinchMidX, g.cameraPinchMidY, g.cameraPinchDist = cameraTouchMetrics(touchIDs, current)
		return
	}

	if len(touchIDs) == 1 {
		id := touchIDs[0]
		prev, ok := g.cameraTouchLast[id]
		next := current[id]
		if ok {
			g.cam.PanBy(next.x-prev.x, next.y-prev.y)
			g.clampCameraToWorld()
		}
		g.cameraTouchLast = current
		return
	}

	midX, midY, dist := cameraTouchMetrics(touchIDs, current)
	if g.cameraPinchDist > 0 && dist > 0 {
		g.cam.ZoomAround(dist/g.cameraPinchDist, midX, midY)
	}
	g.cam.PanBy(midX-g.cameraPinchMidX, midY-g.cameraPinchMidY)
	g.clampCameraToWorld()
	g.cameraTouchLast = current
	g.cameraPinchMidX = midX
	g.cameraPinchMidY = midY
	g.cameraPinchDist = dist
}

func cameraTouchMetrics(touchIDs []ebiten.TouchID, touches map[ebiten.TouchID]touchPoint) (float64, float64, float64) {
	if len(touchIDs) == 0 {
		return 0, 0, 0
	}
	if len(touchIDs) == 1 {
		p := touches[touchIDs[0]]
		return p.x, p.y, 0
	}
	first := touches[touchIDs[0]]
	second := touches[touchIDs[1]]
	midX := (first.x + second.x) / 2
	midY := (first.y + second.y) / 2
	dist := math.Hypot(second.x-first.x, second.y-first.y)
	return midX, midY, dist
}

// Draw implements ebiten.Game. Called every frame (up to 60fps).
func (g *Game) Draw(screen *ebiten.Image) {
	if !g.initialized {
		return
	}
	g.logResumeProbeDraw(screen)
	// RoomScene.Draw takes a z-order value for Milo and a closure that draws him.
	// This lets the scene interleave Milo correctly between props (painter's algorithm).
	g.scene.Draw(screen, g.milo.ZOrder, func() {
		g.milo.Draw(screen)
	})
}

func (g *Game) logResumeProbeUpdate() {
	if g.resumeProbeUpdateLogs >= resumeProbeLogLimit {
		return
	}
	g.resumeProbeUpdateLogs++
	log.Printf(
		"XMILO_RESUME_PROBE_R1 go_update count=%d initialized=%t screen=%dx%d room=%s state=%s cam_zoom=%.3f cam_pan=(%.1f,%.1f)",
		g.resumeProbeUpdateLogs,
		g.initialized,
		g.screenW,
		g.screenH,
		g.currentRoomID,
		g.currentState,
		g.cameraZoomForProbe(),
		g.cameraPanXForProbe(),
		g.cameraPanYForProbe(),
	)
}

func (g *Game) logResumeProbeDraw(screen *ebiten.Image) {
	if g.resumeProbeDrawLogs >= resumeProbeLogLimit {
		return
	}
	g.resumeProbeDrawLogs++
	screenW, screenH := 0, 0
	if screen != nil {
		bounds := screen.Bounds()
		screenW = bounds.Dx()
		screenH = bounds.Dy()
	}
	log.Printf(
		"XMILO_RESUME_PROBE_R1 go_draw count=%d initialized=%t screen=%dx%d image=%dx%d room=%s state=%s cam_zoom=%.3f cam_pan=(%.1f,%.1f)",
		g.resumeProbeDrawLogs,
		g.initialized,
		g.screenW,
		g.screenH,
		screenW,
		screenH,
		g.currentRoomID,
		g.currentState,
		g.cameraZoomForProbe(),
		g.cameraPanXForProbe(),
		g.cameraPanYForProbe(),
	)
}

func (g *Game) cameraZoomForProbe() float64 {
	if g.cam == nil {
		return 0
	}
	return g.cam.View.Zoom
}

func (g *Game) cameraPanXForProbe() float64 {
	if g.cam == nil {
		return 0
	}
	return g.cam.View.PanX
}

func (g *Game) cameraPanYForProbe() float64 {
	if g.cam == nil {
		return 0
	}
	return g.cam.View.PanY
}

func (g *Game) maybeApplyMainHallProceduralFallback() {
	if g.mainHallFallbackChecked || g.scene == nil {
		return
	}
	if g.scene.activeID != "main_hall" {
		g.mainHallFallbackChecked = true
		return
	}
	room, ok := g.scene.rooms[g.scene.activeID]
	if !ok || room == nil || room.background == nil {
		g.mainHallFallbackChecked = true
		return
	}

	bgW, bgH := room.background.Bounds().Dx(), room.background.Bounds().Dy()
	if bgW <= 0 || bgH <= 0 {
		g.mainHallFallbackChecked = true
		return
	}

	pixels := make([]byte, 4*bgW*bgH)
	room.background.ReadPixels(pixels)
	cx, cy := bgW/2, bgH/2
	index := 4 * (cy*bgW + cx)
	red, green, blue, alpha := pixels[index], pixels[index+1], pixels[index+2], pixels[index+3]
	luma := 0.2126*float64(red) + 0.7152*float64(green) + 0.0722*float64(blue)

	// Bounded trigger: only fallback when main_hall center sample is effectively near-black.
	if luma <= 20 && alpha > 0 {
		room.background = assets.LoadRoomBackgroundProcedural("main_hall", bgW, bgH)
		g.mainHallFallbackApplied = true
		log.Printf("game: main_hall procedural fallback applied center_rgba=%d,%d,%d,%d luma=%.2f", red, green, blue, alpha, luma)
	}

	g.mainHallFallbackChecked = true
}

func (g *Game) logFirstFrameContentProbe(screen *ebiten.Image) {
	if g.probeLogged || g.scene == nil || screen == nil {
		return
	}
	room, ok := g.scene.rooms[g.scene.activeID]
	if !ok || room == nil || room.background == nil {
		log.Printf("probe: room/background missing active=%s", g.scene.activeID)
		g.probeLogged = true
		return
	}

	bgW, bgH := room.background.Bounds().Dx(), room.background.Bounds().Dy()
	if bgW <= 0 || bgH <= 0 {
		log.Printf("probe: background invalid size active=%s bg=%dx%d", g.scene.activeID, bgW, bgH)
		g.probeLogged = true
		return
	}

	// Sample the background texture directly.
	bgPixels := make([]byte, 4*bgW*bgH)
	room.background.ReadPixels(bgPixels)
	bcx, bcy := bgW/2, bgH/2
	bi := 4 * (bcy*bgW + bcx)
	br, bgc, bb, ba := bgPixels[bi], bgPixels[bi+1], bgPixels[bi+2], bgPixels[bi+3]

	// Sample the composed screen after scene draw.
	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
	screenPixels := make([]byte, 4*sw*sh)
	screen.ReadPixels(screenPixels)
	scx, scy := sw/2, sh/2
	si := 4 * (scy*sw + scx)
	sr, sg, sb, sa := screenPixels[si], screenPixels[si+1], screenPixels[si+2], screenPixels[si+3]

	log.Printf(
		"probe: active=%s state=%s cam_zoom=%.3f cam_pan=(%.1f,%.1f) bg_center_rgba=%d,%d,%d,%d screen_center_rgba=%d,%d,%d,%d",
		g.scene.activeID,
		g.currentState,
		g.cam.View.Zoom,
		g.cam.View.PanX,
		g.cam.View.PanY,
		br, bgc, bb, ba,
		sr, sg, sb, sa,
	)
	g.probeLogged = true
}

// ApplyRawEvent routes a single event through the production renderer path.
// Callers are expected to initialize the game via Layout first.
func (g *Game) ApplyRawEvent(ev client.RawEvent) {
	if !g.initialized {
		return
	}
	g.handleEvent(ev)
}

// handleEvent processes a single raw event from the sidecar.
// The switch covers every event type the task engine emits.
// Unknown event types are silently ignored — forward compatibility.
func (g *Game) handleEvent(ev client.RawEvent) {
	switch ev.Type {

	// "milo.movement_started" — Milo begins walking to a new anchor.
	// This is emitted before the room changes, so we start the walk animation
	// using the destination anchor's screen coordinates.
	case "milo.movement_started":
		var p MovementStarted
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			log.Printf("game: decode movement_started: %v", err)
			return
		}
		toX, toY := g.cam.AnchorToScreen(p.ToAnchor)
		facing := WalkFacing(p.FromAnchor, p.ToAnchor)
		// If the sidecar provided a nonzero duration use it; otherwise compute from grid distance.
		durationMS := p.EstimatedMS
		if durationMS == 0 {
			durationMS = WalkDurationMS(p.FromAnchor, p.ToAnchor)
		}
		g.milo.StartWalk(toX, toY, facing, durationMS)
		g.currentAnchor = p.ToAnchor
		g.scene.SetMovementIntent(p.ToRoom, p.ToAnchor, p.Reason)
		variant := g.nextRouteVariant(g.currentRoomID, p.ToRoom)
		g.currentRoute = RouteBetweenVariant(g.currentRoomID, p.ToRoom, variant)
		g.scene.SetRoute(g.currentRoute)

	// "milo.room_changed" — Milo has arrived. Update the active room state.
	case "milo.room_changed":
		var p RoomChanged
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			log.Printf("game: decode room_changed: %v", err)
			return
		}
		g.currentRoomID = p.RoomID
		g.currentAnchor = p.AnchorID
		g.scene.SetActiveRoom(p.RoomID)
		log.Printf("game: room changed to=%s anchor=%s scene=%s", p.RoomID, p.AnchorID, SceneRoomID(p.RoomID))
		g.scene.ClearMovementIntent()
		g.currentRoute = nil
		g.scene.SetRoute(nil)
		if g.idle != nil {
			g.idle.Reset()
		}

	// "milo.state_changed" — Milo's behavioral state changed.
	// Maps "idle" | "moving" | "working" to visual animation states.
	case "milo.state_changed":
		var p StateChanged
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			log.Printf("game: decode state_changed: %v", err)
			return
		}
		g.currentState = p.ToState
		// facing stays as-is unless overridden by a subsequent movement event
		g.milo.SetState(p.ToState, g.currentRoomID, "", true)
		g.scene.SetMiloState(p.ToState)
		if g.idle != nil && p.ToState != "idle" {
			g.idle.Reset()
		}

	// "milo.thought" — show a thought bubble above Milo
	case "milo.thought":
		var p MiloThought
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			log.Printf("game: decode milo.thought: %v", err)
			return
		}
		g.milo.ShowThought(p.Text)

	case "maintenance.nightly_deferred":
		g.scene.SetRitualState("deferred")
		g.milo.ShowThought("Nightly upkeep waits until this task is done.")

	case "maintenance.nightly_started":
		g.scene.SetRitualState("started")
		g.milo.ShowThought("Beginning the nightly upkeep ritual.")

	case "maintenance.nightly_completed":
		g.scene.SetRitualState("completed")
		g.milo.ShowThought("Nightly upkeep complete. Archive sealed.")

	// "task.completed" — task done. Milo will return to main_hall via
	// a subsequent "milo.movement_started" event — no extra handling needed here.
	case "task.completed":
		// Trophy eligible could trigger a sparkle effect on the trophy room.
		// Wired for future use — no-op until trophy ambient effects are implemented.

	// "task.stuck" — something went wrong. Play a confused/thinking animation.
	case "task.stuck":
		g.milo.SetState("thinking", g.currentRoomID, g.milo.facing, false)
		g.scene.SetMiloState("thinking")

	// "task.cancelled" — interrupted. Snap back to idle.
	case "task.cancelled":
		g.milo.SetState("idle", "main_hall", "s", true)
		g.currentRoomID = "main_hall"
		g.currentAnchor = "main_hall_center"
		g.currentState = "idle"
		g.scene.SetActiveRoom("main_hall")
		g.scene.SetMiloState("idle")
		g.scene.ClearMovementIntent()
		g.currentRoute = nil
		g.scene.SetRoute(nil)
		if g.idle != nil {
			g.idle.Reset()
		}
	}
}

func (g *Game) nextRouteVariant(from, to string) string {
	variants := HallVariantsForRoute(from, to)
	if len(variants) == 0 {
		return ""
	}
	family := RouteFamily(from, to)
	index := g.routeLast[family] % len(variants)
	variant := variants[index]
	g.routeLast[family] = (index + 1) % len(variants)
	return variant
}
