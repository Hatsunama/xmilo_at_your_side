package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"xmilo/castle-go/internal/fixtures"
	"xmilo/castle-go/internal/game"
)

type fixturePreviewGame struct {
	sequence fixtures.Sequence
	outDir   string
	width    int
	height   int

	game        *game.Game
	stepIndex   int
	stepTick    int
	captureTick bool
	finished    bool
}

type fixtureReport struct {
	Suite     string         `json:"suite"`
	Width     int            `json:"width"`
	Height    int            `json:"height"`
	Fixtures  []fixtureEntry `json:"fixtures"`
	Generated string         `json:"generated"`
}

type fixtureEntry struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Frames      []string `json:"frames"`
}

func newFixturePreviewGame(sequence fixtures.Sequence, outDir string, width, height int) *fixturePreviewGame {
	return &fixturePreviewGame{
		sequence: sequence,
		outDir:   outDir,
		width:    width,
		height:   height,
		game:     game.NewOfflineGame(),
	}
}

func (g *fixturePreviewGame) Update() error {
	if g.finished {
		return ebiten.Termination
	}

	if g.stepIndex >= len(g.sequence.Steps) {
		g.finished = true
		return nil
	}

	step := g.sequence.Steps[g.stepIndex]
	if g.stepTick == 0 && step.Event != nil {
		g.game.ApplyRawEvent(*step.Event)
	}

	if err := g.game.Update(); err != nil {
		return err
	}

	g.stepTick++
	if step.Capture && g.stepTick >= step.Ticks {
		g.captureTick = true
	}
	if g.stepTick >= step.Ticks {
		g.stepIndex++
		g.stepTick = 0
	}

	return nil
}

func (g *fixturePreviewGame) Draw(screen *ebiten.Image) {
	g.game.Draw(screen)

	if !g.captureTick {
		return
	}

	step := g.sequence.Steps[g.stepIndex-1]
	fileName := fmt.Sprintf("%02d_%s.png", g.stepIndex, sanitize(step.Label))
	outPath := filepath.Join(g.outDir, fileName)

	pixels := make([]byte, 4*g.width*g.height)
	screen.ReadPixels(pixels)

	img := image.NewRGBA(image.Rect(0, 0, g.width, g.height))
	copy(img.Pix, pixels)

	file, err := os.Create(outPath)
	if err != nil {
		log.Fatalf("create output %s: %v", outPath, err)
	}
	defer file.Close()

	if err := png.Encode(file, img); err != nil {
		log.Fatalf("encode output %s: %v", outPath, err)
	}

	log.Printf("fixture frame written: %s", outPath)
	g.captureTick = false
}

func (g *fixturePreviewGame) Layout(_, _ int) (int, int) {
	g.game.Layout(g.width, g.height)
	return g.width, g.height
}

func sanitize(label string) string {
	return strings.ReplaceAll(strings.ToLower(label), " ", "_")
}

func main() {
	name := flag.String("fixture", "nightly_ritual_cycle", "fixture sequence to render")
	outDir := flag.String("outdir", "fixture-previews", "output directory for PNG frames")
	width := flag.Int("width", 393, "frame width")
	height := flag.Int("height", 851, "frame height")
	flag.Parse()

	if *name == "list" {
		fmt.Println(strings.Join(fixtures.Names(), "\n"))
		return
	}

	if *name == "acceptance" {
		if err := runAcceptanceSuite(*outDir, *width, *height); err != nil {
			log.Fatalf("render acceptance suite: %v", err)
		}
		log.Printf("acceptance fixture suite complete: %s", *outDir)
		return
	}

	ebiten.SetWindowSize(*width, *height)
	ebiten.SetWindowTitle("xMilo castle fixture preview")
	ebiten.SetRunnableOnUnfocused(true)

	sequence, err := fixtures.Named(*name)
	if err != nil {
		log.Fatal(err)
	}

	targetDir := filepath.Join(*outDir, sequence.Name)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
	}

	if err := ebiten.RunGame(newFixturePreviewGame(sequence, targetDir, *width, *height)); err != nil && err != ebiten.Termination {
		log.Fatalf("render fixture: %v", err)
	}

	log.Printf("fixture render complete: %s", targetDir)
}

func runAcceptanceSuite(outDir string, width, height int) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}

	report := fixtureReport{
		Suite:     "acceptance",
		Width:     width,
		Height:    height,
		Generated: time.Now().Format(time.RFC3339),
	}

	for _, fixtureName := range fixtures.AcceptanceNames() {
		sequence, err := fixtures.Named(fixtureName)
		if err != nil {
			return err
		}
		cmd := exec.Command(exePath,
			"-fixture", fixtureName,
			"-outdir", outDir,
			"-width", strconv.Itoa(width),
			"-height", strconv.Itoa(height),
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("%s: %w", fixtureName, err)
		}

		report.Fixtures = append(report.Fixtures, fixtureEntry{
			Name:        sequence.Name,
			Description: sequence.Description,
			Frames:      framePaths(outDir, sequence),
		})
	}

	return writeAcceptanceReport(outDir, report)
}

func framePaths(outDir string, sequence fixtures.Sequence) []string {
	paths := make([]string, 0, len(sequence.Steps))
	for index, step := range sequence.Steps {
		if !step.Capture {
			continue
		}
		paths = append(paths, filepath.Join(outDir, sequence.Name, fmt.Sprintf("%02d_%s.png", index+1, sanitize(step.Label))))
	}
	return paths
}

func writeAcceptanceReport(outDir string, report fixtureReport) error {
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "acceptance-report.json"), body, 0o644)
}
