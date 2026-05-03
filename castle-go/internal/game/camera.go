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

type WorldBounds struct {
	MinX float64
	MinY float64
	MaxX float64
	MaxY float64
}

func (b WorldBounds) Normalized() WorldBounds {
	if b.MinX > b.MaxX {
		b.MinX, b.MaxX = b.MaxX, b.MinX
	}
	if b.MinY > b.MaxY {
		b.MinY, b.MaxY = b.MaxY, b.MinY
	}
	return b
}

func (b WorldBounds) Width() float64 {
	b = b.Normalized()
	return b.MaxX - b.MinX
}

func (b WorldBounds) Height() float64 {
	b = b.Normalized()
	return b.MaxY - b.MinY
}

func (b WorldBounds) Valid() bool {
	return b.Width() > 0 && b.Height() > 0
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
	if x, y, ok := c.dynamicOverviewAnchor(anchorID); ok {
		return x, y
	}
	coord, ok := anchorRegistry[anchorID]
	if !ok {
		return c.OffsetX, c.OffsetY
	}
	return c.TileToScreen(coord.X, coord.Y, coord.Z)
}

func (c *Camera) AnchorToRoomScreen(roomID, anchorID string) (float64, float64) {
	if CanonicalRoomID(roomID) == RoomMainHall {
		if x, y, ok := c.dynamicOverviewAnchor(anchorID); ok {
			return x, y
		}
	}

	layout, ok := RoomWorldLayoutFor(roomID)
	if !ok {
		return c.AnchorToScreen(anchorID)
	}
	plate := layout.OverviewPlateBounds()
	fx, fy := roomAnchorFraction(CanonicalRoomID(roomID), anchorID)
	return plate.MinX + plate.Width()*fx, plate.MinY + plate.Height()*fy
}

func roomAnchorFraction(roomID RoomID, anchorID string) (float64, float64) {
	switch roomID {
	case RoomArchive:
		if anchorID == "archive_lectern" {
			return 0.50, 0.62
		}
	case RoomStudy:
		if anchorID == "library_desk" {
			return 0.45, 0.62
		}
	case RoomObservatory:
		if anchorID == "crystal_orb_watch" {
			return 0.55, 0.58
		}
	case RoomWorkshop:
		if anchorID == "war_room_table" {
			return 0.50, 0.58
		}
	case RoomPotions:
		if anchorID == "cauldron_stir" {
			return 0.50, 0.62
		}
	case RoomThreshold:
		if anchorID == "threshold_center" {
			return 0.50, 0.50
		}
	case RoomBedroom:
		if anchorID == "bedroom_center" {
			return 0.50, 0.50
		}
	}
	return 0.50, 0.50
}

