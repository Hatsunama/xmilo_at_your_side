package game

import (
	"image/color"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
	"xmilo/castle-go/internal/assets"
)

// colorWhite is a 1x1 white image used as a drawing primitive.
// It must be created lazily after the Ebiten runtime is ready on Android.
var colorWhite *ebiten.Image

func ensureDrawPrimitives() {
	if colorWhite != nil {
		return
	}
	colorWhite = ebiten.NewImage(1, 1)
	colorWhite.Fill(color.White)
}

// Prop is a single room decoration at a fixed grid position.
// Props with an AmbientKey can be activated by "ambient" events from PicoClaw.
type Prop struct {
	SpriteKey    string
	GridX, GridY int
	GridZ        int
	ZOrder       int // precomputed at scene load
	sprite       *ebiten.Image
	screenX      float64
	screenY      float64
	// ambient animation
	AmbientKey    string // matches AmbientEvent.EffectID, empty if no ambient
	ambientActive bool
	ambientFrame  int
	ambientTick   int
	ambientFPS    int // frames per second for the ambient cycle (default 8)
}

// Room holds the visual representation of one Wizard Lair room.
type Room struct {
	ID         string
	background *ebiten.Image
	props      []*Prop // sorted by ZOrder ascending for painter's algorithm
}

// RoomScene manages all nine rooms and renders the active one.
type RoomScene struct {
	rooms        map[string]*Room
	activeID     string
	cam          *Camera
	tickCount    int
	ritualStatus string
	ritualPulse  int
	miloState    string
	moveIntent   *movementIntent
	route        []RouteStep
}

type movementIntent struct {
	toRoom   string
	toAnchor string
	reason   string
	targetX  float64
	targetY  float64
}

func NewRoomScene(cam *Camera) *RoomScene {
	rs := &RoomScene{
		rooms:     make(map[string]*Room),
		activeID:  "main_hall",
		cam:       cam,
		miloState: "idle",
	}
	rs.buildRooms()
	return rs
}

// SetActiveRoom switches the visible room. Called on "milo.room_changed" events.
func (rs *RoomScene) SetActiveRoom(roomID string) {
	sceneRoomID := SceneRoomID(roomID)
	if _, ok := rs.rooms[sceneRoomID]; ok {
		rs.activeID = sceneRoomID
	}
}

func (rs *RoomScene) SetMiloState(state string) {
	rs.miloState = state
}

func (rs *RoomScene) SetMovementIntent(toRoom, toAnchor, reason string) {
	targetX, targetY := rs.cam.AnchorToScreen(toAnchor)
	rs.moveIntent = &movementIntent{
		toRoom:   toRoom,
		toAnchor: toAnchor,
		reason:   reason,
		targetX:  targetX,
		targetY:  targetY,
	}
}

func (rs *RoomScene) ClearMovementIntent() {
	rs.moveIntent = nil
}

func (rs *RoomScene) SetRoute(route []RouteStep) {
	rs.route = append([]RouteStep(nil), route...)
}

// TriggerAmbient activates an ambient prop effect by effect ID.
// Called on "ambient" events — currently PicoClaw does not emit these yet,
// but the handler is wired so adding them to the engine requires zero renderer changes.
func (rs *RoomScene) TriggerAmbient(effectID string) {
	room, ok := rs.rooms[rs.activeID]
	if !ok {
		return
	}
	for _, p := range room.props {
		if p.AmbientKey == effectID {
			p.ambientActive = true
			p.ambientFrame = 0
			p.ambientTick = 0
		}
	}
}

// Tick advances prop ambient animations. Called from Game.Update() every frame.
func (rs *RoomScene) Tick() {
	rs.tickCount++
	rs.ritualPulse++
	room, ok := rs.rooms[rs.activeID]
	if !ok {
		return
	}
	for _, p := range room.props {
		if !p.ambientActive {
			continue
		}
		fps := p.ambientFPS
		if fps == 0 {
			fps = 8
		}
		ticksPerFrame := 60 / fps
		p.ambientTick++
		if p.ambientTick >= ticksPerFrame {
			p.ambientTick = 0
			p.ambientFrame++
			// ambient effects loop indefinitely once triggered
		}
	}
}

