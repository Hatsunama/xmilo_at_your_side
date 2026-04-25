package game

import "github.com/hajimehoshi/ebiten/v2"

// Camera handles 2:1 isometric projection.
// Tile grid origin (0,0) maps to the top of the diamond on screen.
// X increases right-and-down (east), Y increases left-and-down (south).
// Z is elevation — each Z level raises the sprite by one tile height.
//
// Standard Sims-style 2:1 isometric projection:
//
//	screen_x = (gx - gy) * (tileW / 2) + offsetX
//	screen_y = (gx + gy) * (tileH / 2) - gz * tileH + offsetY
type Camera struct {
	TileW   int     // tile width in pixels (base: 128)
	TileH   int     // tile height in pixels (always TileW/2 for 2:1 iso)
	OffsetX float64 // screen X of grid origin (0,0) — typically screen center
	OffsetY float64 // screen Y of grid origin (0,0) — typically screen upper-third
	View    CameraViewState
	ScreenW int
	ScreenH int
}

type CameraViewState struct {
	PanX    float64
	PanY    float64
	Zoom    float64
	MinZoom float64
	MaxZoom float64
}

func NewCamera(screenW, screenH int) *Camera {
	tileW := 128
	return &Camera{
		TileW:   tileW,
		TileH:   tileW / 2,
		OffsetX: float64(screenW) / 2,
		OffsetY: float64(screenH) / 4,
		ScreenW: screenW,
		ScreenH: screenH,
		View: CameraViewState{
			Zoom:    1,
			MinZoom: 0.7,
			MaxZoom: 2.2,
		},
	}
}

// TileToScreen converts a grid coordinate to screen pixel position.
// Returns the screen position of the bottom-center of the tile — the
// baseline from which sprites are drawn upward.
func (c *Camera) TileToScreen(gx, gy, gz int) (float64, float64) {
	px := float64((gx-gy)*c.TileW/2) + c.OffsetX
	py := float64((gx+gy)*c.TileH/2) - float64(gz*c.TileH) + c.OffsetY
	return px, py
}

// AnchorToScreen looks up an anchor by ID and returns its screen position.
// Falls back to the camera origin if the anchor is unknown.
func (c *Camera) AnchorToScreen(anchorID string) (float64, float64) {
	coord, ok := anchorRegistry[anchorID]
	if !ok {
		return c.OffsetX, c.OffsetY
	}
	return c.TileToScreen(coord.X, coord.Y, coord.Z)
}

// ZOrder returns a sort key for painter's algorithm depth ordering.
// Sprites with higher ZOrder are drawn later (appear in front).
// For props at a fixed grid position, compute once at scene load.
// For Milo, recompute each frame from his current interpolated tile position.
func (c *Camera) ZOrder(gx, gy, gz int) int {
	return gx + gy + gz*1000
}

// MiloZOrder computes a continuous z-order from Milo's projected room position.
// Used for depth-sorting Milo against room props during movement.
func (c *Camera) MiloZOrderFromScreen(screenX, screenY float64) int {
	// Projected Y carries the painter-order sum (gx + gy) for floor-level sprites.
	relY := (screenY - c.OffsetY) / float64(c.TileH/2)
	return int(relY)
}

func (c *Camera) PanBy(dx, dy float64) {
	c.View.PanX += dx
	c.View.PanY += dy
}

func (c *Camera) ZoomAround(factor, focusX, focusY float64) {
	if factor == 0 {
		return
	}
	nextZoom := clamp(c.View.Zoom*factor, c.View.MinZoom, c.View.MaxZoom)
	if nextZoom == c.View.Zoom {
		return
	}
	worldX := (focusX - c.OffsetX - c.View.PanX) / c.View.Zoom
	worldY := (focusY - c.OffsetY - c.View.PanY) / c.View.Zoom
	c.View.Zoom = nextZoom
	c.View.PanX = focusX - c.OffsetX - worldX*c.View.Zoom
	c.View.PanY = focusY - c.OffsetY - worldY*c.View.Zoom
}

func (c *Camera) ClampViewToProjectedRect(minX, minY, maxX, maxY, padX, padY float64) {
	if c == nil || c.ScreenW <= 0 || c.ScreenH <= 0 {
		return
	}
	if minX > maxX {
		minX, maxX = maxX, minX
	}
	if minY > maxY {
		minY, maxY = maxY, minY
	}

	availableW := float64(c.ScreenW) - padX*2
	availableH := float64(c.ScreenH) - padY*2
	if availableW <= 0 || availableH <= 0 {
		return
	}

	rectW := maxX - minX
	rectH := maxY - minY
	maxZoom := c.View.MaxZoom
	if rectW > 0 {
		maxZoom = clamp(maxZoom, c.View.MinZoom, availableW/rectW)
	}
	if rectH > 0 {
		maxZoom = clamp(maxZoom, c.View.MinZoom, minFloat(maxZoom, availableH/rectH))
	}
	c.View.Zoom = clamp(c.View.Zoom, c.View.MinZoom, maxZoom)

	screenMinX := (minX-c.OffsetX)*c.View.Zoom + c.OffsetX
	screenMaxX := (maxX-c.OffsetX)*c.View.Zoom + c.OffsetX
	screenMinY := (minY-c.OffsetY)*c.View.Zoom + c.OffsetY
	screenMaxY := (maxY-c.OffsetY)*c.View.Zoom + c.OffsetY

	minPanX := padX - screenMinX
	maxPanX := float64(c.ScreenW) - padX - screenMaxX
	minPanY := padY - screenMinY
	maxPanY := float64(c.ScreenH) - padY - screenMaxY

	if minPanX > maxPanX {
		c.View.PanX = (minPanX + maxPanX) / 2
	} else {
		c.View.PanX = clamp(c.View.PanX, minPanX, maxPanX)
	}
	if minPanY > maxPanY {
		c.View.PanY = (minPanY + maxPanY) / 2
	} else {
		c.View.PanY = clamp(c.View.PanY, minPanY, maxPanY)
	}
}

