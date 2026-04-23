// Package rooms maps task intent to a room and anchor in the Wizard Lair.
// Existing room IDs (main_hall, war_room, library, training_room) are unchanged.
// New Wizard Lair rooms added below — additive only.
package rooms

import "strings"

type Route struct {
	Intent   string
	RoomID   string
	AnchorID string
}

func Resolve(prompt, intentHint string) Route {
	intent := strings.ToLower(strings.TrimSpace(intentHint))
	lower := strings.ToLower(prompt)

	if intent == "" {
		switch {
		case strings.Contains(lower, "plan"), strings.Contains(lower, "priorit"), strings.Contains(lower, "strateg"):
			intent = "planning"
		case strings.Contains(lower, "summar"), strings.Contains(lower, "read"), strings.Contains(lower, "explain"), strings.Contains(lower, "analyz"):
			intent = "analysis"
		case strings.Contains(lower, "practice"), strings.Contains(lower, "learn"), strings.Contains(lower, "train"):
			intent = "training"
		case strings.Contains(lower, "hello"), strings.Contains(lower, "hi "), strings.Contains(lower, "how are you"), strings.Contains(lower, "what's up"):
			intent = "casual_conversation"
		case strings.Contains(lower, "spell"), strings.Contains(lower, "creat"), strings.Contains(lower, "writ"), strings.Contains(lower, "compos"):
			intent = "creative"
		case strings.Contains(lower, "brew"), strings.Contains(lower, "mix"), strings.Contains(lower, "combin"), strings.Contains(lower, "process"):
			intent = "processing"
		case strings.Contains(lower, "predict"), strings.Contains(lower, "forecast"), strings.Contains(lower, "vision"), strings.Contains(lower, "future"):
			intent = "prediction"
		case strings.Contains(lower, "remember"), strings.Contains(lower, "recall"), strings.Contains(lower, "histor"), strings.Contains(lower, "log"):
			intent = "memory"
		default:
			intent = "general"
		}
	}

	switch intent {
	// Original rooms — behavior frozen
	case "planning":
		return Route{Intent: intent, RoomID: "war_room", AnchorID: "war_room_table"}
	case "analysis":
		return Route{Intent: intent, RoomID: "library", AnchorID: "library_desk"}
	case "training":
		return Route{Intent: intent, RoomID: "training_room", AnchorID: "training_circle"}

	// New Wizard Lair rooms
	case "creative":
		return Route{Intent: intent, RoomID: "spellbook", AnchorID: "spellbook_reading"}
	case "processing":
		return Route{Intent: intent, RoomID: "cauldron", AnchorID: "cauldron_stir"}
	case "prediction":
		return Route{Intent: intent, RoomID: "crystal_orb", AnchorID: "crystal_orb_stand"}
	case "memory":
		return Route{Intent: intent, RoomID: "archive", AnchorID: "archive_lectern"}

	default:
		return Route{Intent: intent, RoomID: "main_hall", AnchorID: "main_hall_center"}
	}
}
