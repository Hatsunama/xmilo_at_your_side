// Package game implements the Ebiten game loop for the Milo Wizard Lair castle scene.
// The Game struct is the single point of integration between:
//   - PicoClaw WebSocket events (received via client.Connect)
//   - Milo's visual state (MiloAnimator)
//   - Room scene rendering (RoomScene)
//   - Isometric camera projection (Camera)
//
// All spatial decisions come from PicoClaw. This file only executes them visually.
package game

import (
	"encoding/json"
	"image/color"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"xmilo/castle-go/internal/client"
)

// Game implements the ebiten.Game interface.
// It is constructed by NewGame() and passed to ebiten.RunGame() (standalone)
// or mobile.SetGame() (gomobile bind for React Native integration).
type Game struct {
	cam     *Camera
	milo    *MiloAnimator
	scene   *RoomScene
	idle    *IdleDirector
	eventCh chan client.RawEvent

	// current known room and anchor from PicoClaw
	currentRoomID string
	currentAnchor string
	currentState  string
	currentRoute  []RouteStep
	routeLast     map[string]int

	// screen dimensions — updated in Layout()
	screenW int
	screenH int

	initialized bool
	loggedLayout bool
	loggedDraw   bool
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

// NewGame constructs the game and starts the PicoClaw WebSocket connection.
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
		g.initialized = true
		log.Printf("game: layout initialized outside=%dx%d room=%s state=%s", outsideW, outsideH, g.currentRoomID, g.currentState)
		g.loggedLayout = true
		g.loggedDraw = false
	}
	return outsideW, outsideH
}

// Update implements ebiten.Game. Called 60 times per second.
// Drains all pending PicoClaw events and advances animations.
func (g *Game) Update() error {
	if !g.initialized {
		return nil
	}

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

// Draw implements ebiten.Game. Called every frame (up to 60fps).
func (g *Game) Draw(screen *ebiten.Image) {
	if !g.initialized {
		return
	}
	screen.Fill(color.RGBA{R: 255, G: 0, B: 255, A: 255})
	if !g.loggedDraw {
		roomID := ""
		bgW, bgH := 0, 0
		if g.scene != nil {
			roomID = g.scene.activeID
			if room, ok := g.scene.rooms[g.scene.activeID]; ok && room.background != nil {
				bgW = room.background.Bounds().Dx()
				bgH = room.background.Bounds().Dy()
			}
		}
		log.Printf("game: first draw screen=%dx%d active=%s bg=%dx%d", screen.Bounds().Dx(), screen.Bounds().Dy(), roomID, bgW, bgH)
		g.loggedDraw = true
	}
	// RoomScene.Draw takes a z-order value for Milo and a closure that draws him.
	// This lets the scene interleave Milo correctly between props (painter's algorithm).
	g.scene.Draw(screen, g.milo.ZOrder, func() {
		g.milo.Draw(screen)
	})
}

// ApplyRawEvent routes a single event through the production renderer path.
// Callers are expected to initialize the game via Layout first.
func (g *Game) ApplyRawEvent(ev client.RawEvent) {
	if !g.initialized {
		return
	}
	g.handleEvent(ev)
}

// handleEvent processes a single raw event from PicoClaw.
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
		// If PicoClaw provided a nonzero duration use it; otherwise compute from grid distance.
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

	// "milo.room_changed" — Milo has arrived. Switch the active room background.
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
