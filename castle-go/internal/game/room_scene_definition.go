package game

import (
	"image/color"
	"math"
	"sort"
)

const (
	RoomRenderOverviewPlate = "overview_plate"
	RoomRenderAuthored25D   = "authored_2_5d"
)

type RoomSceneDefinition struct {
	RoomID              RoomID
	RenderMode          string
	Floor               RoomLayer
	RearWall            RoomLayer
	SideWalls           []RoomLayer
	Doors               map[RoomID]DoorAnchor
	WalkableAreas       []WalkableArea
	InteriorWaypoints   map[string]ProofWaypoint
	Props               []RoomLayer
	ForegroundOccluders []RoomLayer
	Lighting            []RoomLayer
	ProofMarkers        []ProofWaypoint
}

type RoomLayer struct {
	ID             string
	Kind           string
	Bounds         WorldBounds
	Color          color.RGBA
	ZMin           int
	ZMax           int
	ProductVisible bool
	ProofVisible   bool
}

type DoorAnchor struct {
	ID      string
	ToRoom  RoomID
	X       float64
	Y       float64
	Width   float64
	Allowed bool
}

type WalkableArea struct {
	ID     string
	Kind   string
	Bounds WorldBounds
}

type AuthoredRouteValidation struct {
	Status        string
	DoorAnchors   []string
	WalkableAreas []string
	InvalidPoints []string
}

func AuthoredRoomDefinitions() map[RoomID]RoomSceneDefinition {
	defs := map[RoomID]RoomSceneDefinition{}
	for _, roomID := range []RoomID{RoomTrophy, RoomBedroom, RoomThreshold} {
		layout, ok := RoomWorldLayoutFor(string(roomID))
		if !ok {
			continue
		}
		defs[roomID] = authoredRoomDefinition(roomID, layout)
	}
	return defs
}

func AuthoredRoomDefinitionFor(roomID string) (RoomSceneDefinition, bool) {
	def, ok := AuthoredRoomDefinitions()[CanonicalRoomID(roomID)]
	return def, ok
}

func IsAuthoredRoom(roomID string) bool {
	_, ok := AuthoredRoomDefinitionFor(roomID)
	return ok
}

