package fixtures

import (
	"encoding/json"
	"fmt"

	"xmilo/castle-go/internal/client"
	"xmilo/castle-go/internal/game"
)

type Step struct {
	Label   string
	Ticks   int
	Capture bool
	Event   *client.RawEvent
}

type Sequence struct {
	Name        string
	Description string
	Steps       []Step
}

func AcceptanceNames() []string {
	return []string{
		"main_hall_arrival",
		"lair_work_cycle",
		"nightly_ritual_cycle",
	}
}

func Named(name string) (Sequence, error) {
	switch name {
	case "main_hall_arrival":
		return mainHallArrival(), nil
	case "lair_work_cycle":
		return lairWorkCycle(), nil
	case "nightly_ritual_cycle":
		return nightlyRitualCycle(), nil
	case "threshold_route_demo":
		return thresholdRouteDemo(), nil
	case "threshold_variant_demo":
		return thresholdVariantDemo(), nil
	case "home_idle_demo":
		return homeIdleDemo(), nil
	case "arrival_main_hall":
		return arrivalMainHall(), nil
	case "arrival_archive":
		return arrivalArchive(), nil
	case "arrival_study":
		return arrivalStudy(), nil
	case "arrival_observatory":
		return arrivalObservatory(), nil
	case "arrival_workshop":
		return arrivalWorkshop(), nil
	case "arrival_potions":
		return arrivalPotions(), nil
	default:
		return Sequence{}, fmt.Errorf("unknown fixture sequence %q", name)
	}
}

func Names() []string {
	return append(append(append(AcceptanceNames(), "threshold_route_demo", "threshold_variant_demo", "home_idle_demo"), "arrival_main_hall", "arrival_archive", "arrival_study", "arrival_observatory"), "arrival_workshop", "arrival_potions")
}

func mainHallArrival() Sequence {
	return Sequence{
		Name:        "main_hall_arrival",
		Description: "Validates return-to-hall presence and report arrival readability.",
		Steps: []Step{
			{Label: "01_idle_main_hall", Ticks: 18, Capture: true},
			{
				Label:   "02_report_return",
				Ticks:   24,
				Capture: true,
				Event: rawEvent("milo.movement_started", game.MovementStarted{
					FromRoom:    "library",
					FromAnchor:  "library_desk",
					ToRoom:      "main_hall",
					ToAnchor:    "main_hall_center",
					Reason:      "report",
					EstimatedMS: 900,
				}),
			},
			{
				Label:   "03_room_settled",
				Ticks:   28,
				Capture: true,
				Event: rawEvent("milo.room_changed", game.RoomChanged{
					RoomID:   "main_hall",
					AnchorID: "main_hall_center",
				}),
			},
			{
				Label:   "04_idle_after_report",
				Ticks:   28,
				Capture: true,
				Event: rawEvent("milo.state_changed", game.StateChanged{
					FromState: "moving",
					ToState:   "idle",
				}),
			},
		},
	}
}

func lairWorkCycle() Sequence {
	return Sequence{
		Name:        "lair_work_cycle",
		Description: "Validates lair-adjacent task departure, arrival, and working-state expression.",
		Steps: []Step{
			{Label: "01_main_hall_ready", Ticks: 18, Capture: true},
			{
				Label:   "02_depart_for_library",
				Ticks:   24,
				Capture: true,
				Event: rawEvent("milo.movement_started", game.MovementStarted{
					FromRoom:    "main_hall",
					FromAnchor:  "main_hall_center",
					ToRoom:      "library",
					ToAnchor:    "library_desk",
					Reason:      "task_start",
					EstimatedMS: 1100,
				}),
			},
			{
				Label:   "03_library_arrival",
				Ticks:   24,
				Capture: true,
				Event: rawEvent("milo.room_changed", game.RoomChanged{
					RoomID:   "library",
					AnchorID: "library_desk",
				}),
			},
			{
				Label:   "04_library_working",
				Ticks:   32,
				Capture: true,
				Event: rawEvent("milo.state_changed", game.StateChanged{
					FromState: "moving",
					ToState:   "working",
				}),
			},
		},
	}
}

