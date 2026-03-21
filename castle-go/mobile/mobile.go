// Package mobile is the gomobile bind target for castle-go.
// Build with:
//   ebitenmobile bind -target android -androidapi 21 -javapkg com.xmilo.castle -o castle.aar ./mobile
//
// The resulting castle.aar is placed in android/app/libs/ and loaded by
// the CastleModule React Native NativeModule.
package mobile

import (
	"github.com/hajimehoshi/ebiten/v2/mobile"
	"xmilo/castle-go/internal/game"
)

// Start initializes the Ebiten castle scene and connects to PicoClaw.
// wsURL should be "ws://127.0.0.1:42817/ws" on device.
// This must be called from the Android NativeModule before the SurfaceView is attached.
func Start(wsURL string) {
	mobile.SetGame(game.NewGame(wsURL))
}