func (c *Camera) dynamicOverviewAnchor(anchorID string) (float64, float64, bool) {
	layout, ok := RoomWorldLayoutFor("main_hall")
	if !ok {
		return 0, 0, false
	}
	plate := layout.OverviewPlateBounds()

	switch anchorID {
	case "main_hall_center":
		return plate.MinX + plate.Width()*0.50, plate.MinY + plate.Height()*0.50, true
	case "main_hall_left":
		return plate.MinX + plate.Width()*0.32, plate.MinY + plate.Height()*0.52, true
	case "main_hall_right":
		return plate.MinX + plate.Width()*0.68, plate.MinY + plate.Height()*0.52, true
	case "main_hall_front":
		return plate.MinX + plate.Width()*0.50, plate.MinY + plate.Height()*0.72, true
	default:
		return 0, 0, false
	}
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

func (c *Camera) FitZoomForBounds(bounds WorldBounds, padX, padY float64) float64 {
	if c == nil || c.ScreenW <= 0 || c.ScreenH <= 0 {
		return 1
	}
	bounds = bounds.Normalized()
	if !bounds.Valid() {
		return c.View.Zoom
	}

	availableW := float64(c.ScreenW) - padX*2
	availableH := float64(c.ScreenH) - padY*2
	if availableW <= 0 || availableH <= 0 {
		return c.View.Zoom
	}

	return clamp(minFloat(availableW/bounds.Width(), availableH/bounds.Height()), 0.1, 3.0)
}

func (c *Camera) FitToBounds(bounds WorldBounds, padX, padY float64) {
	if c == nil || c.ScreenW <= 0 || c.ScreenH <= 0 {
		return
	}
	bounds = bounds.Normalized()
	if !bounds.Valid() {
		return
	}

	fitZoom := c.FitZoomForBounds(bounds, padX, padY)
	c.View.MinZoom = fitZoom
	c.View.MaxZoom = clamp(fitZoom*3.0, 1.25, 3.0)
	c.View.Zoom = fitZoom

	worldCenterX := (bounds.MinX + bounds.MaxX) / 2
	worldCenterY := (bounds.MinY + bounds.MaxY) / 2
	screenCenterX := float64(c.ScreenW) / 2
	screenCenterY := float64(c.ScreenH) / 2

	c.View.PanX = screenCenterX - c.OffsetX - (worldCenterX-c.OffsetX)*c.View.Zoom
	c.View.PanY = screenCenterY - c.OffsetY - (worldCenterY-c.OffsetY)*c.View.Zoom
	c.ClampToBounds(bounds, padX, padY)
}

func (c *Camera) ClampToBounds(bounds WorldBounds, padX, padY float64) {
	if c == nil || c.ScreenW <= 0 || c.ScreenH <= 0 {
		return
	}
	bounds = bounds.Normalized()
	if !bounds.Valid() {
		return
	}

	noPanMinX := (bounds.MinX-c.OffsetX)*c.View.Zoom + c.OffsetX
	noPanMaxX := (bounds.MaxX-c.OffsetX)*c.View.Zoom + c.OffsetX
	noPanMinY := (bounds.MinY-c.OffsetY)*c.View.Zoom + c.OffsetY
	noPanMaxY := (bounds.MaxY-c.OffsetY)*c.View.Zoom + c.OffsetY

	availableW := float64(c.ScreenW) - padX*2
	availableH := float64(c.ScreenH) - padY*2
	if noPanMaxX-noPanMinX <= availableW {
		c.View.PanX = float64(c.ScreenW)/2 - (noPanMinX+noPanMaxX)/2
	} else {
		minPanX := float64(c.ScreenW) - padX - noPanMaxX
		maxPanX := padX - noPanMinX
		c.View.PanX = clamp(c.View.PanX, minPanX, maxPanX)
	}
	if noPanMaxY-noPanMinY <= availableH {
		c.View.PanY = float64(c.ScreenH)/2 - (noPanMinY+noPanMaxY)/2
	} else {
		minPanY := float64(c.ScreenH) - padY - noPanMaxY
		maxPanY := padY - noPanMinY
		c.View.PanY = clamp(c.View.PanY, minPanY, maxPanY)
	}
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
// X/Y are tile coordinates; Z is elevation (0 = floor level).
var anchorRegistry = map[string]AnchorCoord{
	// Main Hall — the default room, where Milo returns after every task
	"main_hall_center": {X: 8, Y: 8, Z: 0},
	"main_hall_door":   {X: 4, Y: 8, Z: 0},
	"main_hall_throne": {X: 12, Y: 5, Z: 2}, // raised dais
	"main_hall_left":   {X: 6, Y: 8, Z: 0},
	"main_hall_right":  {X: 10, Y: 8, Z: 0},
	"main_hall_front":  {X: 8, Y: 10, Z: 0},

	// Legacy compatibility anchors retained for older sidecar room names.
	"war_room_table":      {X: 8, Y: 6, Z: 0},
	"war_room_map_wall":   {X: 13, Y: 4, Z: 0},
	"war_room_corner":     {X: 5, Y: 6, Z: 0},
	"war_room_toolbench":  {X: 11, Y: 7, Z: 0},
	"library_desk":        {X: 6, Y: 10, Z: 0},
	"library_shelf_east":  {X: 14, Y: 6, Z: 0},
	"library_shelf_north": {X: 6, Y: 3, Z: 0},
	"library_window":      {X: 10, Y: 5, Z: 0},
	"training_circle":     {X: 8, Y: 8, Z: 0},
	"training_dummy":      {X: 11, Y: 6, Z: 0},
	"spellbook_reading":   {X: 7, Y: 8, Z: 0},
	"spellbook_shelf":     {X: 5, Y: 5, Z: 0},
	"cauldron_stir":       {X: 8, Y: 9, Z: 0},
	"cauldron_shelf":      {X: 5, Y: 12, Z: 0},
	"cauldron_table":      {X: 8, Y: 11, Z: 0},
	"crystal_orb_stand":   {X: 8, Y: 7, Z: 1}, // orb on a plinth
	"crystal_orb_watch":   {X: 10, Y: 9, Z: 0},
	"crystal_orb_rim":     {X: 6, Y: 8, Z: 0},
	"crystal_orb_window":  {X: 12, Y: 5, Z: 0},
	"dragon_perch":        {X: 10, Y: 6, Z: 3}, // dragon on high perch
	"dragon_play_mat":     {X: 7, Y: 9, Z: 0},

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
