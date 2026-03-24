package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"xmilo/castle-go/internal/game"
)

type promptSample struct {
	Prompt   string                  `json:"prompt"`
	Variants []game.DirectedMovePlan `json:"variants"`
}

type behaviorReport struct {
	Samples []promptSample `json:"samples"`
}

func main() {
	outPath := flag.String("out", "behavior-report.json", "output JSON report path")
	flag.Parse()

	director := game.NewBehaviorDirector()
	samples := []string{
		"go to the archive",
		"go to the trophy room",
		"go to the observatory",
		"go to the workshop",
		"go to the potions room",
		"take me somewhere",
		"show me around",
		"walk around",
		"patrol the castle",
		"take the scenic route",
		"show me something cool",
		"where would you go?",
		"what's your favorite room?",
		"surprise me",
		"do something",
		"take me somewhere quiet",
		"take me somewhere useful",
		"take me somewhere impressive",
		"head home",
		"after you",
	}

	report := behaviorReport{}
	for _, prompt := range samples {
		entry := promptSample{Prompt: prompt}
		currentRoom := game.RoomMainHall
		currentState := "idle"
		for i := 0; i < 3; i++ {
			plan, ok := director.Plan(prompt, currentRoom, currentState)
			if !ok {
				continue
			}
			entry.Variants = append(entry.Variants, plan)
			currentRoom = plan.Destination
		}
		if len(entry.Variants) > 0 {
			report.Samples = append(report.Samples, entry)
		}
	}

	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*outPath, body, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("behavior report written: %s", *outPath)
}