func (rs *RoomScene) SetRitualState(status string) {
	rs.ritualStatus = status
	rs.ritualPulse = 0
}

// Draw renders the active room background and all its props in z-order.
// Milo is NOT drawn here — he is drawn by the Game after receiving
// a ZOrder value from the animator so he can be interleaved with props.
func (rs *RoomScene) Draw(screen *ebiten.Image, miloZ int, drawMilo func()) {
	room, ok := rs.rooms[rs.activeID]
	if !ok {
		return
	}

	// Background — fills the screen, drawn first (behind everything)
	bgOp := &ebiten.DrawImageOptions{}
	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
	bgW, bgH := room.background.Bounds().Dx(), room.background.Bounds().Dy()
	bgOp.GeoM.Scale(float64(sw)/float64(bgW), float64(sh)/float64(bgH))
	screen.DrawImage(room.background, bgOp)
	rs.drawRoomMoodOverlay(screen)
	rs.drawRouteReveal(screen)
	rs.drawMovementIntent(screen)
	rs.drawRitualOverlay(screen)

	// Collect all props + Milo into a z-sorted draw list
	type drawable struct {
		z    int
		draw func()
	}
	var drawables []drawable
	drawables = append(drawables, drawable{z: miloZ, draw: drawMilo})
	for _, p := range room.props {
		prop := p // capture
		drawables = append(drawables, drawable{
			z: prop.ZOrder,
			draw: func() {
				rs.drawProp(screen, prop)
			},
		})
	}

	sort.Slice(drawables, func(i, j int) bool {
		return drawables[i].z < drawables[j].z
	})
	for _, d := range drawables {
		d.draw()
	}
}

func (rs *RoomScene) drawRouteReveal(screen *ebiten.Image) {
	if len(rs.route) == 0 {
		return
	}

	requiresHall := false
	variant := ""
	for _, step := range rs.route {
		if step.ViaHall {
			requiresHall = true
			if step.Variant != "" {
				variant = step.Variant
			}
			break
		}
	}
	if !requiresHall {
		return
	}

	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
	accent, inner, shadow := routeRevealPalette(rs.route)
	pulse := uint8(20 + (rs.tickCount % 20))
	warmStone := color.RGBA{R: 84, G: 73, B: 66, A: pulse}
	warmStoneInner := color.RGBA{R: 118, G: 103, B: 92, A: pulse / 2}
	sconceGlow := color.RGBA{R: 239, G: 190, B: 103, A: pulse + 10}
	centerX := sw / 2
	pathTop := sh/2 - 12
	pathHeight := 200
	accentWidth := 48
	innerWidth := 18
	sconceOffset := 74

	switch variant {
	case "window_arc", "forge_turn", "service_pass":
		centerX -= 28
		accentWidth = 54
		sconceOffset = 92
	case "observatory_glow", "copper_arc", "lantern_bridge":
		centerX += 26
		accentWidth = 42
		innerWidth = 22
		sconceOffset = 88
	}

	drawTintRect(screen, centerX-54, sh/2-18, 108, 212, color.RGBA{R: shadow.R, G: shadow.G, B: shadow.B, A: pulse})
	drawTintRect(screen, centerX-48, pathTop, 96, pathHeight, warmStone)
	drawTintRect(screen, centerX-38, pathTop, 76, pathHeight, warmStoneInner)
	drawTintRect(screen, centerX-accentWidth/2, pathTop, accentWidth, pathHeight, color.RGBA{R: accent.R, G: accent.G, B: accent.B, A: pulse + 8})
	drawTintRect(screen, centerX-innerWidth/2, pathTop, innerWidth, pathHeight, color.RGBA{R: inner.R, G: inner.G, B: inner.B, A: pulse + 16})
	drawTintRect(screen, centerX-70, sh/2+54, 140, 34, color.RGBA{R: shadow.R, G: shadow.G, B: shadow.B, A: pulse / 2})
	drawTintRect(screen, centerX-48, sh/2+62, 96, 18, color.RGBA{R: accent.R, G: accent.G, B: accent.B, A: pulse / 2})

	switch variant {
	case "window_arc":
		drawTintRect(screen, centerX-116, sh/2-18, 18, 78, color.RGBA{R: 236, G: 216, B: 156, A: pulse + 12})
		drawTintRect(screen, centerX+98, sh/2-18, 18, 78, color.RGBA{R: 236, G: 216, B: 156, A: pulse + 12})
	case "observatory_glow", "archive_sconces":
		drawTintRect(screen, centerX-32, sh/2-26, 64, 24, color.RGBA{R: inner.R, G: inner.G, B: inner.B, A: pulse + 10})
	case "ember_sconces", "forge_turn", "copper_arc":
		sconceGlow = color.RGBA{R: 244, G: 172, B: 89, A: pulse + 10}
	case "lantern_bridge":
		sconceGlow = color.RGBA{R: 210, G: 226, B: 255, A: pulse + 10}
	}

	drawTintRect(screen, centerX-sconceOffset-8, sh/2+6, 16, 28, sconceGlow)
	drawTintRect(screen, centerX+sconceOffset-8, sh/2+6, 16, 28, sconceGlow)
	drawTintRect(screen, centerX-sconceOffset-4, sh/2+10, 8, 18, color.RGBA{R: 247, G: 226, B: 170, A: pulse + 20})
	drawTintRect(screen, centerX+sconceOffset-4, sh/2+10, 8, 18, color.RGBA{R: 247, G: 226, B: 170, A: pulse + 20})
}

