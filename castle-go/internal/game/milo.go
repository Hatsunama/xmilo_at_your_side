package game

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"xmilo/castle-go/internal/assets"
)

// MiloAnimator owns all of Milo's visual state.
// State transitions are driven exclusively by events received from PicoClaw.
// The animator never decides what state Milo should be in — it only executes
// what it is told via SetState() and StartWalk().
type MiloAnimator struct {
	// sprite sheets indexed by "state_facing", e.g. "idle_s", "walking_e"
	sheets map[string]*ebiten.Image

	// current animation state
	state  string
	facing string
	loop   bool

	// frame advance
	frame         int
	tickCount     int
	ticksPerFrame int // 60tps / ticksPerFrame = animation fps

	// screen position (bottom-center of Milo's feet)
	screenX float64
	screenY float64

	// walk interpolation — used during "milo.movement_started" events
	walkActive   bool
	walkFromX    float64
	walkFromY    float64
	walkToX      float64
	walkToY      float64
	walkProgress float64 // 0.0 → 1.0
	walkTicks    float64 // total ticks for this walk

	// thought bubble — shown briefly after "milo.thought" events
	thoughtText    string
	thoughtTicks   int
	thoughtMaxTicks int

	// z-order for depth sorting against room props
	ZOrder int
}

func NewMiloAnimator(cam *Camera) *MiloAnimator {
	m := &MiloAnimator{
		sheets:          make(map[string]*ebiten.Image),
		state:           "idle",
		facing:          "s",
		loop:            true,
		ticksPerFrame:   6, // 10fps animation at 60tps game loop
		thoughtMaxTicks: 180, // 3 seconds at 60tps
	}

	states := []string{
		"idle", "walking", "talking", "thinking", "sleeping",
		"working", "reading", "stirring", "gazing",
	}
	facings := []string{"n", "s", "e", "w"}
	for _, s := range states {
		for _, f := range facings {
			key := s + "_" + f
			m.sheets[key] = assets.LoadMiloSheet(s, f)
		}
	}

	// Start Milo at main_hall_center
	x, y := cam.AnchorToScreen("main_hall_center")
	m.screenX = x
	m.screenY = y
	m.ZOrder = cam.MiloZOrderFromScreen(x, y)

	return m
}

// SetState changes Milo's animation state and facing.
// Called in response to "milo.state_changed" events from PicoClaw.
// picoClawState is one of "idle" | "moving" | "working".
// It is mapped to the visual animation state appropriate for the current room.
func (m *MiloAnimator) SetState(picoClawState, roomID, facing string, loop bool) {
	visualState := mapState(picoClawState, roomID)
	if visualState == m.state && facing == m.facing {
		return // no redundant sheet swap
	}
	m.state = visualState
	if facing != "" {
		m.facing = facing
	}
	m.loop = loop
	m.frame = 0
	m.tickCount = 0
}

// StartWalk begins a walk interpolation from current position to a target anchor.
// Called in response to "milo.movement_started" events.
// durationMS is the PicoClaw-specified walk duration (estimated_ms field).
func (m *MiloAnimator) StartWalk(toX, toY float64, facing string, durationMS int) {
	if durationMS <= 0 {
		m.screenX = toX
		m.screenY = toY
		return
	}
	m.walkActive = true
	m.walkFromX = m.screenX
	m.walkFromY = m.screenY
	m.walkToX = toX
	m.walkToY = toY
	m.walkProgress = 0
	m.walkTicks = float64(durationMS) / (1000.0 / 60.0) // ms → ticks at 60tps
	m.facing = facing
	m.state = "walking"
	m.frame = 0
	m.tickCount = 0
}

// ShowThought displays a thought bubble above Milo for 3 seconds.
// Called in response to "milo.thought" events.
func (m *MiloAnimator) ShowThought(text string) {
	m.thoughtText = text
	m.thoughtTicks = m.thoughtMaxTicks
}

