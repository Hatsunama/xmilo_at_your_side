package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"xmilo/castle-go/internal/game"
)

type topologyReport struct {
	Rooms          map[string]topologyRoom     `json:"rooms"`
	SafeIdle       []string                    `json:"safe_idle_rooms"`
	LaunchLive     []string                    `json:"launch_live_rooms"`
	RitualRoute    []game.RouteStep            `json:"nightly_ritual_route"`
	SampleRoutes   map[string][]game.RouteStep `json:"sample_routes"`
	HallVariants   map[string][]string         `json:"hall_variants"`
	VariantSamples map[string][]game.RouteStep `json:"variant_samples"`
	PromptTargets  map[string]string           `json:"prompt_targets"`
}

type topologyRoom struct {
	DisplayName string   `json:"display_name"`
	Cluster     string   `json:"cluster"`
	LaunchLive  bool     `json:"launch_live"`
	SafeIdle    bool     `json:"safe_idle"`
	Neighbors   []string `json:"neighbors"`
	MapX        int      `json:"map_x"`
	MapY        int      `json:"map_y"`
}

func main() {
	outPath := flag.String("out", "topology-report.json", "output JSON report path")
	flag.Parse()

	report := topologyReport{
		Rooms:          map[string]topologyRoom{},
		SafeIdle:       roomIDsToStrings(game.SafeIdleRooms()),
		LaunchLive:     roomIDsToStrings(game.LaunchLiveRooms()),
		RitualRoute:    game.RouteBetween("archive", "observatory"),
		SampleRoutes:   map[string][]game.RouteStep{},
		HallVariants:   map[string][]string{},
		VariantSamples: map[string][]game.RouteStep{},
		PromptTargets:  map[string]string{},
	}

	for roomID, topo := range game.RoomGraph() {
		report.Rooms[string(roomID)] = topologyRoom{
			DisplayName: topo.DisplayName,
			Cluster:     topo.Cluster,
			LaunchLive:  topo.LaunchLive,
			SafeIdle:    topo.SafeIdle,
			Neighbors:   roomIDsToStrings(topo.Neighbors),
			MapX:        topo.MapX,
			MapY:        topo.MapY,
		}
	}

	report.SampleRoutes["main_hall_to_archive"] = game.RouteBetween("main_hall", "archive")
	report.SampleRoutes["main_hall_to_observatory"] = game.RouteBetween("main_hall", "observatory")
	report.SampleRoutes["archive_to_observatory"] = game.RouteBetween("archive", "observatory")
	report.SampleRoutes["workshop_to_trophy_room"] = game.RouteBetween("workshop", "trophy_room")
	report.HallVariants["home_knowledge"] = game.HallVariantsForRoute("main_hall", "observatory")
	report.HallVariants["home_making"] = game.HallVariantsForRoute("main_hall", "workshop")
	report.HallVariants["knowledge_making"] = game.HallVariantsForRoute("observatory", "workshop")
	report.VariantSamples["main_hall_to_study_first"] = game.RouteBetweenVariant("main_hall", "study", report.HallVariants["home_knowledge"][0])
	report.VariantSamples["archive_to_observatory_second"] = game.RouteBetweenVariant("archive", "observatory", report.HallVariants["home_knowledge"][1])
	report.PromptTargets["go_to_archive"] = string(game.RoomArchive)
	report.PromptTargets["go_to_trophy_room"] = string(game.RoomTrophy)
	report.PromptTargets["go_to_study"] = string(game.RoomStudy)
	report.PromptTargets["go_to_workshop"] = string(game.RoomWorkshop)
	report.PromptTargets["go_to_observatory"] = string(game.RoomObservatory)
	report.PromptTargets["go_to_potions_room"] = string(game.RoomPotions)

	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*outPath, body, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("topology report written: %s", *outPath)
}

func roomIDsToStrings(roomIDs []game.RoomID) []string {
	values := make([]string, 0, len(roomIDs))
	for _, roomID := range roomIDs {
		values = append(values, string(roomID))
	}
	return values
}