func nightlyRitualCycle() Sequence {
	return Sequence{
		Name:        "nightly_ritual_cycle",
		Description: "Validates deferred, started, and completed nightly upkeep ritual states.",
		Steps: []Step{
			{
				Label:   "01_archive_working",
				Ticks:   22,
				Capture: true,
				Event: rawEvent("milo.room_changed", game.RoomChanged{
					RoomID:   "archive",
					AnchorID: "archive_lectern",
				}),
			},
			{
				Label:   "02_working_state",
				Ticks:   26,
				Capture: true,
				Event: rawEvent("milo.state_changed", game.StateChanged{
					FromState: "idle",
					ToState:   "working",
				}),
			},
			{
				Label:   "03_ritual_deferred",
				Ticks:   34,
				Capture: true,
				Event: rawEvent("maintenance.nightly_deferred", game.NightlyMaintenanceDeferred{
					ArchiveDate: "2026-03-23",
					Reason:      "active_task",
					TaskID:      "fixture-task",
					Message:     "Nightly upkeep waits until the current task is complete.",
				}),
			},
			{
				Label:   "04_ritual_started",
				Ticks:   40,
				Capture: true,
				Event: rawEvent("maintenance.nightly_started", game.NightlyMaintenanceStarted{
					ArchiveDate:       "2026-03-23",
					Trigger:           "after_task_completion",
					StartedAt:         "2026-03-23T02:14:00-04:00",
					LocalTime:         "2:14 AM",
					LatestReleaseTag:  "v0.14.0",
					LatestReleaseURL:  "https://github.com/Hatsunama/xmilo_at_your_side/releases/tag/v0.14.0",
					UpdateCheckStatus: "checked",
					VoiceCue:          "The observatory awakens for nightly upkeep.",
					PhysicalCue:       "Milo turns toward the archive dais as the crystal brightens.",
				}),
			},
			{
				Label:   "05_ritual_completed",
				Ticks:   46,
				Capture: true,
				Event: rawEvent("maintenance.nightly_completed", game.NightlyMaintenanceCompleted{
					ArchiveDate:       "2026-03-23",
					Trigger:           "after_task_completion",
					CompletedAt:       "2026-03-23T02:16:00-04:00",
					TaskCount:         4,
					LatestReleaseTag:  "v0.14.0",
					LatestReleaseURL:  "https://github.com/Hatsunama/xmilo_at_your_side/releases/tag/v0.14.0",
					UpdateCheckStatus: "checked",
					VoiceCue:          "The archive is sealed for the night.",
					PhysicalCue:       "The observatory glow settles and the chamber returns to rest.",
					Message:           "Nightly upkeep complete. Archive sealed.",
				}),
			},
		},
	}
}

func thresholdRouteDemo() Sequence {
	return Sequence{
		Name:        "threshold_route_demo",
		Description: "Validates corridor reveal when travel requires visible Threshold hall framing.",
		Steps: []Step{
			{
				Label:   "01_archive_depart",
				Ticks:   20,
				Capture: true,
				Event: rawEvent("milo.room_changed", game.RoomChanged{
					RoomID:   "archive",
					AnchorID: "archive_lectern",
				}),
			},
			{
				Label:   "02_observatory_route_start",
				Ticks:   30,
				Capture: true,
				Event: rawEvent("milo.movement_started", game.MovementStarted{
					FromRoom:    "archive",
					FromAnchor:  "archive_lectern",
					ToRoom:      "crystal_orb",
					ToAnchor:    "crystal_orb_watch",
					Reason:      "ritual_transfer",
					EstimatedMS: 1200,
				}),
			},
			{
				Label:   "03_observatory_arrive",
				Ticks:   24,
				Capture: true,
				Event: rawEvent("milo.room_changed", game.RoomChanged{
					RoomID:   "crystal_orb",
					AnchorID: "crystal_orb_watch",
				}),
			},
		},
	}
}

func homeIdleDemo() Sequence {
	return Sequence{
		Name:        "home_idle_demo",
		Description: "Validates first-pass hybrid idle loops across the home cluster.",
		Steps: []Step{
			{
				Label:   "01_main_hall_idle_start",
				Ticks:   120,
				Capture: true,
				Event: rawEvent("milo.room_changed", game.RoomChanged{
					RoomID:   "main_hall",
					AnchorID: "main_hall_center",
				}),
			},
			{
				Label:   "02_main_hall_idle_variant",
				Ticks:   220,
				Capture: true,
			},
			{
				Label:   "03_archive_idle_variant",
				Ticks:   150,
				Capture: true,
				Event: rawEvent("milo.room_changed", game.RoomChanged{
					RoomID:   "archive",
					AnchorID: "archive_lectern",
				}),
			},
			{
				Label:   "04_trophy_idle_variant",
				Ticks:   150,
				Capture: true,
				Event: rawEvent("milo.room_changed", game.RoomChanged{
					RoomID:   "trophy",
					AnchorID: "trophy_display",
				}),
			},
		},
	}
}

func arrivalMainHall() Sequence {
	return Sequence{
		Name:        "arrival_main_hall",
		Description: "Main Hall arrival settle validation.",
		Steps: []Step{
			{
				Label:   "01_main_hall_arrive_center",
				Ticks:   64,
				Capture: true,
				Event: rawEvent("milo.room_changed", game.RoomChanged{
					RoomID:   "main_hall",
					AnchorID: "main_hall_center",
				}),
			},
			{
				Label:   "02_idle_home_ready",
				Ticks:   48,
				Capture: true,
				Event: rawEvent("milo.state_changed", game.StateChanged{
					FromState: "moving",
					ToState:   "idle",
				}),
			},
		},
	}
}

