// Manifest tool — prints the complete list of PNG files expected by castle-go.
// Run this and hand the output to your artist or image generation pipeline.
//
// Usage:
//   cd castle-go
//   go run ./tools/manifest
package main

import (
	"fmt"
	"xmilo/castle-go/internal/assets"
)

func main() {
	fmt.Println("=== Milo Sprite Sheets (sprites/milo/) ===")
	fmt.Println("Each file: 512 x 80 px horizontal strip, 8 frames of 64x80 px")
	fmt.Println("Character: wizard duck, isometric 2:1 angle, facing indicated by filename suffix")
	fmt.Println()

	states := []string{"idle", "walking", "talking", "thinking", "sleeping", "working", "reading", "stirring", "gazing"}
	facings := []string{"n", "s", "e", "w"}
	for _, s := range states {
		for _, f := range facings {
			fmt.Printf("  sprites/milo/%s_%s.png\n", s, f)
		}
	}

	fmt.Println()
	fmt.Println("=== Room Backgrounds (rooms/) ===")
	fmt.Println("Each file: 960 x 720 px, pre-rendered isometric room, no characters")
	fmt.Println()
	rooms := []struct {
		id   string
		desc string
	}{
		{"main_hall", "grand stone hall, throne on raised dais, two banners, lit fireplace"},
		{"war_room", "dark tactical room, large map table, wall maps, flags"},
		{"library", "tall bookshelves, reading desk with lamp, arcane tomes"},
		{"training_room", "open floor with mat, training dummy, weapon rack"},
		{"spellbook", "intimate study, glowing spellbook on stand, scroll shelves, candles"},
		{"cauldron", "stone chamber, large bubbling cauldron over fire pit, ingredient shelves"},
		{"crystal_orb", "circular room, crystal orb on plinth glowing, star map on wall, rune circle on floor"},
		{"baby_dragon", "warm cave-like room, high perch, toy pile, small gem hoard"},
		{"trophy", "hall of fame, trophy cases, victory banners, achievement plaques"},
		{"archive", "deep archive, rows of scroll shelves, lectern, floating memory crystal"},
	}
	for _, r := range rooms {
		fmt.Printf("  rooms/%s.png\n    ↳ %s\n", r.id, r.desc)
	}

	fmt.Println()
	fmt.Println("=== Full manifest from assets package ===")
	for _, p := range assets.AssetManifest() {
		fmt.Println(" ", p)
	}
}
