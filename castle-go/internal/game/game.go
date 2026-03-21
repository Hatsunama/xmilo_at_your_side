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
	eventCh chan client.RawEvent

	// current known room and anchor from PicoClaw
	currentRoomID  string
	currentAnchor  string
	currentState   string

	// screen dimensions — updated in Layout()
	screenW int
	screenH int

	initialized bool
}

// NewGame constructs the game and starts the PicoClaw WebSocket connection.
// wsURL is typically "ws://127.0.0.1:42817/ws".
func NewGame(wsURL string) *Game {
	ch := make(chan client.RawEvent, 128)
	go client.Connect(wsURL, ch)

	return &Game{
		eventCh:       ch,
		currentRoomID: "main_hall",
		currentAnchor: "main_hall_center",
		currentState:  "idle",
	}
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
		g.initialized = true
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
	g.scene.Tick()
	return nil
}

// Draw implements ebiten.Game. Called every frame (up to 60fps).
func (g *Game) Draw(screen *ebiten.Image) {
	if !g.initialized {
		return
	}
	// RoomScene.Draw takes a z-order value for Milo and a closure that draws him.
	// This lets the scene interleave Milo correctly between props (painter's algorithm).
	g.scene.Draw(screen, g.milo.ZOrder, func() {
		g.milo.Draw(screen)
	})
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

	// "milo.thought" — show a thought bubble above Milo
	case "milo.thought":
		var p MiloThought
		if err := json.Unmarshal(ev.Payload, &p); err != nil {
			log.Printf("game: decode milo.thought: %v", err)
			return
		}
		g.milo.ShowThought(p.Text)

	// "task.completed" — task done. Milo will return to main_hall via
	// a subsequent "milo.movement_started" event — no extra handling needed here.
	case "task.completed":
		// Trophy eligible could trigger a sparkle effect on the trophy room.
		// Wired for future use — no-op until trophy ambient effects are implemented.

	// "task.stuck" — something went wrong. Play a confused/thinking animation.
	case "task.stuck":
		g.milo.SetState("thinking", g.currentRoomID, g.milo.facing, false)

	// "task.cancelled" — interrupted. Snap back to idle.
	case "task.cancelled":
		g.milo.SetState("idle", "main_hall", "s", true)
		g.currentRoomID = "main_hall"
		g.currentAnchor = "main_hall_center"
		g.scene.SetActiveRoom("main_hall")
	}
}