func arrivalArchive() Sequence {
	return Sequence{
		Name:        "arrival_archive",
		Description: "Archive arrival settle validation near lectern/crystal.",
		Steps: []Step{
			{
				Label:   "01_archive_arrive",
				Ticks:   64,
				Capture: true,
				Event: rawEvent("milo.room_changed", game.RoomChanged{
					RoomID:   "archive",
					AnchorID: "archive_lectern",
				}),
			},
			{
				Label:   "02_archive_idle_calm",
				Ticks:   48,
				Capture: true,
				Event: rawEvent("milo.state_changed", game.StateChanged{
					FromState: "moving",
					ToState:   "idle",
				}),
			},
		},
	}
}

func arrivalStudy() Sequence {
	return Sequence{
		Name:        "arrival_study",
		Description: "Study arrival settle near desk/shelf.",
		Steps: []Step{
			{
				Label:   "01_study_arrive",
				Ticks:   64,
				Capture: true,
				Event: rawEvent("milo.room_changed", game.RoomChanged{
					RoomID:   "library",
					AnchorID: "library_desk",
				}),
			},
			{
				Label:   "02_study_idle_focus",
				Ticks:   48,
				Capture: true,
				Event: rawEvent("milo.state_changed", game.StateChanged{
					FromState: "moving",
					ToState:   "idle",
				}),
			},
		},
	}
}

func arrivalObservatory() Sequence {
	return Sequence{
		Name:        "arrival_observatory",
		Description: "Observatory arrival settle near orb.",
		Steps: []Step{
			{
				Label:   "01_observatory_arrive",
				Ticks:   64,
				Capture: true,
				Event: rawEvent("milo.room_changed", game.RoomChanged{
					RoomID:   "crystal_orb",
					AnchorID: "crystal_orb_watch",
				}),
			},
			{
				Label:   "02_observatory_idle_outward",
				Ticks:   48,
				Capture: true,
				Event: rawEvent("milo.state_changed", game.StateChanged{
					FromState: "moving",
					ToState:   "idle",
				}),
			},
		},
	}
}

func arrivalWorkshop() Sequence {
	return Sequence{
		Name:        "arrival_workshop",
		Description: "Workshop arrival settle near table/toolbench.",
		Steps: []Step{
			{
				Label:   "01_workshop_arrive",
				Ticks:   64,
				Capture: true,
				Event: rawEvent("milo.room_changed", game.RoomChanged{
					RoomID:   "war_room",
					AnchorID: "war_room_table",
				}),
			},
			{
				Label:   "02_workshop_idle_practical",
				Ticks:   48,
				Capture: true,
				Event: rawEvent("milo.state_changed", game.StateChanged{
					FromState: "moving",
					ToState:   "idle",
				}),
			},
		},
	}
}

func arrivalPotions() Sequence {
	return Sequence{
		Name:        "arrival_potions",
		Description: "Potions Room arrival settle near cauldron.",
		Steps: []Step{
			{
				Label:   "01_potions_arrive",
				Ticks:   64,
				Capture: true,
				Event: rawEvent("milo.room_changed", game.RoomChanged{
					RoomID:   "cauldron",
					AnchorID: "cauldron_stir",
				}),
			},
			{
				Label:   "02_potions_idle_curious",
				Ticks:   48,
				Capture: true,
				Event: rawEvent("milo.state_changed", game.StateChanged{
					FromState: "moving",
					ToState:   "idle",
				}),
			},
		},
	}
}

func thresholdVariantDemo() Sequence {
	return Sequence{
		Name:        "threshold_variant_demo",
		Description: "Validates curated hall-route variation across repeated cross-cluster travel families.",
		Steps: []Step{
			{
				Label:   "01_main_hall_depart",
				Ticks:   20,
				Capture: true,
				Event: rawEvent("milo.room_changed", game.RoomChanged{
					RoomID:   "main_hall",
					AnchorID: "main_hall_center",
				}),
			},
			{
				Label:   "02_study_route_start",
				Ticks:   28,
				Capture: true,
				Event: rawEvent("milo.movement_started", game.MovementStarted{
					FromRoom:    "main_hall",
					FromAnchor:  "main_hall_center",
					ToRoom:      "library",
					ToAnchor:    "library_desk",
					Reason:      "curated_route_a",
					EstimatedMS: 1100,
				}),
			},
			{
				Label:   "03_archive_depart",
				Ticks:   22,
				Capture: true,
				Event: rawEvent("milo.room_changed", game.RoomChanged{
					RoomID:   "archive",
					AnchorID: "archive_lectern",
				}),
			},
			{
				Label:   "04_observatory_route_variant",
				Ticks:   28,
				Capture: true,
				Event: rawEvent("milo.movement_started", game.MovementStarted{
					FromRoom:    "archive",
					FromAnchor:  "archive_lectern",
					ToRoom:      "crystal_orb",
					ToAnchor:    "crystal_orb_watch",
					Reason:      "curated_route_b",
					EstimatedMS: 1200,
				}),
			},
		},
	}
}

func rawEvent(eventType string, payload any) *client.RawEvent {
	body, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return &client.RawEvent{
		Type:      eventType,
		Timestamp: "2026-03-23T00:00:00Z",
		Payload:   body,
	}
}
