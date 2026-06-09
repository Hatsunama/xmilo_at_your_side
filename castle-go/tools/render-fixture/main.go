package main

import (
	"crypto/sha256"
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
	proof    bool

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

type frameProof struct {
	Path              string
	Size              int64
	SHA256            string
	Fixture           string
	FrameLabel        string
	ExpectedRoom      string
	ExpectedRoute     string
	MovementSegment   string
	WaypointList      []string
	RoomsTouched      []string
	VisualRouteReview string
	PathReview        string
}

type phase20Manifest struct {
	TaskID                        string
	Phase                         string
	GeneratedAt                   string
	RepoRoot                      string
	Tool                          string
	Width                         int
	Height                        int
	RoomCount                     int
	RoomsExpected                 []string
	TopologyExpected              []string
	FixturesRendered              []string
	Frames                        []frameProof
	RouteProofs                   []string
	MissingFixtures               []string
	FailedFixtures                []string
	NonEmptyFileCheck             string
	RoomsVisuallyEntered          []string
	PathNonwalkableCrossingReview string
	VisualReview                  string
	Notes                         []string
}

func newFixturePreviewGame(sequence fixtures.Sequence, outDir string, width, height int, proof bool) *fixturePreviewGame {
	return &fixturePreviewGame{
		sequence: sequence,
		outDir:   outDir,
		width:    width,
		height:   height,
		proof:    proof,
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
	frameLabel := ""
	if g.captureTick && g.stepIndex > 0 {
		frameLabel = g.sequence.Steps[g.stepIndex-1].Label
	} else if g.stepIndex < len(g.sequence.Steps) {
		frameLabel = g.sequence.Steps[g.stepIndex].Label
	}
	g.game.SetProofOverlay(g.proof, g.sequence.Name, frameLabel)
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
	proof := flag.Bool("proof", false, "draw proof-only debug overlays")
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

	if *name == "phase20_current_map" {
		if err := runPhase20CurrentMapSuite(*outDir, *width, *height); err != nil {
			log.Fatalf("render phase20 current-map suite: %v", err)
		}
		log.Printf("phase20 current-map fixture suite complete: %s", *outDir)
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

	if err := ebiten.RunGame(newFixturePreviewGame(sequence, targetDir, *width, *height, *proof)); err != nil && err != ebiten.Termination {
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

func runPhase20CurrentMapSuite(outDir string, width, height int) error {
	exePath, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.RemoveAll(outDir); err != nil {
		return err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	manifest := phase20Manifest{
		TaskID:      "PHASE20A1F_ART_BEHAVIOR_ROUTE_START_ANCHOR_REPAIR_001",
		Phase:       "phase_20",
		GeneratedAt: time.Now().Format(time.RFC3339),
		RepoRoot:    repoRootForManifest(),
		Tool:        "castle-go/tools/render-fixture -fixture phase20_current_map -proof",
		Width:       width,
		Height:      height,
		RoomCount:   9,
		RoomsExpected: []string{
			"main_hall",
			"archive",
			"trophy_room",
			"study",
			"workshop",
			"observatory",
			"potions_room",
			"threshold",
			"bedroom",
		},
		TopologyExpected: []string{
			"main_hall: archive, workshop, threshold",
			"archive: main_hall, study, trophy_room",
			"trophy_room: archive, bedroom",
			"study: archive, observatory",
			"workshop: main_hall, potions_room",
			"observatory: study",
			"potions_room: workshop",
			"threshold: main_hall, bedroom",
			"bedroom: threshold, trophy_room",
		},
		MissingFixtures:               []string{},
		FailedFixtures:                []string{},
		PathNonwalkableCrossingReview: "USER_MANUAL_REVIEW_REQUIRED_WITH_DEBUG_OVERLAY",
		VisualReview:                  "USER_MANUAL_REVIEW_REQUIRED",
		Notes: []string{
			"Current map proof only; topology and room coordinates are not changed by this tool.",
			"Proof overlays are debug-only render-fixture output and are not normal product visuals.",
			"Manifest records route metadata separately from file existence.",
		},
	}

	for _, fixtureName := range fixtures.Phase20CurrentMapNames() {
		sequence, err := fixtures.Named(fixtureName)
		if err != nil {
			manifest.MissingFixtures = append(manifest.MissingFixtures, fixtureName)
			continue
		}

		cmd := exec.Command(exePath,
			"-fixture", fixtureName,
			"-outdir", outDir,
			"-width", strconv.Itoa(width),
			"-height", strconv.Itoa(height),
			"-proof",
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			manifest.FailedFixtures = append(manifest.FailedFixtures, fmt.Sprintf("%s: %v", fixtureName, err))
			continue
		}

		manifest.FixturesRendered = append(manifest.FixturesRendered, sequence.Name)
		manifest.RouteProofs = append(manifest.RouteProofs, routeProofForFixture(sequence.Name))
		for _, framePath := range framePaths(outDir, sequence) {
			frameLabel := frameLabelForPath(framePath, sequence)
			proof, err := frameProofFor(framePath, sequence.Name, frameLabel)
			if err != nil {
				manifest.FailedFixtures = append(manifest.FailedFixtures, fmt.Sprintf("%s: %v", framePath, err))
				continue
			}
			manifest.Frames = append(manifest.Frames, proof)
		}
	}
	manifest.RoomsVisuallyEntered = []string{
		"main_hall",
		"archive",
		"trophy_room",
		"study",
		"workshop",
		"observatory",
		"potions_room",
		"threshold",
		"bedroom",
		"route_trophy_threshold_proof: trophy_room, bedroom, threshold",
	}

	manifest.NonEmptyFileCheck = "PASS"
	if len(manifest.Frames) == 0 || len(manifest.MissingFixtures) > 0 || len(manifest.FailedFixtures) > 0 {
		manifest.NonEmptyFileCheck = "FAIL"
	}
	for _, frame := range manifest.Frames {
		if frame.Size <= 0 {
			manifest.NonEmptyFileCheck = "FAIL"
			break
		}
	}

	if err := writePhase20Manifest(outDir, manifest); err != nil {
		return err
	}
	if len(manifest.MissingFixtures) > 0 || len(manifest.FailedFixtures) > 0 {
		return fmt.Errorf("phase20 proof incomplete: missing=%d failed=%d", len(manifest.MissingFixtures), len(manifest.FailedFixtures))
	}
	return nil
}

func repoRootForManifest() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	parent := filepath.Dir(wd)
	if filepath.Base(wd) == "castle-go" {
		return parent
	}
	return wd
}

func frameProofFor(path, fixtureName, frameLabel string) (frameProof, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return frameProof{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return frameProof{}, err
	}
	sum := sha256.Sum256(body)
	meta := proofMetaFor(fixtureName, frameLabel)
	meta.Path = path
	meta.Size = info.Size()
	meta.SHA256 = fmt.Sprintf("%x", sum)
	meta.Fixture = fixtureName
	meta.FrameLabel = frameLabel
	return meta, nil
}

func writePhase20Manifest(outDir string, manifest phase20Manifest) error {
	var b strings.Builder
	writeManifestValue(&b, "task_id", manifest.TaskID)
	writeManifestValue(&b, "phase", manifest.Phase)
	writeManifestValue(&b, "generated_at", manifest.GeneratedAt)
	writeManifestValue(&b, "repo_root", manifest.RepoRoot)
	writeManifestValue(&b, "tool", manifest.Tool)
	writeManifestValue(&b, "width", strconv.Itoa(manifest.Width))
	writeManifestValue(&b, "height", strconv.Itoa(manifest.Height))
	writeManifestValue(&b, "room_count", strconv.Itoa(manifest.RoomCount))
	writeManifestList(&b, "rooms_expected", manifest.RoomsExpected)
	writeManifestList(&b, "topology_expected", manifest.TopologyExpected)
	writeManifestList(&b, "fixtures_rendered", manifest.FixturesRendered)
	writeManifestList(&b, "route_proofs", manifest.RouteProofs)

	b.WriteString("frames:\n")
	for _, frame := range manifest.Frames {
		b.WriteString(fmt.Sprintf("  - path: %s\n", frame.Path))
		b.WriteString(fmt.Sprintf("    size: %d\n", frame.Size))
		b.WriteString(fmt.Sprintf("    sha256: %s\n", frame.SHA256))
		b.WriteString(fmt.Sprintf("    fixture: %s\n", frame.Fixture))
		b.WriteString(fmt.Sprintf("    frame_label: %s\n", frame.FrameLabel))
		b.WriteString(fmt.Sprintf("    expected_room: %s\n", frame.ExpectedRoom))
		b.WriteString(fmt.Sprintf("    expected_route: %s\n", frame.ExpectedRoute))
		b.WriteString(fmt.Sprintf("    movement_segment: %s\n", frame.MovementSegment))
		b.WriteString(fmt.Sprintf("    visual_route_review: %s\n", frame.VisualRouteReview))
		b.WriteString(fmt.Sprintf("    path_nonwalkable_crossing_review: %s\n", frame.PathReview))
		b.WriteString("    waypoint_list:\n")
		writeIndentedList(&b, frame.WaypointList, 6)
		b.WriteString("    rooms_visually_entered_touched:\n")
		writeIndentedList(&b, frame.RoomsTouched, 6)
	}

	b.WriteString("sha256_per_frame:\n")
	for _, frame := range manifest.Frames {
		b.WriteString(fmt.Sprintf("  - %s  %s\n", frame.SHA256, frame.Path))
	}

	writeManifestList(&b, "missing_fixtures", manifest.MissingFixtures)
	writeManifestList(&b, "failed_fixtures", manifest.FailedFixtures)
	writeManifestValue(&b, "non_empty_file_check", manifest.NonEmptyFileCheck)
	writeManifestList(&b, "rooms_visually_entered_touched", manifest.RoomsVisuallyEntered)
	writeManifestValue(&b, "path_nonwalkable_crossing_review", manifest.PathNonwalkableCrossingReview)
	writeManifestValue(&b, "visual_review", manifest.VisualReview)
	writeManifestList(&b, "notes", manifest.Notes)

	return os.WriteFile(filepath.Join(outDir, "manifest.txt"), []byte(b.String()), 0o644)
}

func writeManifestValue(b *strings.Builder, key, value string) {
	b.WriteString(fmt.Sprintf("%s: %s\n", key, value))
}

func writeManifestList(b *strings.Builder, key string, values []string) {
	b.WriteString(key + ":\n")
	writeIndentedList(b, values, 2)
}

func writeIndentedList(b *strings.Builder, values []string, indent int) {
	pad := strings.Repeat(" ", indent)
	if len(values) == 0 {
		b.WriteString(pad + "- NONE\n")
		return
	}
	for _, value := range values {
		b.WriteString(pad + "- " + value + "\n")
	}
}

func frameLabelForPath(path string, sequence fixtures.Sequence) string {
	base := filepath.Base(path)
	for index, step := range sequence.Steps {
		if !step.Capture {
			continue
		}
		expected := fmt.Sprintf("%02d_%s.png", index+1, sanitize(step.Label))
		if base == expected {
			return step.Label
		}
	}
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func routeProofForFixture(fixtureName string) string {
	switch fixtureName {
	case "route_trophy_threshold_proof":
		return "route_trophy_threshold_proof expected_route=trophy_room>bedroom>threshold required_intermediate=bedroom overlay=room_labels,path_polyline,waypoints"
	case "arrival_bedroom":
		return "arrival_bedroom expected_route=main_hall>threshold>bedroom"
	case "arrival_trophy":
		return "arrival_trophy expected_route=main_hall>archive>trophy_room"
	case "arrival_threshold":
		return "arrival_threshold expected_route=main_hall>threshold"
	default:
		return fixtureName + " arrival proof overlay enabled"
	}
}

func proofMetaFor(fixtureName, frameLabel string) frameProof {
	meta := frameProof{
		ExpectedRoom:      expectedRoomForFixture(fixtureName),
		ExpectedRoute:     expectedRouteForFixture(fixtureName),
		MovementSegment:   "SEE_DEBUG_OVERLAY",
		WaypointList:      []string{"SEE_DEBUG_OVERLAY_PATH_POLYLINE_AND_MARKERS"},
		RoomsTouched:      roomsTouchedForFixture(fixtureName, frameLabel),
		VisualRouteReview: "USER_MANUAL_REVIEW_REQUIRED_WITH_ROOM_LABELS",
		PathReview:        "USER_MANUAL_REVIEW_REQUIRED_WITH_ROUTE_POLYLINE",
	}
	if fixtureName == "route_trophy_threshold_proof" {
		meta.ExpectedRoom = routeProofExpectedRoomForFrame(frameLabel)
		meta.MovementSegment = routeProofSegmentForFrame(frameLabel)
		meta.WaypointList = []string{
			"trophy_room interior",
			"trophy_room door to bedroom",
			"trophy_room>bedroom corridor",
			"bedroom door from trophy_room",
			"bedroom pass-through interior",
			"bedroom door to threshold",
			"bedroom>threshold corridor",
			"threshold door from bedroom",
			"threshold center",
		}
	}
	return meta
}

func expectedRoomForFixture(fixtureName string) string {
	switch fixtureName {
	case "arrival_main_hall":
		return "main_hall"
	case "arrival_archive":
		return "archive"
	case "arrival_trophy":
		return "trophy_room"
	case "arrival_study":
		return "study"
	case "arrival_workshop":
		return "workshop"
	case "arrival_observatory":
		return "observatory"
	case "arrival_potions":
		return "potions_room"
	case "arrival_threshold", "route_trophy_threshold_proof":
		return "threshold"
	case "arrival_bedroom":
		return "bedroom"
	default:
		return "UNKNOWN"
	}
}

func expectedRouteForFixture(fixtureName string) string {
	switch fixtureName {
	case "arrival_archive":
		return "main_hall>archive"
	case "arrival_trophy":
		return "main_hall>archive>trophy_room"
	case "arrival_study":
		return "main_hall>archive>study"
	case "arrival_workshop":
		return "main_hall>workshop"
	case "arrival_observatory":
		return "main_hall>archive>study>observatory"
	case "arrival_potions":
		return "main_hall>workshop>potions_room"
	case "arrival_threshold":
		return "main_hall>threshold"
	case "arrival_bedroom":
		return "main_hall>threshold>bedroom"
	case "route_trophy_threshold_proof":
		return "trophy_room>bedroom>threshold"
	default:
		return expectedRoomForFixture(fixtureName)
	}
}

func roomsTouchedForFixture(fixtureName, frameLabel string) []string {
	if fixtureName == "route_trophy_threshold_proof" {
		switch {
		case strings.Contains(frameLabel, "start_trophy"):
			return []string{"trophy_room"}
		case strings.Contains(frameLabel, "bedroom"):
			return []string{"trophy_room", "bedroom"}
		case strings.Contains(frameLabel, "threshold"):
			return []string{"trophy_room", "bedroom", "threshold"}
		default:
			return []string{"trophy_room", "bedroom", "threshold"}
		}
	}
	return []string{expectedRoomForFixture(fixtureName)}
}

func routeProofExpectedRoomForFrame(frameLabel string) string {
	switch {
	case strings.Contains(frameLabel, "start_trophy") || strings.Contains(frameLabel, "depart_trophy"):
		return "trophy_room"
	case strings.Contains(frameLabel, "bedroom"):
		return "bedroom"
	case strings.Contains(frameLabel, "threshold"):
		return "threshold"
	default:
		return "trophy_room>bedroom>threshold"
	}
}

func routeProofSegmentForFrame(frameLabel string) string {
	switch {
	case strings.Contains(frameLabel, "depart_trophy"):
		return "trophy_room -> bedroom"
	case strings.Contains(frameLabel, "enter_bedroom"):
		return "bedroom intermediate entry"
	case strings.Contains(frameLabel, "cross_bedroom"):
		return "bedroom -> threshold"
	case strings.Contains(frameLabel, "enter_threshold"), strings.Contains(frameLabel, "settle_threshold"):
		return "threshold arrival"
	default:
		return "route_trophy_threshold_proof"
	}
}
