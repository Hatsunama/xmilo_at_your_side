package game

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"xmilo/castle-go/internal/assets"
)

const (
	miloDrawScale  = 1.45
	thoughtBubbleW = 132
	thoughtBubbleH = 36
)

// MiloAnimator owns all of Milo's visual state.
// State transitions are driven exclusively by events received from the sidecar.
// The animator never decides what state Milo should be in — it only executes
// what it is told via SetState() and StartWalk().
type MiloAnimator struct {
	// sprite sheets indexed by "state_facing", e.g. "idle_s", "walking_e"
	sheets map[string]*ebiten.Image

	// current animation state
	state  string
	facing string
	loop   bool
	style  string

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
	thoughtText     string
	thoughtTicks    int
	thoughtMaxTicks int

	// z-order for depth sorting against room props
	ZOrder int
}

func NewMiloAnimator(cam *Camera) *MiloAnimator {
	m := &MiloAnimator{
		sheets:          make(map[string]*ebiten.Image),
		state:           "idle",
		facing:          "s",
		style:           "home_ready",
		loop:            true,
		ticksPerFrame:   6,   // 10fps animation at 60tps game loop
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
// Called in response to "milo.state_changed" events from the sidecar.
// sidecarState is one of "idle" | "moving" | "working".
// It is mapped to the visual animation state appropriate for the current room.
func (m *MiloAnimator) SetState(sidecarState, roomID, facing string, loop bool) {
	visualState := mapState(sidecarState, roomID)
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

func (m *MiloAnimator) SetIdleStyle(style string) {
	if style == "" {
		style = "home_ready"
	}
	m.style = style
}

// StartWalk begins a walk interpolation from current position to a target anchor.
// Called in response to "milo.movement_started" events.
// durationMS is the sidecar-specified walk duration (estimated_ms field).
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
	styleX, styleY, styleScale := idleStyleTransform(m.style, m.tickCount)
	op.GeoM.Translate(-float64(assets.MiloFrameW)/2, -float64(assets.MiloFrameH))
	op.GeoM.Scale(miloDrawScale*styleScale, miloDrawScale*styleScale)
	op.GeoM.Translate(m.screenX+styleX, m.screenY+styleY)
	screen.DrawImage(frame, op)

	// Thought bubble — simple white rectangle with text overlay
	// Replace with a proper nine-slice bubble sprite once art is ready
	if m.thoughtTicks > 0 && m.thoughtText != "" {
		drawThoughtBubble(screen, m.screenX, m.screenY, m.thoughtText, m.thoughtTicks, m.thoughtMaxTicks)
	}
}

// mapState translates the sidecar's behavioral state names into
// Milo's visual animation state for the current room.
// This is the only place where room-specific idle animations are chosen.
func mapState(sidecarState, roomID string) string {
	switch sidecarState {
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

func idleStyleTransform(style string, tick int) (x, y, scale float64) {
	scale = 1
	sway := math.Sin(float64(tick) / 14)
	bob := math.Sin(float64(tick) / 18)
	switch style {
	case "skeptical_shift":
		return -1.6 + sway*0.5, bob * 0.25, 1
	case "robe_settle":
		return 0.4 + sway*0.3, 0.6 + bob*0.35, 1
	case "attentive_wait":
		return 1.1 + sway*0.35, -0.8 + bob*0.25, 1.012
	case "hat_tilt":
		return 0.9 + sway*0.2, -1.0 + bob*0.2, 1.008
	case "outward_glance":
		return 1.4 + sway*0.3, -0.35 + bob*0.18, 1.01
	case "ready_shift":
		return -0.7 + sway*0.45, bob * 0.18, 1
	case "calm_archive":
		return sway * 0.18, 0.35 + bob*0.2, 1
	case "home_return":
		return sway * 0.12, 0.18 + bob*0.14, 1.008
	case "available_settle":
		return 0.2 + sway*0.16, 0.1 + bob*0.1, 1.004
	case "archive_notice":
		return -0.35 + sway*0.14, -0.2 + bob*0.12, 1.006
	case "quiet_reentry":
		return sway * 0.08, 0.28 + bob*0.12, 1
	case "observing":
		return 0.75 + sway*0.25, -0.75 + bob*0.22, 1.012
	case "archive_read":
		return -0.55 + sway*0.22, 0.15 + bob*0.16, 1
	case "study_focus":
		return -0.3 + sway*0.18, -0.45 + bob*0.12, 1.006
	case "note_check":
		return 0.26 + sway*0.16, -0.18 + bob*0.1, 1.004
	case "idea_pause":
		return sway * 0.1, -0.36 + bob*0.16, 1.01
	case "horizon_check":
		return 0.48 + sway*0.2, -0.72 + bob*0.16, 1.014
	case "sky_listen":
		return -0.18 + sway*0.14, -0.66 + bob*0.14, 1.012
	case "observatory_settle":
		return sway * 0.1, -0.3 + bob*0.12, 1.008
	case "proud":
		return sway * 0.12, -1.1 + bob*0.18, 1.022
	case "reflective_proud":
		return -0.5 + sway*0.15, -0.7 + bob*0.18, 1.012
	case "composed":
		return sway * 0.1, bob * 0.08, 1
	default:
		if tick%120 < 60 {
			return 0, 0, 1
		}
		return 0.4, 0, 1
	}
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

	bubble := ebiten.NewImage(thoughtBubbleW, thoughtBubbleH)
	bubble.Fill(color.RGBA{R: 34, G: 41, B: 64, A: uint8(210 * alpha)})
	inner := ebiten.NewImage(thoughtBubbleW-4, thoughtBubbleH-4)
	inner.Fill(color.RGBA{R: 247, G: 244, B: 234, A: uint8(235 * alpha)})

	bubbleOp := &ebiten.DrawImageOptions{}
	bubbleOp.GeoM.Translate(x-float64(thoughtBubbleW)/2, y-float64(assets.MiloFrameH)*miloDrawScale-42)
	screen.DrawImage(bubble, bubbleOp)

	innerOp := &ebiten.DrawImageOptions{}
	innerOp.GeoM.Translate(x-float64(thoughtBubbleW)/2+2, y-float64(assets.MiloFrameH)*miloDrawScale-40)
	screen.DrawImage(inner, innerOp)

	tail := ebiten.NewImage(14, 10)
	tail.Fill(color.RGBA{R: 247, G: 244, B: 234, A: uint8(235 * alpha)})
	tailBorder := ebiten.NewImage(18, 14)
	tailBorder.Fill(color.RGBA{R: 34, G: 41, B: 64, A: uint8(210 * alpha)})

	tailBorderOp := &ebiten.DrawImageOptions{}
	tailBorderOp.GeoM.Translate(x-9, y-float64(assets.MiloFrameH)*miloDrawScale-8)
	screen.DrawImage(tailBorder, tailBorderOp)

	tailOp := &ebiten.DrawImageOptions{}
	tailOp.GeoM.Translate(x-7, y-float64(assets.MiloFrameH)*miloDrawScale-6)
	screen.DrawImage(tail, tailOp)
}
