package movement

import "strings"

type Plan struct {
	Prompt         string `json:"prompt"`
	Mode           string `json:"mode"`
	Destination    string `json:"destination"`
	Response       string `json:"response"`
	ResponseFamily string `json:"response_family"`
	RouteFamily    string `json:"route_family"`
	RouteVariant   string `json:"route_variant"`
	ArrivalAnchor  string `json:"arrival_anchor"`
	ArrivalStyle   string `json:"arrival_style"`
	EasterEgg      string `json:"easter_egg,omitempty"`
	UsesThreshold  bool   `json:"uses_threshold"`
}

type roomBehavior struct {
	aliases        []string
	responseSet    []string
	arrivalAnchors []string
	arrivalStyles  []string
}

type exploratoryBehavior struct {
	mode         string
	destinations []string
	responses    []string
	easterEggs   []string
	scenic       bool
}

type Planner struct {
	responseLast map[string]int
	arrivalLast  map[string]int
	promptLast   map[string]int
}

func NewPlanner() *Planner {
	return &Planner{
		responseLast: map[string]int{},
		arrivalLast:  map[string]int{},
		promptLast:   map[string]int{},
	}
}

func (p *Planner) Plan(prompt, currentRoom, currentState string) (Plan, bool) {
	normalized := normalizePrompt(prompt)
	if normalized == "" {
		return Plan{}, false
	}

	if destination, ok := resolveNamedDestination(normalized); ok {
		return p.planNamedDestination(prompt, destination, currentRoom), true
	}

	if plan, ok := p.planExploratory(prompt, normalized, currentRoom, currentState); ok {
		return plan, true
	}

	return Plan{}, false
}

func (p *Planner) planNamedDestination(prompt, destination, currentRoom string) Plan {
	behavior := roomBehaviors[destination]
	responseIndex := p.nextIndex("named:"+destination, len(behavior.responseSet))
	arrivalIndex := p.nextArrival(destination, len(behavior.arrivalAnchors))

	return Plan{
		Prompt:         prompt,
		Mode:           "named_destination",
		Destination:    destination,
		Response:       behavior.responseSet[responseIndex],
		ResponseFamily: responseFamilyForIndex(responseIndex),
		RouteFamily:    routeFamily(currentRoom, destination),
		RouteVariant:   p.selectRouteVariant(currentRoom, destination, false),
		ArrivalAnchor:  behavior.arrivalAnchors[arrivalIndex],
		ArrivalStyle:   behavior.arrivalStyles[arrivalIndex%len(behavior.arrivalStyles)],
		UsesThreshold:  requiresThreshold(currentRoom, destination),
	}
}

func (p *Planner) planExploratory(prompt, normalized, currentRoom, currentState string) (Plan, bool) {
	config, ok := exploratoryPrompts[normalized]
	if !ok {
		return Plan{}, false
	}

	destination := p.pickExploratoryDestination(config, currentRoom, currentState)
	behavior := roomBehaviors[destination]
	responseIndex := p.nextIndex("explore:"+normalized, len(config.responses))
	arrivalIndex := p.nextArrival(destination, len(behavior.arrivalAnchors))
	easterEgg := ""
	if len(config.easterEggs) > 0 {
		easterEgg = config.easterEggs[p.nextIndex("egg:"+normalized, len(config.easterEggs))]
	}

	return Plan{
		Prompt:         prompt,
		Mode:           config.mode,
		Destination:    destination,
		Response:       config.responses[responseIndex],
		ResponseFamily: responseFamilyForIndex(responseIndex),
		RouteFamily:    routeFamily(currentRoom, destination),
		RouteVariant:   p.selectRouteVariant(currentRoom, destination, config.scenic),
		ArrivalAnchor:  behavior.arrivalAnchors[arrivalIndex],
		ArrivalStyle:   behavior.arrivalStyles[arrivalIndex%len(behavior.arrivalStyles)],
		EasterEgg:      easterEgg,
		UsesThreshold:  requiresThreshold(currentRoom, destination),
	}, true
}

func (p *Planner) pickExploratoryDestination(config exploratoryBehavior, currentRoom, currentState string) string {
	if len(config.destinations) == 0 {
		return currentRoom
	}

	switch config.mode {
	case "favorite_room":
		if currentState == "working" {
			return "study"
		}
		if currentState == "idle" {
			return "main_hall"
		}
		return "archive"
	case "show_me_something_cool":
		if currentState == "working" {
			return "observatory"
		}
		return "trophy_room"
	default:
		return config.destinations[p.nextIndex("dest:"+config.mode, len(config.destinations))]
	}
}

func (p *Planner) selectRouteVariant(currentRoom, destination string, scenic bool) string {
	variants := hallVariantsForRoute(currentRoom, destination)
	if len(variants) == 0 {
		return ""
	}
	key := "route:" + routeFamily(currentRoom, destination)
	if scenic && len(variants) > 1 {
		index := p.nextIndex(key+":scenic", len(variants)-1)
		return variants[(index+1)%len(variants)]
	}
	return variants[p.nextIndex(key, len(variants))]
}

