package game

import "strings"

type DirectedMovePlan struct {
	Prompt         string `json:"prompt"`
	Mode           string `json:"mode"`
	Destination    RoomID `json:"destination"`
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
	aliases      []string
	destinations []RoomID
	responses    []string
	easterEggs   []string
	scenic       bool
}

type BehaviorDirector struct {
	responseLast map[string]int
	arrivalLast  map[RoomID]int
	promptLast   map[string]int
}

func NewBehaviorDirector() *BehaviorDirector {
	return &BehaviorDirector{
		responseLast: map[string]int{},
		arrivalLast:  map[RoomID]int{},
		promptLast:   map[string]int{},
	}
}

func (d *BehaviorDirector) Plan(prompt string, currentRoom RoomID, currentState string) (DirectedMovePlan, bool) {
	normalized := normalizePrompt(prompt)
	if normalized == "" {
		return DirectedMovePlan{}, false
	}

	if destination, ok := resolveNamedDestination(normalized); ok {
		return d.planNamedDestination(prompt, destination, currentRoom), true
	}

	if plan, ok := d.planExploratory(prompt, normalized, currentRoom, currentState); ok {
		return plan, true
	}

	return DirectedMovePlan{}, false
}

func (d *BehaviorDirector) planNamedDestination(prompt string, destination, currentRoom RoomID) DirectedMovePlan {
	behavior := roomBehaviors[destination]
	responseIndex := d.nextIndex("named:"+string(destination), len(behavior.responseSet))
	arrivalIndex := d.nextArrival(destination, len(behavior.arrivalAnchors))
	routeFamily := RouteFamily(string(currentRoom), string(destination))
	routeVariant := d.selectRouteVariant(currentRoom, destination, false)

	return DirectedMovePlan{
		Prompt:         prompt,
		Mode:           "named_destination",
		Destination:    destination,
		Response:       behavior.responseSet[responseIndex],
		ResponseFamily: responseFamilyForIndex(responseIndex),
		RouteFamily:    routeFamily,
		RouteVariant:   routeVariant,
		ArrivalAnchor:  behavior.arrivalAnchors[arrivalIndex],
		ArrivalStyle:   behavior.arrivalStyles[arrivalIndex%len(behavior.arrivalStyles)],
		UsesThreshold:  RequiresThreshold(string(currentRoom), string(destination)),
	}
}

func (d *BehaviorDirector) planExploratory(prompt, normalized string, currentRoom RoomID, currentState string) (DirectedMovePlan, bool) {
	config, ok := resolveExploratoryPrompt(normalized)
	if !ok {
		return DirectedMovePlan{}, false
	}

	destination := d.pickExploratoryDestination(config, currentRoom, currentState)
	behavior := roomBehaviors[destination]
	responseIndex := d.nextIndex("explore:"+normalized, len(config.responses))
	arrivalIndex := d.nextArrival(destination, len(behavior.arrivalAnchors))
	routeVariant := d.selectRouteVariant(currentRoom, destination, config.scenic)
	easterEgg := ""
	if len(config.easterEggs) > 0 {
		easterEggIndex := d.nextIndex("egg:"+normalized, len(config.easterEggs))
		easterEgg = config.easterEggs[easterEggIndex]
	}

	return DirectedMovePlan{
		Prompt:         prompt,
		Mode:           config.mode,
		Destination:    destination,
		Response:       config.responses[responseIndex],
		ResponseFamily: responseFamilyForIndex(responseIndex),
		RouteFamily:    RouteFamily(string(currentRoom), string(destination)),
		RouteVariant:   routeVariant,
		ArrivalAnchor:  behavior.arrivalAnchors[arrivalIndex],
		ArrivalStyle:   behavior.arrivalStyles[arrivalIndex%len(behavior.arrivalStyles)],
		EasterEgg:      easterEgg,
		UsesThreshold:  RequiresThreshold(string(currentRoom), string(destination)),
	}, true
}

func (d *BehaviorDirector) pickExploratoryDestination(config exploratoryBehavior, currentRoom RoomID, currentState string) RoomID {
	candidates := config.destinations
	if len(candidates) == 0 {
		return currentRoom
	}

	switch config.mode {
	case "favorite_room":
		if currentState == "working" {
			return RoomStudy
		}
		if currentState == "idle" {
			return RoomMainHall
		}
		return RoomArchive
	case "show_me_something_cool":
		if currentState == "working" {
			return RoomObservatory
		}
		return RoomTrophy
	default:
		index := d.nextIndex("dest:"+config.mode, len(candidates))
		return candidates[index]
	}
}

