package game

import "sort"

type RoomID string

const (
	RoomMainHall    RoomID = "main_hall"
	RoomArchive     RoomID = "archive"
	RoomTrophy      RoomID = "trophy_room"
	RoomStudy       RoomID = "study"
	RoomWorkshop    RoomID = "workshop"
	RoomObservatory RoomID = "observatory"
	RoomPotions     RoomID = "potions_room"
	RoomThreshold   RoomID = "threshold"
	RoomBedroom     RoomID = "bedroom"
)

type RouteStep struct {
	Room       RoomID `json:"room"`
	ViaHall    bool   `json:"via_hall"`
	Transition string `json:"transition"`
	Variant    string `json:"variant,omitempty"`
}

type RoomTopology struct {
	Canonical   RoomID
	DisplayName string
	Cluster     string
	LaunchLive  bool
	SafeIdle    bool
	Neighbors   []RoomID
	MapX        int
	MapY        int
}

func RoomGraph() map[RoomID]RoomTopology {
	graph := make(map[RoomID]RoomTopology, len(roomTopologies))
	for roomID, topo := range roomTopologies {
		copyTopo := topo
		copyTopo.Neighbors = append([]RoomID(nil), topo.Neighbors...)
		graph[roomID] = copyTopo
	}
	return graph
}

var roomAliasMap = map[string]RoomID{
	"main_hall":    RoomMainHall,
	"archive":      RoomArchive,
	"trophy":       RoomTrophy,
	"trophy_room":  RoomTrophy,
	"library":      RoomStudy,
	"study":        RoomStudy,
	"war_room":     RoomWorkshop,
	"workshop":     RoomWorkshop,
	"cauldron":     RoomPotions,
	"potions_room": RoomPotions,
	"crystal_orb":  RoomObservatory,
	"observatory":  RoomObservatory,
	"threshold":    RoomThreshold,
	"bedroom":      RoomBedroom,
}

var sceneRoomMap = map[RoomID]string{
	RoomArchive:     "archive",
	RoomTrophy:      "trophy_room",
	RoomStudy:       "study",
	RoomWorkshop:    "workshop",
	RoomObservatory: "observatory",
	RoomPotions:     "potions_room",
	RoomThreshold:   "threshold",
	RoomBedroom:     "bedroom",
	RoomMainHall:    "main_hall",
}

var roomTopologies = map[RoomID]RoomTopology{
	RoomMainHall: {
		Canonical:   RoomMainHall,
		DisplayName: "Main Hall",
		Cluster:     "home",
		LaunchLive:  false,
		SafeIdle:    true,
		Neighbors:   []RoomID{RoomArchive, RoomWorkshop, RoomThreshold},
		MapX:        0,
		MapY:        0,
	},
	RoomArchive: {
		Canonical:   RoomArchive,
		DisplayName: "Archive",
		Cluster:     "home",
		LaunchLive:  true,
		SafeIdle:    true,
		Neighbors:   []RoomID{RoomMainHall, RoomStudy, RoomTrophy},
		MapX:        0,
		MapY:        -1,
	},
	RoomTrophy: {
		Canonical:   RoomTrophy,
		DisplayName: "Trophy Room",
		Cluster:     "home",
		LaunchLive:  false,
		SafeIdle:    true,
		Neighbors:   []RoomID{RoomArchive, RoomBedroom},
		MapX:        1,
		MapY:        -1,
	},
	RoomStudy: {
		Canonical:   RoomStudy,
		DisplayName: "Study",
		Cluster:     "knowledge",
		LaunchLive:  true,
		SafeIdle:    false,
		Neighbors:   []RoomID{RoomArchive, RoomObservatory},
		MapX:        0,
		MapY:        -2,
	},
	RoomWorkshop: {
		Canonical:   RoomWorkshop,
		DisplayName: "Workshop",
		Cluster:     "making",
		LaunchLive:  false,
		SafeIdle:    false,
		Neighbors:   []RoomID{RoomMainHall, RoomPotions},
		MapX:        -1,
		MapY:        0,
	},
	RoomObservatory: {
		Canonical:   RoomObservatory,
		DisplayName: "Observatory",
		Cluster:     "knowledge",
		LaunchLive:  true,
		SafeIdle:    false,
		Neighbors:   []RoomID{RoomStudy},
		MapX:        0,
		MapY:        -3,
	},
	RoomPotions: {
		Canonical:   RoomPotions,
		DisplayName: "Potions Room",
		Cluster:     "making",
		LaunchLive:  false,
		SafeIdle:    false,
		Neighbors:   []RoomID{RoomWorkshop},
		MapX:        -1,
		MapY:        1,
	},
	RoomBedroom: {
		Canonical:   RoomBedroom,
		DisplayName: "Bedroom",
		Cluster:     "rest",
		LaunchLive:  false,
		SafeIdle:    true,
		Neighbors:   []RoomID{RoomThreshold, RoomTrophy},
		MapX:        2,
		MapY:        1,
	},
	RoomThreshold: {
		Canonical:   RoomThreshold,
		DisplayName: "Threshold",
		Cluster:     "threshold",
		LaunchLive:  false,
		SafeIdle:    false,
		Neighbors:   []RoomID{RoomMainHall, RoomBedroom},
		MapX:        0,
		MapY:        1,
	},
}