// Tick advances the animator by one game frame. Called from Game.Update().
func (m *MiloAnimator) Tick(cam *Camera) {
	// Walk interpolation
	if m.walkActive {
		m.walkProgress += 1.0 / m.walkTicks
		if m.walkProgress >= 1.0 {
			m.walkProgress = 1.0
			m.walkActive = false
		}
		t := easeInOut(m.walkProgress)
		m.screenX = lerp(m.walkFromX, m.walkToX, t)
		m.screenY = lerp(m.walkFromY, m.walkToY, t)
		m.ZOrder = cam.MiloZOrderFromScreen(m.screenX, m.screenY)
	}

	// Thought bubble countdown
	if m.thoughtTicks > 0 {
		m.thoughtTicks--
	}

	// Sprite frame advance
	m.tickCount++
	if m.tickCount >= m.ticksPerFrame {
		m.tickCount = 0
		m.frame++
		if m.frame >= assets.MiloFrames {
			if m.loop {
				m.frame = 0
			} else {
				m.frame = assets.MiloFrames - 1
			}
		}
	}
}

// Draw renders Milo and (if active) his thought bubble onto the screen.
func (m *MiloAnimator) Draw(screen *ebiten.Image) {
	key := m.state + "_" + m.facing
	sheet, ok := m.sheets[key]
	if !ok {
		key = "idle_s"
		sheet = m.sheets[key]
	}

	// Extract the current frame from the horizontal sprite sheet
	fx := m.frame * assets.MiloFrameW
	frameRect := image.Rect(fx, 0, fx+assets.MiloFrameW, assets.MiloFrameH)
	frame := sheet.SubImage(frameRect).(*ebiten.Image)

	op := &ebiten.DrawImageOptions{}
	// Anchor is the bottom-center of the sprite (Milo's feet)
	op.GeoM.Translate(-float64(assets.MiloFrameW)/2, -float64(assets.MiloFrameH))
	op.GeoM.Translate(m.screenX, m.screenY)
	screen.DrawImage(frame, op)

	// Thought bubble — simple white rectangle with text overlay
	// Replace with a proper nine-slice bubble sprite once art is ready
	if m.thoughtTicks > 0 && m.thoughtText != "" {
		drawThoughtBubble(screen, m.screenX, m.screenY, m.thoughtText, m.thoughtTicks, m.thoughtMaxTicks)
	}
}

// mapState translates PicoClaw's behavioral state names into
// Milo's visual animation state for the current room.
// This is the only place where room-specific idle animations are chosen.
func mapState(picoState, roomID string) string {
	switch picoState {
	case "moving":
		return "walking"
	case "working":
		switch roomID {
		case "library", "spellbook", "archive":
			return "reading"
		case "cauldron":
			return "stirring"
		case "crystal_orb":
			return "gazing"
		case "baby_dragon":
			return "working" // playing with dragon
		default:
			return "working"
		}
	case "idle":
		return "idle"
	default:
		return "idle"
	}
}

func lerp(a, b, t float64) float64 { return a + (b-a)*t }

func easeInOut(t float64) float64 {
	// Smoothstep — matches the Sims character movement feel
	return t * t * (3 - 2*t)
}

// drawThoughtBubble is a placeholder text renderer.
// Replace the rectangle with a proper sprite-based speech bubble
// once art assets are available.
func drawThoughtBubble(screen *ebiten.Image, x, y float64, text string, ticks, maxTicks int) {
	// Fade out over last 30 ticks
	alpha := float64(1)
	if ticks < 30 {
		alpha = float64(ticks) / 30.0
	}
	_ = alpha // used when real bubble sprite compositing is added
	_ = text

	// placeholder: small white square above Milo's head
	bubble := ebiten.NewImage(120, 32)
	bubble.Fill(color.White)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(x-60, y-float64(assets.MiloFrameH)-40)
	screen.DrawImage(bubble, op)
}