func (d *BehaviorDirector) selectRouteVariant(currentRoom, destination RoomID, scenic bool) string {
	variants := HallVariantsForRoute(string(currentRoom), string(destination))
	if len(variants) == 0 {
		return ""
	}
	key := "route:" + RouteFamily(string(currentRoom), string(destination))
	if scenic && len(variants) > 1 {
		index := d.nextIndex(key+":scenic", len(variants)-1)
		return variants[(index+1)%len(variants)]
	}
	index := d.nextIndex(key, len(variants))
	return variants[index]
}

func (d *BehaviorDirector) nextIndex(key string, size int) int {
	if size <= 1 {
		return 0
	}
	index := d.promptLast[key] % size
	d.promptLast[key] = (index + 1) % size
	return index
}

func (d *BehaviorDirector) nextArrival(room RoomID, size int) int {
	if size <= 1 {
		return 0
	}
	index := d.arrivalLast[room] % size
	d.arrivalLast[room] = (index + 1) % size
	return index
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

func resolveNamedDestination(normalized string) (RoomID, bool) {
	bestMatch := ""
	bestRoom := RoomID("")
	for roomID, behavior := range roomBehaviors {
		for _, alias := range behavior.aliases {
			if matchesPhrase(normalized, alias) && len(alias) > len(bestMatch) {
				bestMatch = alias
				bestRoom = roomID
			}
		}
	}
	if bestRoom != "" {
		return bestRoom, true
	}
	return "", false
}

func normalizePrompt(prompt string) string {
	return strings.Join(strings.Fields(strings.ToLower(prompt)), " ")
}

func resolveExploratoryPrompt(normalized string) (exploratoryBehavior, bool) {
	if config, ok := exploratoryPrompts[normalized]; ok {
		return config, true
	}
	bestMatch := ""
	var bestConfig exploratoryBehavior
	found := false
	for key, config := range exploratoryPrompts {
		if matchesPhrase(normalized, key) && len(key) > len(bestMatch) {
			bestMatch = key
			bestConfig = config
			found = true
		}
		for _, alias := range config.aliases {
			if matchesPhrase(normalized, alias) && len(alias) > len(bestMatch) {
				bestMatch = alias
				bestConfig = config
				found = true
			}
		}
	}
	if found {
		return bestConfig, true
	}
	return exploratoryBehavior{}, false
}

func matchesPhrase(normalized, phrase string) bool {
	padded := " " + normalized + " "
	needle := " " + phrase + " "
	return strings.Contains(padded, needle)
}

var roomBehaviors = map[RoomID]roomBehavior{
	RoomMainHall: {
		aliases: []string{"main hall", "hall", "home"},
		responseSet: []string{
			"On my way to the Main Hall.",
			"Very well. To the Main Hall.",
			"Home again, then. Main Hall.",
		},
		arrivalAnchors: []string{"main_hall_center", "main_hall_left", "main_hall_right", "main_hall_front"},
		arrivalStyles:  []string{"home_ready", "home_return", "attentive_wait", "available_settle"},
	},
	RoomArchive: {
		aliases: []string{"archive"},
		responseSet: []string{
			"On my way to the Archive.",
			"Very well. To the Archive.",
			"A sensible choice. Archive.",
		},
		arrivalAnchors: []string{"archive_lectern", "archive_crystal", "archive_shelf", "archive_shelf_east"},
		arrivalStyles:  []string{"calm_archive", "quiet_reentry", "archive_notice", "observing"},
	},
	RoomTrophy: {
		aliases: []string{"trophy room", "trophy"},
		responseSet: []string{
			"Heading to the Trophy Room.",
			"Let’s see what waits in the Trophy Room.",
			"To the Trophy Room, then. Modestly proud, of course.",
		},
		arrivalAnchors: []string{"trophy_display", "trophy_pedestal"},
		arrivalStyles:  []string{"proud", "reflective_proud"},
	},
	RoomStudy: {
		aliases: []string{"study"},
		responseSet: []string{
			"On my way to the Study.",
			"Very well. To the Study.",
			"Study it is. Try not to look too pleased with yourself.",
		},
		arrivalAnchors: []string{"library_desk", "library_shelf_east", "library_shelf_north", "library_window"},
		arrivalStyles:  []string{"study_focus", "archive_read", "note_check", "idea_pause"},
	},
	RoomWorkshop: {
		aliases: []string{"workshop"},
		responseSet: []string{
			"On my way to the Workshop.",
			"To the Workshop, then.",
			"Workshop it is. Something useful had better come of it.",
		},
		arrivalAnchors: []string{"war_room_table", "war_room_map_wall", "war_room_corner", "war_room_toolbench"},
		arrivalStyles:  []string{"ready_shift", "attentive_wait", "practical_steady", "tool_ready"},
	},
	RoomObservatory: {
		aliases: []string{"observatory"},
		responseSet: []string{
			"Heading to the Observatory.",
			"Very well. To the Observatory.",
			"Observatory it is. Try not to look too smug about it.",
		},
		arrivalAnchors: []string{"crystal_orb_watch", "crystal_orb_stand", "crystal_orb_rim", "crystal_orb_window"},
		arrivalStyles:  []string{"outward_glance", "horizon_check", "observatory_settle", "sky_listen"},
	},
	RoomPotions: {
		aliases: []string{"potions room", "potion room", "potions"},
		responseSet: []string{
			"On my way to the Potions Room.",
			"I’ll make for the Potions Room.",
			"Potions Room, then. Let’s avoid anything unlabeled.",
		},
		arrivalAnchors: []string{"cauldron_stir", "cauldron_shelf", "cauldron_table"},
		arrivalStyles:  []string{"observing", "robe_settle", "cauldron_focus", "ingredient_peek"},
	},
}

var exploratoryPrompts = map[string]exploratoryBehavior{
	"take me somewhere": {
		mode:         "wander",
		aliases:      []string{"take me somewhere nice", "pick a room", "take me anywhere"},
		destinations: []RoomID{RoomMainHall, RoomArchive, RoomTrophy},
		responses: []string{
			"On my way.",
			"Dealer’s choice, then.",
			"Let’s see where the castle feels most agreeable.",
		},
		easterEggs: []string{"scenic_threshold_home", "dealers_choice"},
		scenic:     true,
	},
	"show me around": {
		mode:         "tour",
		aliases:      []string{"show me the castle", "lead the way", "walk with me"},
		destinations: []RoomID{RoomArchive, RoomTrophy, RoomStudy},
		responses: []string{
			"Very well. I’ll show you a little of the place.",
			"Come along, then. The castle has manners to maintain.",
			"A short tour, then. No getting lost in the halls.",
		},
		easterEggs: []string{"tiny_castle_tour_line", "formal_intro_pause"},
		scenic:     true,
	},
	"walk around": {
		mode:         "wander",
		aliases:      []string{"wander a bit", "go stretch your legs", "take a walk"},
		destinations: []RoomID{RoomMainHall, RoomArchive, RoomTrophy},
		responses: []string{
			"I’ll take a turn through the castle.",
			"A little walk, then.",
			"Very well. I’ll stretch my legs through the home wing.",
		},
		easterEggs: []string{"extra_threshold_segment", "doorway_pause_check"},
		scenic:     true,
	},
	"patrol the castle": {
		mode:         "patrol",
		aliases:      []string{"make a round", "do a round of the halls", "walk the castle"},
		destinations: []RoomID{RoomArchive, RoomTrophy, RoomMainHall},
		responses: []string{
			"A proper round, then.",
			"I’ll make the rounds.",
			"Very well. The halls won’t mind being checked.",
		},
		easterEggs: []string{"doorway_pause_check", "formal_round"},
		scenic:     true,
	},
	"take the scenic route": {
		mode:         "scenic_route",
		aliases:      []string{"take the long way", "take the scenic way home"},
		destinations: []RoomID{RoomArchive, RoomObservatory, RoomWorkshop},
		responses: []string{
			"As you wish. We’ll take the scenic route.",
			"Very well. A little extra hall drama never hurt anyone.",
			"Scenic it is. Try to look appropriately impressed.",
		},
		easterEggs: []string{"scenic_route_preference", "threshold_linger"},
		scenic:     true,
	},
	"surprise me": {
		mode:         "surprise",
		aliases:      []string{"pick something", "take me somewhere interesting"},
		destinations: []RoomID{RoomArchive, RoomTrophy, RoomObservatory, RoomWorkshop},
		responses: []string{
			"Very well. Leave it to me.",
			"Dealer’s choice, then.",
			"I do have standards, you know. Surprise accepted.",
		},
		easterEggs: []string{"curated_surprise_route", "tester_curiosity_line"},
		scenic:     true,
	},
	"show me something cool": {
		mode:         "show_me_something_cool",
		aliases:      []string{"show me something interesting", "show me something impressive"},
		destinations: []RoomID{RoomTrophy, RoomObservatory},
		responses: []string{
			"I have a thought or two.",
			"Very well. I know just the place.",
			"Something cool, then. Sensible request.",
		},
		easterEggs: []string{"trophy_gleam_or_horizon_check", "subtle_showman"},
	},
	"where would you go?": {
		mode:         "self_directed_choice",
		aliases:      []string{"where would you rather be", "show me where you work", "show me what you like"},
		destinations: []RoomID{RoomArchive, RoomStudy, RoomMainHall},
		responses: []string{
			"I have a preference.",
			"If it were up to me, I know where I’d stand.",
			"You ask excellent questions when you’re being nosy.",
		},
		easterEggs: []string{"state_tinted_preference"},
	},
	"what's your favorite room?": {
		mode:         "favorite_room",
		aliases:      []string{"favorite room", "what is your favorite room", "show me your favorite place"},
		destinations: []RoomID{RoomArchive, RoomStudy, RoomMainHall},
		responses: []string{
			"I have one, yes.",
			"That depends on my mood.",
			"You’ll get an answer, not a treaty.",
		},
		easterEggs: []string{"state_tinted_preference"},
	},
	"what is your favorite room?": {
		mode:         "favorite_room",
		aliases:      []string{"where do you go when you think", "where do you go at night", "where do you go when you're tired"},
		destinations: []RoomID{RoomArchive, RoomStudy, RoomMainHall},
		responses: []string{
			"I have one, yes.",
			"That depends on my mood.",
			"You’ll get an answer, not a treaty.",
		},
		easterEggs: []string{"state_tinted_preference"},
	},
	"do something": {
		mode:         "micro_action",
		aliases:      []string{"do a little something", "show me a little magic"},
		destinations: []RoomID{RoomMainHall, RoomArchive, RoomTrophy},
		responses: []string{
			"Very well.",
			"I can manage something small.",
			"All right. No need to sound so mysterious about it.",
		},
		easterEggs: []string{"tiny_voiced_quip", "context_idle_choice"},
	},
	"go somewhere quiet": {
		mode:         "quiet_choice",
		aliases:      []string{"take me somewhere quiet", "show me something old", "take me somewhere cozy"},
		destinations: []RoomID{RoomArchive, RoomMainHall},
		responses: []string{
			"Somewhere quieter, then.",
			"Very well. I know a calmer corner.",
			"An excellent instinct. Let’s keep to the quieter rooms.",
		},
		easterEggs: []string{"archive_reflective_hush", "warm_home_glow"},
	},
	"go somewhere useful": {
		mode:         "useful_choice",
		aliases:      []string{"take me somewhere useful", "show me something useful"},
		destinations: []RoomID{RoomStudy, RoomWorkshop},
		responses: []string{
			"Somewhere useful, then.",
			"Very well. We’ll go where things get done.",
			"A sensible request. Usefulness is underrated.",
		},
		easterEggs: []string{"practical_stance_bias"},
	},
	"go somewhere impressive": {
		mode:         "impressive_choice",
		aliases:      []string{"take me somewhere impressive", "take me somewhere magical", "show me the best view"},
		destinations: []RoomID{RoomTrophy, RoomObservatory},
		responses: []string{
			"Somewhere impressive, then.",
			"Very well. I do have options.",
			"You want spectacle without nonsense. Admirable.",
		},
		easterEggs: []string{"trophy_gleam_or_horizon_check"},
		scenic:     true,
	},
	"go to the hall": {
		mode:         "named_destination",
		aliases:      []string{"take me to the hall", "head home"},
		destinations: []RoomID{RoomMainHall},
		responses: []string{
			"On my way to the Main Hall.",
			"Very well. To the Main Hall.",
			"You mean the Main Hall. I forgive the shorthand.",
		},
		easterEggs: []string{"hall_shorthand_correction"},
	},
	"after you": {
		mode:         "escort",
		aliases:      []string{"lead the way", "walk with me"},
		destinations: []RoomID{RoomMainHall, RoomArchive, RoomStudy},
		responses: []string{
			"After me, then.",
			"Very well. Keep up.",
			"I’ll lead. Do try to look intentional about it.",
		},
		easterEggs: []string{"ceremonial_first_step"},
		scenic:     true,
	},
}