func (p *Planner) nextIndex(key string, size int) int {
	if size <= 1 {
		return 0
	}
	index := p.promptLast[key] % size
	p.promptLast[key] = (index + 1) % size
	return index
}

func (p *Planner) nextArrival(room string, size int) int {
	if size <= 1 {
		return 0
	}
	index := p.arrivalLast[room] % size
	p.arrivalLast[room] = (index + 1) % size
	return index
}

func normalizePrompt(prompt string) string {
	return strings.Join(strings.Fields(strings.ToLower(prompt)), " ")
}

func resolveNamedDestination(normalized string) (string, bool) {
	for roomID, behavior := range roomBehaviors {
		for _, alias := range behavior.aliases {
			if strings.Contains(normalized, alias) {
				return roomID, true
			}
		}
	}
	return "", false
}

func responseFamilyForIndex(index int) string {
	switch index {
	case 0:
		return "simple_compliance"
	case 1:
		return "in_character_compliance"
	default:
		return "warm_attitude"
	}
}

func canonicalRoomID(roomID string) string {
	switch roomID {
	case "trophy":
		return "trophy_room"
	case "library":
		return "study"
	case "war_room":
		return "workshop"
	case "cauldron":
		return "potions_room"
	case "crystal_orb":
		return "observatory"
	default:
		return roomID
	}
}

func sharesWall(a, b string) bool {
	a = canonicalRoomID(a)
	b = canonicalRoomID(b)
	if a == b {
		return true
	}
	switch a {
	case "main_hall":
		return b == "archive" || b == "trophy_room"
	case "archive":
		return b == "main_hall"
	case "trophy_room":
		return b == "main_hall"
	case "study":
		return b == "observatory"
	case "observatory":
		return b == "study"
	case "workshop":
		return b == "potions_room"
	case "potions_room":
		return b == "workshop"
	default:
		return false
	}
}

func requiresThreshold(a, b string) bool {
	a = canonicalRoomID(a)
	b = canonicalRoomID(b)
	if a == b {
		return false
	}
	return !sharesWall(a, b)
}

func routeFamily(fromRoom, toRoom string) string {
	fromRoom = canonicalRoomID(fromRoom)
	toRoom = canonicalRoomID(toRoom)
	if fromRoom == toRoom {
		return fromRoom + "_local"
	}
	return fromRoom + "_to_" + toRoom
}

func hallVariantsForRoute(fromRoom, toRoom string) []string {
	if !requiresThreshold(fromRoom, toRoom) {
		return []string{"direct_wall_cross"}
	}

	switch routeFamily(fromRoom, toRoom) {
	case "main_hall_to_archive", "archive_to_main_hall", "main_hall_to_trophy_room", "trophy_room_to_main_hall":
		return []string{"home_threshold_short", "home_threshold_sconce_pause"}
	case "main_hall_to_study", "study_to_main_hall", "archive_to_observatory", "observatory_to_archive":
		return []string{"knowledge_arcade", "knowledge_crystal_walk", "knowledge_scenic_hall"}
	case "main_hall_to_workshop", "workshop_to_main_hall", "archive_to_workshop", "workshop_to_archive":
		return []string{"making_corridor", "making_braided_hall", "making_sconces_route"}
	default:
		return []string{"threshold_standard", "threshold_scenic"}
	}
}

var roomBehaviors = map[string]roomBehavior{
	"main_hall": {
		aliases: []string{"main hall", "hall", "home"},
		responseSet: []string{
			"On my way to the Main Hall.",
			"Very well. To the Main Hall.",
			"Home again, then. Main Hall.",
		},
		arrivalAnchors: []string{"main_hall_center", "main_hall_left", "main_hall_right"},
		arrivalStyles:  []string{"home_ready", "attentive_wait", "ready_shift"},
	},
	"archive": {
		aliases: []string{"archive"},
		responseSet: []string{
			"On my way to the Archive.",
			"Very well. To the Archive.",
			"A sensible choice. Archive.",
		},
		arrivalAnchors: []string{"archive_lectern", "archive_crystal"},
		arrivalStyles:  []string{"calm_archive", "observing"},
	},
	"trophy_room": {
		aliases: []string{"trophy room", "trophy"},
		responseSet: []string{
			"Heading to the Trophy Room.",
			"Let's see what waits in the Trophy Room.",
			"To the Trophy Room, then. Modestly proud, of course.",
		},
		arrivalAnchors: []string{"trophy_display", "trophy_pedestal"},
		arrivalStyles:  []string{"proud", "reflective_proud"},
	},
	"study": {
		aliases: []string{"study", "library"},
		responseSet: []string{
			"On my way to the Study.",
			"Very well. To the Study.",
			"Study it is. Try not to look too pleased with yourself.",
		},
		arrivalAnchors: []string{"library_desk", "library_shelf_east"},
		arrivalStyles:  []string{"archive_read", "attentive_wait"},
	},
	"workshop": {
		aliases: []string{"workshop"},
		responseSet: []string{
			"On my way to the Workshop.",
			"To the Workshop, then.",
			"Workshop it is. Something useful had better come of it.",
		},
		arrivalAnchors: []string{"war_room_table", "war_room_map_wall"},
		arrivalStyles:  []string{"ready_shift", "attentive_wait"},
	},
	"observatory": {
		aliases: []string{"observatory"},
		responseSet: []string{
			"Heading to the Observatory.",
			"Very well. To the Observatory.",
			"Observatory it is. Try not to look too smug about it.",
		},
		arrivalAnchors: []string{"crystal_orb_watch", "crystal_orb_stand"},
		arrivalStyles:  []string{"outward_glance", "observing"},
	},
	"potions_room": {
		aliases: []string{"potions room", "potion room", "potions"},
		responseSet: []string{
			"On my way to the Potions Room.",
			"I'll make for the Potions Room.",
			"Potions Room, then. Let's avoid anything unlabeled.",
		},
		arrivalAnchors: []string{"cauldron_stir", "cauldron_shelf"},
		arrivalStyles:  []string{"observing", "robe_settle"},
	},
}

