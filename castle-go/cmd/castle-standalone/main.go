// Standalone desktop/Android test binary.
// Build for desktop:    go run ./cmd/castle-standalone
// Build for Android APK (test only, not production):
//   gomobile build -target android -androidapi 21 ./cmd/castle-standalone
//
// Connects to PicoClaw at ws://127.0.0.1:42817/ws by default.
// Override with CASTLE_WS_URL env var.
package main

import (
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"xmilo/castle-go/internal/game"
)

func main() {
	wsURL := os.Getenv("CASTLE_WS_URL")
	if wsURL == "" {
		wsURL = "ws://127.0.0.1:42817/ws"
	}

	ebiten.SetWindowSize(393, 851) // Pixel 7 portrait
	ebiten.SetWindowTitle("Milo Wizard Lair")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)

	if err := ebiten.RunGame(game.NewGame(wsURL)); err != nil {
		log.Fatalf("castle: %v", err)
	}
}
