package main

import (
	"flag"
	"image"
	"image/png"
	"log"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"xmilo/castle-go/internal/game"
)

type previewGame struct {
	roomID    string
	ritual    string
	miloState string
	outPath   string
	width     int
	height    int
	saved     bool

	scene *game.RoomScene
	milo  *game.MiloAnimator
}

func newPreviewGame(roomID, ritual, miloState, outPath string, width, height int) *previewGame {
	cam := game.NewCamera(width, height)
	scene := game.NewRoomScene(cam)
	scene.SetActiveRoom(roomID)
	scene.SetMiloState(miloState)
	if ritual != "" {
		scene.SetRitualState(ritual)
	}

	milo := game.NewMiloAnimator(cam)
	milo.SetState(miloState, roomID, "s", true)

	return &previewGame{
		roomID:    roomID,
		ritual:    ritual,
		miloState: miloState,
		outPath:   outPath,
		width:     width,
		height:    height,
		scene:     scene,
		milo:      milo,
	}
}

func (g *previewGame) Update() error {
	if g.saved {
		return ebiten.Termination
	}
	g.scene.Tick()
	g.milo.Tick(game.NewCamera(g.width, g.height))
	return nil
}

func (g *previewGame) Draw(screen *ebiten.Image) {
	g.scene.Draw(screen, g.milo.ZOrder, func() {
		g.milo.Draw(screen)
	})

	if g.saved {
		return
	}

	pixels := make([]byte, 4*g.width*g.height)
	screen.ReadPixels(pixels)

	img := image.NewRGBA(image.Rect(0, 0, g.width, g.height))
	copy(img.Pix, pixels)

	file, err := os.Create(g.outPath)
	if err != nil {
		log.Fatalf("create output: %v", err)
	}
	defer file.Close()

	if err := png.Encode(file, img); err != nil {
		log.Fatalf("encode png: %v", err)
	}

	g.saved = true
}

func (g *previewGame) Layout(_, _ int) (int, int) {
	return g.width, g.height
}

func main() {
	roomID := flag.String("room", "main_hall", "room id to preview")
	ritual := flag.String("ritual", "", "ritual status: deferred|started|completed")
	miloState := flag.String("state", "idle", "milo visual state: idle|moving|working|thinking")
	outPath := flag.String("out", "castle-preview.png", "output PNG path")
	width := flag.Int("width", 393, "preview width")
	height := flag.Int("height", 851, "preview height")
	flag.Parse()

	ebiten.SetWindowSize(*width, *height)
	ebiten.SetWindowTitle("xMilo castle preview export")
	ebiten.SetRunnableOnUnfocused(true)

	if err := ebiten.RunGame(newPreviewGame(*roomID, *ritual, *miloState, *outPath, *width, *height)); err != nil && err != ebiten.Termination {
		log.Fatalf("render preview: %v", err)
	}

	log.Printf("preview written to %s", *outPath)
}