func routeRevealPalette(route []RouteStep) (accent, inner, shadow color.RGBA) {
	if len(route) == 0 {
		return color.RGBA{R: 130, G: 162, B: 214, A: 255}, color.RGBA{R: 221, G: 233, B: 255, A: 255}, color.RGBA{R: 70, G: 88, B: 118, A: 255}
	}
	destination := route[len(route)-1].Room
	switch roomTopologies[destination].Cluster {
	case "knowledge":
		return color.RGBA{R: 126, G: 185, B: 234, A: 255}, color.RGBA{R: 226, G: 241, B: 255, A: 255}, color.RGBA{R: 66, G: 91, B: 126, A: 255}
	case "making":
		return color.RGBA{R: 214, G: 162, B: 106, A: 255}, color.RGBA{R: 255, G: 236, B: 206, A: 255}, color.RGBA{R: 110, G: 78, B: 48, A: 255}
	default:
		return color.RGBA{R: 170, G: 154, B: 232, A: 255}, color.RGBA{R: 242, G: 236, B: 255, A: 255}, color.RGBA{R: 84, G: 72, B: 128, A: 255}
	}
}

func (rs *RoomScene) drawRitualOverlay(screen *ebiten.Image) {
	if rs.ritualStatus == "" {
		return
	}

	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
	var tint color.RGBA
	var accent color.RGBA

	switch rs.ritualStatus {
	case "deferred":
		tint = color.RGBA{R: 46, G: 31, B: 74, A: 44}
		accent = color.RGBA{R: 175, G: 143, B: 255, A: 80}
	case "started":
		tint = color.RGBA{R: 23, G: 41, B: 74, A: 56}
		accent = color.RGBA{R: 110, G: 182, B: 255, A: 96}
	case "completed":
		tint = color.RGBA{R: 21, G: 47, B: 33, A: 52}
		accent = color.RGBA{R: 125, G: 227, B: 170, A: 84}
	default:
		return
	}

	drawTintRect(screen, 0, 0, sw, sh, tint)
	pulseW := 120 + (rs.ritualPulse%120)*2
	pulseH := 80 + (rs.ritualPulse % 90)
	drawTintRect(screen, sw/2-pulseW/2, sh/5, pulseW, pulseH, accent)
	drawTintRect(screen, sw/2-18, sh/5+18, 36, 36, accent)
	if rs.activeID == "archive" || rs.activeID == "crystal_orb" {
		drawTintRect(screen, sw/2-54, sh/2-8, 108, 74, color.RGBA{R: accent.R, G: accent.G, B: accent.B, A: accent.A / 2})
		drawTintRect(screen, sw/2-18, sh/2+12, 36, 36, color.RGBA{R: 236, G: 244, B: 255, A: accent.A / 2})
	}
}

