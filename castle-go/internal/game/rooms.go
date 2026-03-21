package game

import (
	"image/color"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
	"xmilo/castle-go/internal/assets"
)

// colorWhite is a 1x1 white image used as a drawing primitive.
// Initialized in init() after the Ebiten runtime is ready.
var colorWhite *ebiten.Image

func init() {
	colorWhite = ebiten.NewImage(1, 1)
	colorWhite.Fill(color.White)
}

// Prop is a single room decoration at a fixed grid position.
// Props with an AmbientKey can be activated by "ambient" events from PicoClaw.
type Prop struct {
	SpriteKey     string
	GridX, GridY  int
	GridZ         int
	ZOrder        int // precomputed at scene load
	sprite        *ebiten.Image
	screenX       float64
	screenY       float64
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
	rooms     map[string]*Room
	activeID  string
	cam       *Camera
	tickCount int
}

func NewRoomScene(cam *Camera) *RoomScene {
	rs := &RoomScene{
		rooms:    make(map[string]*Room),
		activeID: "main_hall",
		cam:      cam,
	}
	rs.buildRooms()
	return rs
}

// SetActiveRoom switches the visible room. Called on "milo.room_changed" events.
func (rs *RoomScene) SetActiveRoom(roomID string) {
	if _, ok := rs.rooms[roomID]; ok {
		rs.activeID = roomID
	}
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
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Translate(prop.screenX-float64(prop.sprite.Bounds().Dx())/2,
					prop.screenY-float64(prop.sprite.Bounds().Dy()))
				screen.DrawImage(prop.sprite, op)
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

// buildRooms constructs all nine Wizard Lair rooms.
// Room backgrounds are loaded from the assets package (placeholder until art lands).
// Props are placed at fixed grid positions defined in camera.go's anchorRegistry.
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