var exploratoryPrompts = map[string]exploratoryBehavior{
	"take me somewhere": {
		mode:         "wander",
		destinations: []string{"main_hall", "archive", "trophy_room"},
		responses: []string{
			"On my way.",
			"Dealer's choice, then.",
			"Let's see where the castle feels most agreeable.",
		},
		easterEggs: []string{"scenic_threshold_home", "dealers_choice"},
		scenic:     true,
	},
	"show me around": {
		mode:         "tour",
		destinations: []string{"archive", "trophy_room", "study"},
		responses: []string{
			"Very well. I'll show you a little of the place.",
			"Come along, then. The castle has manners to maintain.",
			"A short tour, then. No getting lost in the halls.",
		},
		easterEggs: []string{"tiny_castle_tour_line", "formal_intro_pause"},
		scenic:     true,
	},
	"walk around": {
		mode:         "wander",
		destinations: []string{"main_hall", "archive", "trophy_room"},
		responses: []string{
			"I'll take a turn through the castle.",
			"A little walk, then.",
			"Very well. I'll stretch my legs through the home wing.",
		},
		easterEggs: []string{"extra_threshold_segment", "doorway_pause_check"},
		scenic:     true,
	},
	"patrol the castle": {
		mode:         "patrol",
		destinations: []string{"archive", "trophy_room", "main_hall"},
		responses: []string{
			"A proper round, then.",
			"I'll make the rounds.",
			"Very well. The halls won't mind being checked.",
		},
		easterEggs: []string{"doorway_pause_check", "formal_round"},
		scenic:     true,
	},
	"take the scenic route": {
		mode:         "scenic_route",
		destinations: []string{"archive", "observatory", "workshop"},
		responses: []string{
			"As you wish. We'll take the scenic route.",
			"Very well. A little extra hall drama never hurt anyone.",
			"Scenic it is. Try to look appropriately impressed.",
		},
		easterEggs: []string{"scenic_route_preference", "threshold_linger"},
		scenic:     true,
	},
	"surprise me": {
		mode:         "surprise",
		destinations: []string{"archive", "trophy_room", "observatory", "workshop"},
		responses: []string{
			"Very well. Leave it to me.",
			"Dealer's choice, then.",
			"I do have standards, you know. Surprise accepted.",
		},
		easterEggs: []string{"curated_surprise_route", "tester_curiosity_line"},
		scenic:     true,
	},
	"show me something cool": {
		mode:         "show_me_something_cool",
		destinations: []string{"trophy_room", "observatory"},
		responses: []string{
			"I have a thought or two.",
			"Very well. I know just the place.",
			"Something cool, then. Sensible request.",
		},
		easterEggs: []string{"trophy_gleam_or_horizon_check", "subtle_showman"},
	},
	"where would you go?": {
		mode:         "self_directed_choice",
		destinations: []string{"archive", "study", "main_hall"},
		responses: []string{
			"I have a preference.",
			"If it were up to me, I know where I'd stand.",
			"You ask excellent questions when you're being nosy.",
		},
		easterEggs: []string{"state_tinted_preference"},
	},
	"what's your favorite room?": {
		mode:         "favorite_room",
		destinations: []string{"archive", "study", "main_hall"},
		responses: []string{
			"I have one, yes.",
			"That depends on my mood.",
			"You'll get an answer, not a treaty.",
		},
		easterEggs: []string{"state_tinted_preference"},
	},
	"what is your favorite room?": {
		mode:         "favorite_room",
		destinations: []string{"archive", "study", "main_hall"},
		responses: []string{
			"I have one, yes.",
			"That depends on my mood.",
			"You'll get an answer, not a treaty.",
		},
		easterEggs: []string{"state_tinted_preference"},
	},
	"do something": {
		mode:         "micro_action",
		destinations: []string{"main_hall", "archive", "trophy_room"},
		responses: []string{
			"Very well.",
			"I can manage something small.",
			"All right. No need to sound so mysterious about it.",
		},
		easterEggs: []string{"tiny_voiced_quip", "context_idle_choice"},
	},
}
