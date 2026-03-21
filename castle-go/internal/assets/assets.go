package assets

import (
	"bytes"
	"embed"
	"image"
	"image/color"
	"image/png"
	"log"
	"path"

	"github.com/hajimehoshi/ebiten/v2"
)

// fs holds all sprite sheets and room backgrounds.
// Files are placed under assets/sprites/ and assets/rooms/.
// The embed pattern matches all PNG files recursively.
// If a PNG does not exist yet, LoadImage falls back to a colored placeholder
// so the game always runs during asset development.
//
//go:embed sprites rooms
var fs embed.FS

// placeholderColors maps asset path prefixes to placeholder colors.
// This lets you visually distinguish Milo states and rooms before real art lands.
var placeholderColors = map[string]color.RGBA{
	"sprites/milo/idle":      {R: 255, G: 220, B: 100, A: 255}, // warm yellow — resting duck
	"sprites/milo/walking":   {R: 100, G: 200, B: 255, A: 255}, // blue — movement
	"sprites/milo/talking":   {R: 255, G: 150, B: 100, A: 255}, // orange — speech
	"sprites/milo/thinking":  {R: 180, G: 100, B: 255, A: 255}, // purple — cognition
	"sprites/milo/sleeping":  {R: 100, G: 130, B: 180, A: 255}, // slate — rest
	"sprites/milo/working":   {R: 80, G: 200, B: 120, A: 255},  // green — active work
	"sprites/milo/reading":   {R: 200, G: 170, B: 100, A: 255}, // parchment
	"sprites/milo/stirring":  {R: 180, G: 60, B: 180, A: 255},  // magenta — cauldron
	"sprites/milo/gazing":    {R: 60, G: 180, B: 200, A: 255},  // cyan — orb gaze
	"rooms/main_hall":        {R: 60, G: 60, B: 80, A: 255},
	"rooms/war_room":         {R: 80, G: 50, B: 50, A: 255},
	"rooms/library":          {R: 60, G: 70, B: 50, A: 255},
	"rooms/training_room":    {R: 50, G: 60, B: 80, A: 255},
	"rooms/spellbook":        {R: 70, G: 50, B: 80, A: 255},
	"rooms/cauldron":         {R: 50, G: 70, B: 50, A: 255},
	"rooms/crystal_orb":      {R: 40, G: 60, B: 90, A: 255},
	"rooms/baby_dragon":      {R: 80, G: 60, B: 40, A: 255},
	"rooms/trophy":           {R: 80, G: 70, B: 30, A: 255},
	"rooms/archive":          {R: 50, G: 50, B: 60, A: 255},
}

const (
	MiloFrameW = 64  // pixels per sprite frame, width
	MiloFrameH = 80  // pixels per sprite frame, height
	MiloFrames = 8   // frames per animation row
	RoomBGW    = 960 // room background width (portrait, ~2.5x tile grid)
	RoomBGH    = 720 // room background height
)

// LoadMiloSheet loads "sprites/milo/{state}_{facing}.png".
// Returns a placeholder colored rectangle if the file is missing.
// Each PNG is a horizontal sprite sheet: MiloFrames columns × 1 row.
// Sheet dimensions: (MiloFrameW * MiloFrames) × MiloFrameH
func LoadMiloSheet(state, facing string) *ebiten.Image {
	p := path.Join("sprites", "milo", state+"_"+facing+".png")
	return loadOrPlaceholder(p, MiloFrameW*MiloFrames, MiloFrameH, "sprites/milo/"+state)
}

// LoadRoomBackground loads "rooms/{roomID}.png".
// Returns a placeholder if the file is missing.
func LoadRoomBackground(roomID string) *ebiten.Image {
	p := path.Join("rooms", roomID+".png")
	return loadOrPlaceholder(p, RoomBGW, RoomBGH, "rooms/"+roomID)
}

// LoadPropSprite loads "sprites/props/{propKey}.png".
func LoadPropSprite(propKey string) *ebiten.Image {
	p := path.Join("sprites", "props", propKey+".png")
	return loadOrPlaceholder(p, 64, 128, "sprites/props/"+propKey)
}

func loadOrPlaceholder(filePath string, w, h int, colorKey string) *ebiten.Image {
	data, err := fs.ReadFile(filePath)
	if err == nil {
		img, _, err2 := image.Decode(bytes.NewReader(data))
		if err2 == nil {
			return ebiten.NewImageFromImage(img)
		}
		log.Printf("assets: decode %s: %v", filePath, err2)
	}
	// file not found or decode error — return a colored placeholder
	return makePlaceholder(w, h, resolveColor(colorKey))
}

func makePlaceholder(w, h int, c color.RGBA) *ebiten.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	border := color.RGBA{R: c.R / 2, G: c.G / 2, B: c.B / 2, A: 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < 2 || x >= w-2 || y < 2 || y >= h-2 {
				img.Set(x, y, border)
			} else {
				img.Set(x, y, c)
			}
		}
	}
	return ebiten.NewImageFromImage(img)
}

func resolveColor(key string) color.RGBA {
	for prefix, c := range placeholderColors {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			return c
		}
	}
	return color.RGBA{R: 80, G: 80, B: 80, A: 255} // fallback gray
}

// PreEncodeAssetManifest writes a PNG manifest to stdout for use by
// the art pipeline to know exactly what files are expected.
// Call with: go run ./tools/manifest
func AssetManifest() []string {
	states := []string{"idle", "walking", "talking", "thinking", "sleeping", "working", "reading", "stirring", "gazing"}
	facings := []string{"n", "s", "e", "w"}
	rooms := []string{"main_hall", "war_room", "library", "training_room", "spellbook", "cauldron", "crystal_orb", "baby_dragon", "trophy", "archive"}
	var paths []string
	for _, s := range states {
		for _, f := range facings {
			paths = append(paths, "sprites/milo/"+s+"_"+f+".png")
		}
	}
	for _, r := range rooms {
		paths = append(paths, "rooms/"+r+".png")
	}
	return paths
}