func CanonicalRoomID(roomID string) RoomID {
	if canonical, ok := roomAliasMap[roomID]; ok {
		return canonical
	}
	return RoomID(roomID)
}

func DisplayRoomID(roomID string) string {
	canonical := CanonicalRoomID(roomID)
	if topo, ok := roomTopologies[canonical]; ok {
		return topo.DisplayName
	}
	return roomID
}

func SceneRoomID(roomID string) string {
	canonical := CanonicalRoomID(roomID)
	if sceneID, ok := sceneRoomMap[canonical]; ok {
		return sceneID
	}
	return roomID
}

func SharesWall(a, b string) bool {
	aID := CanonicalRoomID(a)
	bID := CanonicalRoomID(b)
	if aID == bID {
		return true
	}
	topo, ok := roomTopologies[aID]
	if !ok {
		return false
	}
	for _, neighbor := range topo.Neighbors {
		if neighbor == bID {
			return true
		}
	}
	return false
}

func RequiresThreshold(a, b string) bool {
	aID := CanonicalRoomID(a)
	bID := CanonicalRoomID(b)
	if aID == bID {
		return false
	}
	return !SharesWall(string(aID), string(bID))
}

func SafeIdleRooms() []RoomID {
	rooms := []RoomID{RoomMainHall, RoomArchive, RoomTrophy}
	return append([]RoomID(nil), rooms...)
}

func LaunchLiveRooms() []RoomID {
	rooms := make([]RoomID, 0)
	for _, topo := range roomTopologies {
		if topo.LaunchLive {
			rooms = append(rooms, topo.Canonical)
		}
	}
	sort.Slice(rooms, func(i, j int) bool { return rooms[i] < rooms[j] })
	return rooms
}

func RouteBetween(from, to string) []RouteStep {
	return RouteBetweenVariant(from, to, "")
}

func RouteFamily(from, to string) string {
	start := CanonicalRoomID(from)
	end := CanonicalRoomID(to)
	if start == end {
		return "same_room"
	}
	startCluster := roomTopologies[start].Cluster
	endCluster := roomTopologies[end].Cluster
	if startCluster == endCluster {
		return startCluster + "_cluster"
	}
	switch {
	case (startCluster == "home" && endCluster == "knowledge") || (startCluster == "knowledge" && endCluster == "home"):
		return "home_knowledge"
	case (startCluster == "home" && endCluster == "making") || (startCluster == "making" && endCluster == "home"):
		return "home_making"
	case (startCluster == "knowledge" && endCluster == "making") || (startCluster == "making" && endCluster == "knowledge"):
		return "knowledge_making"
	default:
		return "cross_cluster"
	}
}

func HallVariantsForRoute(from, to string) []string {
	switch RouteFamily(from, to) {
	case "home_knowledge":
		return []string{"archive_sconces", "window_arc", "observatory_glow"}
	case "home_making":
		return []string{"ember_sconces", "forge_turn", "copper_arc"}
	case "knowledge_making":
		return []string{"grand_crossing", "service_pass", "lantern_bridge"}
	default:
		return []string{"castle_walk"}
	}
}

func RouteBetweenVariant(from, to, variant string) []RouteStep {
	start := CanonicalRoomID(from)
	end := CanonicalRoomID(to)
	if start == end {
		return []RouteStep{{Room: start, Transition: "stay"}}
	}
	queue := []RoomID{start}
	visited := map[RoomID]bool{start: true}
	prev := map[RoomID]RoomID{}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == end {
			break
		}
		for _, next := range roomTopologies[current].Neighbors {
			if visited[next] {
				continue
			}
			visited[next] = true
			prev[next] = current
			queue = append(queue, next)
		}
	}

	if !visited[end] {
		return []RouteStep{
			{Room: start, Transition: "depart"},
			{Room: RoomThreshold, ViaHall: true, Transition: "threshold", Variant: variant},
			{Room: end, Transition: "arrive"},
		}
	}

	path := []RoomID{end}
	for cursor := end; cursor != start; {
		cursor = prev[cursor]
		path = append([]RoomID{cursor}, path...)
	}

	steps := make([]RouteStep, 0, len(path)*2)
	for index, room := range path {
		if index == 0 {
			steps = append(steps, RouteStep{Room: room, Transition: "depart"})
			continue
		}
		transition := "arrive"
		if index < len(path)-1 {
			transition = "pass_through"
		}
		steps = append(steps, RouteStep{Room: room, Transition: transition, Variant: variant})
	}
	return steps
}