func authoredRoomDefinition(roomID RoomID, layout RoomWorldLayout) RoomSceneDefinition {
	plate := layout.OverviewPlateBounds()
	floor := WorldBounds{
		MinX: plate.MinX + 34,
		MinY: plate.MinY + 118,
		MaxX: plate.MaxX - 34,
		MaxY: plate.MaxY - 64,
	}
	rear := WorldBounds{
		MinX: plate.MinX + 42,
		MinY: plate.MinY + 42,
		MaxX: plate.MaxX - 42,
		MaxY: plate.MinY + 156,
	}
	leftWall := WorldBounds{
		MinX: plate.MinX + 42,
		MinY: plate.MinY + 92,
		MaxX: plate.MinX + 104,
		MaxY: plate.MaxY - 64,
	}
	rightWall := WorldBounds{
		MinX: plate.MaxX - 104,
		MinY: plate.MinY + 92,
		MaxX: plate.MaxX - 42,
		MaxY: plate.MaxY - 64,
	}
	front := WorldBounds{
		MinX: plate.MinX + 48,
		MinY: plate.MaxY - 104,
		MaxX: plate.MaxX - 48,
		MaxY: plate.MaxY - 54,
	}
	walk := WorldBounds{
		MinX: floor.MinX + 44,
		MinY: floor.MinY + 34,
		MaxX: floor.MaxX - 44,
		MaxY: floor.MaxY - 34,
	}

	palette := authoredPalette(roomID)
	def := RoomSceneDefinition{
		RoomID:     roomID,
		RenderMode: RoomRenderAuthored25D,
		Floor: RoomLayer{
			ID:             string(roomID) + "_floor",
			Kind:           "floor",
			Bounds:         floor,
			Color:          palette.floor,
			ProductVisible: true,
			ProofVisible:   true,
		},
		RearWall: RoomLayer{
			ID:             string(roomID) + "_rear_wall",
			Kind:           "rear_wall",
			Bounds:         rear,
			Color:          palette.rear,
			ProductVisible: true,
			ProofVisible:   true,
		},
		SideWalls: []RoomLayer{
			{ID: string(roomID) + "_left_wall", Kind: "side_wall", Bounds: leftWall, Color: palette.side, ProductVisible: true, ProofVisible: true},
			{ID: string(roomID) + "_right_wall", Kind: "side_wall", Bounds: rightWall, Color: palette.side, ProductVisible: true, ProofVisible: true},
		},
		WalkableAreas: []WalkableArea{
			{ID: string(roomID) + "_walkable_interior", Kind: "interior", Bounds: walk},
		},
		InteriorWaypoints: map[string]ProofWaypoint{
			"center": {X: (walk.MinX + walk.MaxX) / 2, Y: (walk.MinY + walk.MaxY) / 2, Label: string(roomID) + "_center"},
		},
		Props: []RoomLayer{
			{ID: string(roomID) + "_task_prop", Kind: "placeholder_prop", Bounds: insetBounds(rear, 96, 34), Color: palette.prop, ProductVisible: true, ProofVisible: true},
		},
		ForegroundOccluders: []RoomLayer{
			{ID: string(roomID) + "_foreground_occluder", Kind: "foreground_occluder", Bounds: front, Color: palette.front, ProductVisible: true, ProofVisible: true},
		},
		Lighting: []RoomLayer{
			{ID: string(roomID) + "_mood_light", Kind: "lighting", Bounds: insetBounds(floor, 90, 80), Color: palette.light, ProductVisible: true, ProofVisible: true},
		},
		Doors:        map[RoomID]DoorAnchor{},
		ProofMarkers: []ProofWaypoint{},
	}

	switch roomID {
	case RoomTrophy:
		def.Doors[RoomBedroom] = DoorAnchor{ID: "trophy_room_to_bedroom", ToRoom: RoomBedroom, X: walk.MaxX, Y: walk.MaxY - 52, Width: 92, Allowed: true}
		def.InteriorWaypoints["trophy_display"] = ProofWaypoint{X: (walk.MinX + walk.MaxX) / 2, Y: walk.MinY + walk.Height()*0.56, Label: "trophy_room_display"}
	case RoomBedroom:
		def.Doors[RoomTrophy] = DoorAnchor{ID: "bedroom_to_trophy_room", ToRoom: RoomTrophy, X: walk.MinX, Y: walk.MinY + 74, Width: 92, Allowed: true}
		def.Doors[RoomThreshold] = DoorAnchor{ID: "bedroom_to_threshold", ToRoom: RoomThreshold, X: walk.MinX, Y: walk.MaxY - 72, Width: 92, Allowed: true}
		def.InteriorWaypoints["bedroom_pass_through"] = ProofWaypoint{X: (walk.MinX + walk.MaxX) / 2, Y: (walk.MinY + walk.MaxY) / 2, Label: "bedroom_pass_through"}
		def.InteriorWaypoints["bedroom_center"] = def.InteriorWaypoints["bedroom_pass_through"]
	case RoomThreshold:
		def.Doors[RoomBedroom] = DoorAnchor{ID: "threshold_to_bedroom", ToRoom: RoomBedroom, X: walk.MaxX, Y: walk.MinY + 70, Width: 92, Allowed: true}
		def.InteriorWaypoints["threshold_center"] = ProofWaypoint{X: (walk.MinX + walk.MaxX) / 2, Y: (walk.MinY + walk.MaxY) / 2, Label: "threshold_center"}
	}
	for _, door := range def.Doors {
		def.ProofMarkers = append(def.ProofMarkers, ProofWaypoint{X: door.X, Y: door.Y, Label: door.ID})
	}
	return def
}

type roomPalette struct {
	floor color.RGBA
	rear  color.RGBA
	side  color.RGBA
	front color.RGBA
	prop  color.RGBA
	light color.RGBA
}

func authoredPalette(roomID RoomID) roomPalette {
	switch roomID {
	case RoomTrophy:
		return roomPalette{
			floor: color.RGBA{R: 116, G: 90, B: 58, A: 255},
			rear:  color.RGBA{R: 91, G: 66, B: 93, A: 255},
			side:  color.RGBA{R: 71, G: 58, B: 86, A: 255},
			front: color.RGBA{R: 48, G: 36, B: 54, A: 180},
			prop:  color.RGBA{R: 236, G: 194, B: 84, A: 245},
			light: color.RGBA{R: 255, G: 216, B: 106, A: 36},
		}
	case RoomBedroom:
		return roomPalette{
			floor: color.RGBA{R: 86, G: 77, B: 112, A: 255},
			rear:  color.RGBA{R: 78, G: 63, B: 104, A: 255},
			side:  color.RGBA{R: 58, G: 54, B: 86, A: 255},
			front: color.RGBA{R: 44, G: 38, B: 68, A: 180},
			prop:  color.RGBA{R: 184, G: 154, B: 205, A: 245},
			light: color.RGBA{R: 197, G: 180, B: 255, A: 34},
		}
	default:
		return roomPalette{
			floor: color.RGBA{R: 82, G: 88, B: 96, A: 255},
			rear:  color.RGBA{R: 60, G: 70, B: 82, A: 255},
			side:  color.RGBA{R: 48, G: 58, B: 72, A: 255},
			front: color.RGBA{R: 34, G: 40, B: 50, A: 180},
			prop:  color.RGBA{R: 130, G: 190, B: 210, A: 245},
			light: color.RGBA{R: 130, G: 220, B: 255, A: 32},
		}
	}
}

