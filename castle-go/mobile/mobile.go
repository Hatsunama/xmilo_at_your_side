//go:build android || ios

package mobile

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/mobile/ebitenmobileview"
	"xmilo/castle-go/internal/game"
	_ "xmilo/castle-go/internal/assets"
)

// Start initializes the production castle renderer for the supplied websocket URL.
func Start(wsURL string) {
	ebitenmobileview.SetGame(game.NewGame(wsURL), &ebiten.RunGameOptions{})
}