func (rs *RoomScene) drawRoomMoodOverlay(screen *ebiten.Image) {
	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()

	switch rs.activeID {
	case "main_hall":
		windowAlpha := uint8(18 + (rs.tickCount % 48))
		drawTintRect(screen, 90, 130, 30, 72, color.RGBA{R: 228, G: 216, B: 159, A: windowAlpha})
		drawTintRect(screen, sw-120, 130, 30, 72, color.RGBA{R: 228, G: 216, B: 159, A: windowAlpha})
		drawTintRect(screen, sw/2-100, 64, 200, 180, color.RGBA{R: 140, G: 120, B: 215, A: 22})
		drawTintRect(screen, sw/2-56, sh/2+10, 112, 154, color.RGBA{R: 67, G: 56, B: 102, A: 18})
		if rs.miloState == "idle" {
			drawTintRect(screen, sw/2-72, sh/2+58, 144, 48, color.RGBA{R: 255, G: 190, B: 115, A: 28})
		}
		if rs.miloState == "working" {
			drawTintRect(screen, sw/2-126, 84, 252, 128, color.RGBA{R: 114, G: 164, B: 255, A: 24})
		}
	case "archive":
		if rs.miloState == "working" || rs.ritualStatus != "" {
			drawTintRect(screen, sw/2-110, sh/3, 220, 140, color.RGBA{R: 123, G: 173, B: 255, A: 28})
			drawTintRect(screen, sw/2-64, sh/2-18, 128, 84, color.RGBA{R: 169, G: 201, B: 255, A: 18})
			drawTintRect(screen, sw/2-42, sh/2+68, 84, 160, color.RGBA{R: 68, G: 87, B: 132, A: 18})
			pulse := uint8(20 + (rs.tickCount % 30))
			drawTintRect(screen, sw/2-26, 146, 52, 52, color.RGBA{R: 196, G: 225, B: 255, A: pulse})
		}
	case "crystal_orb":
		if rs.miloState == "working" || rs.ritualStatus == "started" {
			drawTintRect(screen, sw/2-96, sh/3-12, 192, 128, color.RGBA{R: 102, G: 192, B: 255, A: 30})
			drawTintRect(screen, sw/2-42, sh/2+66, 84, 148, color.RGBA{R: 40, G: 90, B: 124, A: 18})
			pulse := uint8(18 + (rs.tickCount % 42))
			drawTintRect(screen, sw/2-32, 126, 64, 64, color.RGBA{R: 171, G: 230, B: 255, A: pulse})
		}
	case "library":
		drawTintRect(screen, sw/2-62, sh/2+26, 124, 160, color.RGBA{R: 78, G: 64, B: 36, A: 16})
		if rs.miloState == "working" {
			drawTintRect(screen, sw/2-120, sh/3, 240, 110, color.RGBA{R: 191, G: 162, B: 103, A: 24})
			drawTintRect(screen, sw/2-72, sh/2-4, 144, 92, color.RGBA{R: 222, G: 194, B: 128, A: 16})
		}
	}
}

func (rs *RoomScene) drawProp(screen *ebiten.Image, prop *Prop) {
	op := &ebiten.DrawImageOptions{}
	x := prop.screenX - float64(prop.sprite.Bounds().Dx())/2
	y := prop.screenY - float64(prop.sprite.Bounds().Dy())

	switch prop.AmbientKey {
	case "hearth_glow", "fire_glow":
		pulse := 0.92 + float64((rs.tickCount%18))/100
		op.ColorScale.Scale(float32(1.0), float32(0.9+pulse/10), float32(0.78), float32(1.0))
		op.GeoM.Scale(1, pulse)
		y -= 6 * (pulse - 0.92)
	case "lamp_flicker", "candle_flicker":
		alpha := 0.86 + float64((rs.tickCount%14))/100
		op.ColorScale.Scale(float32(1.0), float32(0.98), float32(0.86), float32(alpha))
	case "orb_pulse", "memory_pulse", "rune_glow":
		pulse := 0.9 + float64((rs.tickCount%24))/80
		op.ColorScale.Scale(float32(0.88+pulse/8), float32(0.95+pulse/12), float32(1.0), float32(0.92))
		op.GeoM.Scale(pulse, pulse)
		x -= (float64(prop.sprite.Bounds().Dx()) * (pulse - 1)) / 2
		y -= (float64(prop.sprite.Bounds().Dy()) * (pulse - 1)) / 2
	}

	op.GeoM.Translate(x, y)
	screen.DrawImage(prop.sprite, op)
}