func insetBounds(bounds WorldBounds, xInset, yInset float64) WorldBounds {
	return WorldBounds{
		MinX: bounds.MinX + xInset,
		MinY: bounds.MinY + yInset,
		MaxX: bounds.MaxX - xInset,
		MaxY: bounds.MaxY - yInset,
	}
}

func AuthoredDoorWaypoint(fromRoom, toRoom string) (ProofWaypoint, bool) {
	def, ok := AuthoredRoomDefinitionFor(fromRoom)
	if !ok {
		return ProofWaypoint{}, false
	}
	door, ok := def.Doors[CanonicalRoomID(toRoom)]
	if !ok || !door.Allowed {
		return ProofWaypoint{}, false
	}
	return ProofWaypoint{X: door.X, Y: door.Y, Label: door.ID}, true
}

func AuthoredInteriorWaypoint(roomID, anchorID string) (ProofWaypoint, bool) {
	def, ok := AuthoredRoomDefinitionFor(roomID)
	if !ok {
		return ProofWaypoint{}, false
	}
	if point, ok := def.InteriorWaypoints[anchorID]; ok {
		return point, true
	}
	if point, ok := def.InteriorWaypoints["center"]; ok {
		return point, true
	}
	return ProofWaypoint{}, false
}

func ValidateAuthoredRouteWaypoints(points []ProofWaypoint) AuthoredRouteValidation {
	validation := AuthoredRouteValidation{Status: "PASS"}
	seenDoors := map[string]bool{}
	seenAreas := map[string]bool{}
	for _, def := range AuthoredRoomDefinitions() {
		for _, door := range def.Doors {
			seenDoors[door.ID] = true
		}
		for _, area := range def.WalkableAreas {
			seenAreas[area.ID] = true
		}
	}
	for door := range seenDoors {
		validation.DoorAnchors = append(validation.DoorAnchors, door)
	}
	for area := range seenAreas {
		validation.WalkableAreas = append(validation.WalkableAreas, area)
	}
	sort.Strings(validation.DoorAnchors)
	sort.Strings(validation.WalkableAreas)
	for _, point := range points {
		if !authoredPointAllowed(point) {
			validation.Status = "FAIL"
			validation.InvalidPoints = append(validation.InvalidPoints, point.Label)
		}
	}
	sort.Strings(validation.InvalidPoints)
	return validation
}

func authoredPointAllowed(point ProofWaypoint) bool {
	for _, def := range AuthoredRoomDefinitions() {
		for _, area := range def.WalkableAreas {
			if boundsContains(area.Bounds, point.X, point.Y) {
				return true
			}
		}
		for _, door := range def.Doors {
			if absFloat(point.X-door.X) <= door.Width/2 && absFloat(point.Y-door.Y) <= door.Width/2 {
				return true
			}
		}
	}
	if authoredCorridorPointAllowed(point, RoomTrophy, RoomBedroom) || authoredCorridorPointAllowed(point, RoomBedroom, RoomThreshold) {
		return true
	}
	return false
}

func authoredCorridorPointAllowed(point ProofWaypoint, fromRoom, toRoom RoomID) bool {
	from, ok := AuthoredDoorWaypoint(string(fromRoom), string(toRoom))
	if !ok {
		return false
	}
	to, ok := AuthoredDoorWaypoint(string(toRoom), string(fromRoom))
	if !ok {
		return false
	}
	dx := to.X - from.X
	dy := to.Y - from.Y
	lengthSquared := dx*dx + dy*dy
	if lengthSquared <= 0 {
		return false
	}
	t := ((point.X-from.X)*dx + (point.Y-from.Y)*dy) / lengthSquared
	if t < 0 || t > 1 {
		return false
	}
	projX := from.X + t*dx
	projY := from.Y + t*dy
	return mathDistance(point.X, point.Y, projX, projY) <= 84
}

func mathDistance(x1, y1, x2, y2 float64) float64 {
	dx := x2 - x1
	dy := y2 - y1
	return math.Hypot(dx, dy)
}

func boundsContains(bounds WorldBounds, x, y float64) bool {
	bounds = bounds.Normalized()
	return x >= bounds.MinX && x <= bounds.MaxX && y >= bounds.MinY && y <= bounds.MaxY
}