func (c *Camera) ApplyView(geom *ebiten.GeoM) {
	if c == nil || geom == nil {
		return
	}
	geom.Translate(-c.OffsetX, -c.OffsetY)
	geom.Scale(c.View.Zoom, c.View.Zoom)
	geom.Translate(c.OffsetX+c.View.PanX, c.OffsetY+c.View.PanY)
}

func (c *Camera) ApplyToScreen(x, y float64) (float64, float64) {
	if c == nil {
		return x, y
	}
	x = (x-c.OffsetX)*c.View.Zoom + c.OffsetX + c.View.PanX
	y = (y-c.OffsetY)*c.View.Zoom + c.OffsetY + c.View.PanY
	return x, y
}

func (c *Camera) ResetView() {
	c.View.PanX = 0
	c.View.PanY = 0
	c.View.Zoom = 1
}

func clamp(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// anchorRegistry maps every anchor ID to its isometric grid coordinate.
// All nine Wizard Lair rooms represented. Extends as new rooms are added.
// X/Y are tile coordinates; Z is elevation (0 = floor level).
var anchorRegistry = map[string]AnchorCoord{
	// Main Hall — the default room, where Milo returns after every task
	"main_hall_center": {X: 8, Y: 8, Z: 0},
	"main_hall_door":   {X: 4, Y: 8, Z: 0},
	"main_hall_throne": {X: 12, Y: 5, Z: 2}, // raised dais
	"main_hall_left":   {X: 6, Y: 8, Z: 0},
	"main_hall_right":  {X: 10, Y: 8, Z: 0},
	"main_hall_front":  {X: 8, Y: 10, Z: 0},

	// War Room — planning / strategy tasks
	"war_room_table":     {X: 8, Y: 6, Z: 0},
	"war_room_map_wall":  {X: 13, Y: 4, Z: 0},
	"war_room_corner":    {X: 5, Y: 6, Z: 0},
	"war_room_toolbench": {X: 11, Y: 7, Z: 0},

	// Library — analysis / reading / summarizing tasks
	"library_desk":        {X: 6, Y: 10, Z: 0},
	"library_shelf_east":  {X: 14, Y: 6, Z: 0},
	"library_shelf_north": {X: 6, Y: 3, Z: 0},
	"library_window":      {X: 10, Y: 5, Z: 0},

	// Training Room — practice / learning tasks
	"training_circle": {X: 8, Y: 8, Z: 0},
	"training_dummy":  {X: 11, Y: 6, Z: 0},

	// Spellbook Room — spell research, creative tasks
	"spellbook_reading": {X: 7, Y: 8, Z: 0},
	"spellbook_shelf":   {X: 5, Y: 5, Z: 0},

	// Cauldron Room — processing, transformation tasks
	"cauldron_stir":   {X: 8, Y: 9, Z: 0},
	"cauldron_shelf":  {X: 5, Y: 12, Z: 0},
	"cauldron_table":  {X: 8, Y: 11, Z: 0},

	// Crystal Orb Room — prediction, analysis, vision tasks
	"crystal_orb_stand":  {X: 8, Y: 7, Z: 1}, // orb on a plinth
	"crystal_orb_watch":  {X: 10, Y: 9, Z: 0},
	"crystal_orb_rim":    {X: 6, Y: 8, Z: 0},
	"crystal_orb_window": {X: 12, Y: 5, Z: 0},

	// Baby Dragon Room — companion, warmth, casual conversation
	"dragon_perch":    {X: 10, Y: 6, Z: 3}, // dragon on high perch
	"dragon_play_mat": {X: 7, Y: 9, Z: 0},

	// Trophy Room — completed task celebration
	"trophy_display":  {X: 8, Y: 7, Z: 0},
	"trophy_pedestal": {X: 11, Y: 5, Z: 1},
	"trophy_wall":     {X: 5, Y: 6, Z: 0},

	// Archive Room — history, logs, memory retrieval
	"archive_lectern":    {X: 8, Y: 10, Z: 0},
	"archive_shelf":      {X: 5, Y: 7, Z: 0},
	"archive_crystal":    {X: 8, Y: 5, Z: 1},
	"archive_shelf_east": {X: 12, Y: 7, Z: 0},
}

// AnchorCoord is a position in the isometric tile grid.
type AnchorCoord struct {
	X, Y, Z int
}

// WalkDurationMS returns the walk animation duration in milliseconds
// between two anchors at 4 tiles per second (250ms per tile, Manhattan distance).
func WalkDurationMS(fromID, toID string) int {
	from, okF := anchorRegistry[fromID]
	to, okT := anchorRegistry[toID]
	if !okF || !okT {
		return 800 // fallback
	}
	dist := iabs(to.X-from.X) + iabs(to.Y-from.Y)
	if dist == 0 {
		return 0
	}
	return dist * 250
}

// WalkFacing returns the cardinal direction Milo faces when walking
// from one anchor to another. Used to select the correct sprite sheet row.
func WalkFacing(fromID, toID string) string {
	from, okF := anchorRegistry[fromID]
	to, okT := anchorRegistry[toID]
	if !okF || !okT {
		return "s"
	}
	dx := to.X - from.X
	dy := to.Y - from.Y
	if iabs(dx) >= iabs(dy) {
		if dx >= 0 {
			return "e"
		}
		return "w"
	}
	if dy >= 0 {
		return "s"
	}
	return "n"
}

func iabs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