func (rs *RoomScene) drawMovementIntent(screen *ebiten.Image) {
	if rs.moveIntent == nil {
		return
	}

	size := 16 + (rs.tickCount%24)/2
	alpha := uint8(52 + (rs.tickCount%30)*2)
	highlight := color.RGBA{R: 166, G: 208, B: 255, A: alpha}
	if rs.moveIntent.toRoom == "main_hall" {
		highlight = color.RGBA{R: 206, G: 184, B: 255, A: alpha}
	}
	if rs.moveIntent.reason == "report" {
		highlight = color.RGBA{R: 151, G: 236, B: 187, A: alpha}
	}

	drawTintRect(
		screen,
		int(rs.moveIntent.targetX)-size,
		int(rs.moveIntent.targetY)-size*2,
		size*2,
		size*2,
		highlight,
	)
	drawTintRect(
		screen,
		int(rs.moveIntent.targetX)-3,
		int(rs.moveIntent.targetY)-size*2+6,
		6,
		size*2-12,
		color.RGBA{R: 244, G: 246, B: 255, A: alpha / 2},
	)
}

// buildRooms constructs the current renderer-visible room set.
// Canon room names are handled through topology aliases so the scene can stay
// future-ready while launch art still uses a bounded subset of room assets.
func (rs *RoomScene) buildRooms() {
	rooms := []struct {
		id    string
		props []propDef
	}{
		{
			id: "main_hall",
			props: []propDef{
				{key: "throne", gx: 12, gy: 5, gz: 2, ambient: ""},
				{key: "banner_left", gx: 3, gy: 6, gz: 0, ambient: ""},
				{key: "banner_right", gx: 13, gy: 6, gz: 0, ambient: ""},
				{key: "fireplace", gx: 8, gy: 3, gz: 0, ambient: "hearth_glow"},
			},
		},
		{
			id: "war_room",
			props: []propDef{
				{key: "strategy_table", gx: 8, gy: 6, gz: 0, ambient: ""},
				{key: "map_wall", gx: 13, gy: 4, gz: 0, ambient: ""},
				{key: "flag_left", gx: 3, gy: 5, gz: 0, ambient: ""},
				{key: "flag_right", gx: 13, gy: 8, gz: 0, ambient: ""},
			},
		},
		{
			id: "library",
			props: []propDef{
				{key: "reading_desk", gx: 6, gy: 10, gz: 0, ambient: ""},
				{key: "bookshelf_east", gx: 14, gy: 6, gz: 0, ambient: ""},
				{key: "bookshelf_north", gx: 6, gy: 3, gz: 0, ambient: ""},
				{key: "reading_lamp", gx: 7, gy: 9, gz: 0, ambient: "lamp_flicker"},
			},
		},
		{
			id: "training_room",
			props: []propDef{
				{key: "training_dummy", gx: 11, gy: 6, gz: 0, ambient: ""},
				{key: "weapon_rack", gx: 4, gy: 5, gz: 0, ambient: ""},
				{key: "training_mat", gx: 8, gy: 8, gz: 0, ambient: ""},
			},
		},
		{
			id: "spellbook",
			props: []propDef{
				{key: "spellbook_stand", gx: 7, gy: 8, gz: 0, ambient: "page_turn"},
				{key: "scroll_shelf", gx: 5, gy: 5, gz: 0, ambient: ""},
				{key: "inkwell", gx: 9, gy: 9, gz: 0, ambient: ""},
				{key: "candle_cluster", gx: 11, gy: 6, gz: 0, ambient: "candle_flicker"},
			},
		},
		{
			id: "cauldron",
			props: []propDef{
				{key: "cauldron", gx: 8, gy: 9, gz: 0, ambient: "cauldron_bubble"},
				{key: "ingredient_shelf", gx: 5, gy: 12, gz: 0, ambient: ""},
				{key: "fire_pit", gx: 8, gy: 11, gz: 0, ambient: "fire_glow"},
				{key: "potion_rack", gx: 12, gy: 7, gz: 0, ambient: ""},
			},
		},
		{
			id: "crystal_orb",
			props: []propDef{
				{key: "orb_plinth", gx: 8, gy: 7, gz: 0, ambient: ""},
				{key: "crystal_orb", gx: 8, gy: 7, gz: 1, ambient: "orb_pulse"},
				{key: "star_map", gx: 13, gy: 5, gz: 0, ambient: "star_twinkle"},
				{key: "rune_circle", gx: 8, gy: 8, gz: 0, ambient: "rune_glow"},
			},
		},
		{
			id: "baby_dragon",
			props: []propDef{
				{key: "dragon_perch", gx: 10, gy: 6, gz: 2, ambient: ""},
				{key: "dragon", gx: 10, gy: 6, gz: 3, ambient: "dragon_yawn"},
				{key: "toy_pile", gx: 6, gy: 10, gz: 0, ambient: ""},
				{key: "gem_hoard", gx: 13, gy: 8, gz: 0, ambient: "gem_sparkle"},
			},
		},
		{
			id: "trophy",
			props: []propDef{
				{key: "trophy_case", gx: 8, gy: 7, gz: 0, ambient: "trophy_glow"},
				{key: "trophy_pedestal", gx: 11, gy: 5, gz: 1, ambient: ""},
				{key: "victory_banner", gx: 4, gy: 4, gz: 0, ambient: ""},
				{key: "achievement_wall", gx: 14, gy: 6, gz: 0, ambient: ""},
			},
		},
		{
			id: "archive",
			props: []propDef{
				{key: "archive_lectern", gx: 8, gy: 10, gz: 0, ambient: ""},
				{key: "archive_shelf_a", gx: 5, gy: 7, gz: 0, ambient: ""},
				{key: "archive_shelf_b", gx: 12, gy: 7, gz: 0, ambient: ""},
				{key: "memory_crystal", gx: 8, gy: 5, gz: 1, ambient: "memory_pulse"},
			},
		},
	}

	for _, rd := range rooms {
		room := &Room{
			ID:         rd.id,
			background: assets.LoadRoomBackground(rd.id),
		}
		for _, pd := range rd.props {
			sx, sy := rs.cam.TileToScreen(pd.gx, pd.gy, pd.gz)
			prop := &Prop{
				SpriteKey:  pd.key,
				GridX:      pd.gx,
				GridY:      pd.gy,
				GridZ:      pd.gz,
				ZOrder:     rs.cam.ZOrder(pd.gx, pd.gy, pd.gz),
				sprite:     assets.LoadPropSprite(pd.key),
				screenX:    sx,
				screenY:    sy,
				AmbientKey: pd.ambient,
				ambientFPS: 8,
			}
			room.props = append(room.props, prop)
		}
		// Pre-sort props by z-order once at load time
		sort.Slice(room.props, func(i, j int) bool {
			return room.props[i].ZOrder < room.props[j].ZOrder
		})
		rs.rooms[rd.id] = room
	}
}

type propDef struct {
	key     string
	gx, gy  int
	gz      int
	ambient string
}

func drawTintRect(screen *ebiten.Image, x, y, w, h int, c color.RGBA) {
	if w <= 0 || h <= 0 {
		return
	}
	ensureDrawPrimitives()
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(float64(w), float64(h))
	op.GeoM.Translate(float64(x), float64(y))
	op.ColorScale.Scale(float32(c.R)/255, float32(c.G)/255, float32(c.B)/255, float32(c.A)/255)
	screen.DrawImage(colorWhite, op)
}
