package assets

import (
	"bytes"
	"embed"
	"image"
	"image/color"
	"image/draw"
	_ "image/png"
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
	"sprites/milo/idle":     {R: 255, G: 220, B: 100, A: 255}, // warm yellow — resting duck
	"sprites/milo/walking":  {R: 100, G: 200, B: 255, A: 255}, // blue — movement
	"sprites/milo/talking":  {R: 255, G: 150, B: 100, A: 255}, // orange — speech
	"sprites/milo/thinking": {R: 180, G: 100, B: 255, A: 255}, // purple — cognition
	"sprites/milo/sleeping": {R: 100, G: 130, B: 180, A: 255}, // slate — rest
	"sprites/milo/working":  {R: 80, G: 200, B: 120, A: 255},  // green — active work
	"sprites/milo/reading":  {R: 200, G: 170, B: 100, A: 255}, // parchment
	"sprites/milo/stirring": {R: 180, G: 60, B: 180, A: 255},  // magenta — cauldron
	"sprites/milo/gazing":   {R: 60, G: 180, B: 200, A: 255},  // cyan — orb gaze
	"rooms/main_hall":       {R: 60, G: 60, B: 80, A: 255},
	"rooms/war_room":        {R: 80, G: 50, B: 50, A: 255},
	"rooms/library":         {R: 60, G: 70, B: 50, A: 255},
	"rooms/training_room":   {R: 50, G: 60, B: 80, A: 255},
	"rooms/spellbook":       {R: 70, G: 50, B: 80, A: 255},
	"rooms/cauldron":        {R: 50, G: 70, B: 50, A: 255},
	"rooms/crystal_orb":     {R: 40, G: 60, B: 90, A: 255},
	"rooms/baby_dragon":     {R: 80, G: 60, B: 40, A: 255},
	"rooms/trophy":          {R: 80, G: 70, B: 30, A: 255},
	"rooms/archive":         {R: 50, G: 50, B: 60, A: 255},
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
	return loadOrPlaceholder(p, MiloFrameW*MiloFrames, MiloFrameH, "sprites/milo/"+state+"_"+facing)
}

// LoadRoomBackground loads "rooms/{roomID}.png".
// Returns a placeholder if the file is missing.
func LoadRoomBackground(roomID string) *ebiten.Image {
	p := path.Join("rooms", roomID+".png")
	return loadOrPlaceholder(p, RoomBGW, RoomBGH, "rooms/"+roomID)
}

// LoadRoomBackgroundProcedural returns the deterministic procedural room background.
// Used as a bounded runtime fallback when a room texture uploads/render as near-black on device.
func LoadRoomBackgroundProcedural(roomID string, w, h int) *ebiten.Image {
	return makeRoomPlaceholder(roomID, w, h)
}

// LoadPropSprite loads "sprites/props/{propKey}.png".
func LoadPropSprite(propKey string) *ebiten.Image {
	p := path.Join("sprites", "props", propKey+".png")
	return loadOrPlaceholder(p, 64, 128, "sprites/props/"+propKey)
}

func loadOrPlaceholder(filePath string, w, h int, colorKey string) *ebiten.Image {
	println("ASSET LOAD TRY:", filePath)

	data, err := fs.ReadFile(filePath)
	if err != nil {
		println("ASSET LOAD FAIL:", filePath, err.Error())
	}
	if err == nil {
		img, _, err2 := image.Decode(bytes.NewReader(data))
		if err2 == nil {
			// On some Android GPU paths, room background PNGs can decode with valid bounds
			// but upload/render effectively black. For rooms, force an explicit RGBA upload.
			if len(filePath) >= len("rooms/") && filePath[:len("rooms/")] == "rooms/" {
				b := img.Bounds()
				rgbaImg := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
				draw.Draw(rgbaImg, rgbaImg.Bounds(), img, b.Min, draw.Src)
				eimg := ebiten.NewImage(b.Dx(), b.Dy())
				eimg.ReplacePixels(rgbaImg.Pix)
				return eimg
			}
			return ebiten.NewImageFromImage(img)
		}
		log.Printf("assets: decode %s: %v", filePath, err2)
	}
	if len(colorKey) >= len("rooms/") && colorKey[:len("rooms/")] == "rooms/" {
		return makeRoomPlaceholder(colorKey[len("rooms/"):], w, h)
	}
	if len(colorKey) >= len("sprites/milo/") && colorKey[:len("sprites/milo/")] == "sprites/milo/" {
		return makeMiloPlaceholderSheet(colorKey[len("sprites/milo/"):], w, h)
	}
	if len(colorKey) >= len("sprites/props/") && colorKey[:len("sprites/props/")] == "sprites/props/" {
		return makePropPlaceholder(colorKey[len("sprites/props/"):], w, h)
	}
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

func makeRoomPlaceholder(roomID string, w, h int) *ebiten.Image {
	palette := roomPalette(roomID)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{palette.bgTop}, image.Point{}, draw.Src)

	for y := 0; y < h; y++ {
		t := float64(y) / float64(h)
		line := blend(palette.bgTop, palette.bgBottom, t)
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, line)
		}
	}

	for y := h / 2; y < h; y += 22 {
		for x := 0; x < w; x += 56 {
			offset := ((y / 22) % 2) * 28
			drawDiamond(img, x+offset, y, 22, 11, palette.floorTile)
		}
	}

	fillRect(img, 0, 0, w, h/5, palette.vignette)
	fillRect(img, 0, h-h/6, w, h/6, palette.floorShadow)
	drawWallWindows(img, roomID, palette, w, h)
	drawRoomRunner(img, roomID, palette, w, h)
	drawPillar(img, 36, 78, 112, 156, palette.arch)
	drawPillar(img, w-148, 78, 112, 156, palette.arch)
	drawDais(img, w/2-160, h/2-12, 320, 112, darken(palette.feature, 0.9), lighten(palette.feature, 0.1))
	drawChamberFeature(img, roomID, palette, w, h)
	drawBorder(img, palette.border)

	return ebiten.NewImageFromImage(img)
}

func makePropPlaceholder(propKey string, w, h int) *ebiten.Image {
	base := resolveColor("sprites/props/" + propKey)
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	transparent := color.RGBA{0, 0, 0, 0}
	draw.Draw(img, img.Bounds(), &image.Uniform{transparent}, image.Point{}, draw.Src)

	switch {
	case hasAny(propKey, "throne", "lectern", "desk", "table", "plinth", "stand"):
		fillRect(img, w/2-8, h-18, 16, 18, darken(base, 0.55))
		fillRect(img, w/2-26, h-52, 52, 28, base)
		fillRect(img, w/2-32, h-60, 64, 10, lighten(base, 0.18))
		fillRect(img, w/2-18, h-42, 36, 10, lighten(base, 0.28))
	case hasAny(propKey, "banner", "flag", "victory"):
		fillRect(img, w/2-4, h-94, 8, 94, darken(base, 0.45))
		fillRect(img, w/2, h-92, 26, 62, lighten(base, 0.12))
		drawStar(img, w/2+14, h-62, 8, color.RGBA{R: 241, G: 221, B: 166, A: 220})
	case hasAny(propKey, "shelf", "rack", "case", "wall", "bookshelf"):
		fillRect(img, w/2-24, h-82, 48, 64, darken(base, 0.2))
		fillRect(img, w/2-28, h-90, 56, 8, lighten(base, 0.18))
		fillRect(img, w/2-28, h-56, 56, 6, lighten(base, 0.08))
		fillRect(img, w/2-18, h-74, 10, 8, lighten(base, 0.22))
		fillRect(img, w/2-4, h-74, 10, 8, lighten(base, 0.12))
		fillRect(img, w/2+10, h-74, 10, 8, lighten(base, 0.26))
	case hasAny(propKey, "orb", "crystal", "memory", "gem"):
		fillRect(img, w/2-6, h-18, 12, 18, darken(base, 0.55))
		fillRect(img, w/2-18, h-46, 36, 18, darken(base, 0.15))
		drawDiamond(img, w/2, h-72, 18, 18, lighten(base, 0.28))
		drawTintGlow(img, w/2-22, h-94, 44, 40, color.RGBA{R: 196, G: 230, B: 255, A: 96})
	case hasAny(propKey, "cauldron", "fire", "candle", "lamp"):
		fillRect(img, w/2-20, h-46, 40, 22, darken(base, 0.2))
		fillRect(img, w/2-16, h-70, 32, 22, base)
		fillRect(img, w/2-10, h-92, 20, 14, color.RGBA{R: 255, G: 192, B: 105, A: 220})
	default:
		fillRect(img, w/2-6, h-18, 12, 18, darken(base, 0.55))
		fillRect(img, w/2-22, h-48, 44, 32, base)
		fillRect(img, w/2-28, h-56, 56, 8, lighten(base, 0.18))
	}

	drawBorder(img, darken(base, 0.5))
	return ebiten.NewImageFromImage(img)
}

func drawPillar(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	fillRect(img, x, y, w, h, c)
	fillRect(img, x+10, y+12, w-20, h-12, lighten(c, 0.08))
	fillRect(img, x, y, w, 14, darken(c, 0.35))
}

func drawDais(img *image.RGBA, x, y, w, h int, base, glow color.RGBA) {
	fillRect(img, x, y, w, h, base)
	fillRect(img, x+36, y+18, w-72, h-34, glow)
	fillRect(img, x+88, y+36, w-176, h-58, lighten(glow, 0.18))
}

func drawChamberFeature(img *image.RGBA, roomID string, palette roomColors, w, h int) {
	switch roomID {
	case "main_hall":
		fillRect(img, w/2-90, 64, 180, 190, palette.glow)
		fillRect(img, w/2-84, 94, 168, 126, lighten(palette.feature, 0.18))
		fillRect(img, w/2-40, 220, 80, 90, darken(palette.feature, 0.25))
		drawStar(img, w/2, 126, 24, color.RGBA{R: 246, G: 232, B: 186, A: 220})
	case "archive":
		fillRect(img, w/2-118, 78, 236, 140, palette.glow)
		fillRect(img, w/2-140, h/2-8, 280, 122, lighten(palette.feature, 0.1))
		fillRect(img, 54, h/2+18, 112, 182, darken(palette.arch, 0.05))
		fillRect(img, w-166, h/2+18, 112, 182, darken(palette.arch, 0.05))
		fillRect(img, w/2-60, h/2+26, 120, 74, lighten(palette.glow, 0.28))
		drawDiamond(img, w/2, 158, 34, 34, color.RGBA{R: 196, G: 225, B: 255, A: 168})
	case "crystal_orb":
		fillRect(img, w/2-98, 72, 196, 172, palette.glow)
		drawDiamond(img, w/2, 170, 78, 78, lighten(palette.feature, 0.22))
		fillRect(img, w/2-56, 220, 112, 96, darken(palette.feature, 0.18))
		drawTintGlow(img, w/2-82, 100, 164, 148, color.RGBA{R: 143, G: 221, B: 255, A: 80})
	case "library":
		fillRect(img, 58, 92, 130, 210, darken(palette.arch, 0.08))
		fillRect(img, w-188, 92, 130, 210, darken(palette.arch, 0.08))
		fillRect(img, w/2-112, h/2-6, 224, 112, palette.feature)
		fillRect(img, w/2-62, h/2+16, 124, 72, lighten(palette.feature, 0.12))
	case "war_room":
		fillRect(img, w/2-150, h/2-8, 300, 116, palette.feature)
		fillRect(img, w/2-54, 84, 108, 128, palette.glow)
	default:
		fillRect(img, w/2-90, 68, 180, 180, palette.glow)
		fillRect(img, w/2-70, 86, 140, 182, palette.feature)
	}
}

func makeMiloPlaceholderSheet(key string, w, h int) *ebiten.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	transparent := color.RGBA{0, 0, 0, 0}
	draw.Draw(img, img.Bounds(), &image.Uniform{transparent}, image.Point{}, draw.Src)

	state, facing := parseMiloKey(key)
	duck := color.RGBA{R: 198, G: 224, B: 166, A: 255}
	duckShade := color.RGBA{R: 170, G: 201, B: 145, A: 255}
	robe := color.RGBA{R: 34, G: 45, B: 96, A: 255}
	robeShade := color.RGBA{R: 22, G: 30, B: 70, A: 255}
	star := color.RGBA{R: 246, G: 231, B: 185, A: 255}
	beak := color.RGBA{R: 239, G: 137, B: 60, A: 255}
	beakShade := color.RGBA{R: 201, G: 104, B: 45, A: 255}
	eye := color.RGBA{R: 24, G: 22, B: 30, A: 255}
	accent := resolveColor("sprites/milo/" + state)

	for frame := 0; frame < MiloFrames; frame++ {
		frameX := frame * MiloFrameW
		bob := (frame % 4) - 1
		sway := 0
		if frame%2 == 1 {
			sway = 1
		}
		drawMiloFrame(img, frameX, bob, sway, state, facing, duck, duckShade, robe, robeShade, star, beak, beakShade, eye, accent)
	}

	applySpriteOutline(img, color.RGBA{R: 16, G: 18, B: 26, A: 190})

	return ebiten.NewImageFromImage(img)
}

func drawMiloFrame(img *image.RGBA, frameX, bob, sway int, state, facing string, duck, duckShade, robe, robeShade, star, beak, beakShade, eye, accent color.RGBA) {
	hatTilt := 0
	if facing == "e" {
		hatTilt = 3
	}
	if facing == "w" {
		hatTilt = -3
	}

	legShiftL, legShiftR := 0, 0
	robeLean := 0
	if state == "walking" {
		if sway%2 == 0 {
			legShiftL = -2
			legShiftR = 2
			robeLean = 1
		} else {
			legShiftL = 2
			legShiftR = -2
			robeLean = -1
		}
	}

	drawWizardHat(img, frameX+13+hatTilt, 0+bob, facing, robe, robeShade, star)
	drawDuckHead(img, frameX+15, 18+bob, facing, duck, duckShade, beak, beakShade, eye, state)
	drawWizardRobe(img, frameX+17+sway+robeLean, 41+bob, facing, robe, robeShade, star, accent, state)
	drawDuckFeet(img, frameX+25+legShiftL, frameX+37+legShiftR, 72+bob, beak, beakShade)
}

func drawWizardHat(img *image.RGBA, x, y int, facing string, robe, robeShade, star color.RGBA) {
	fillRect(img, x+6, y+20, 30, 6, robeShade)
	fillRect(img, x+8, y+16, 26, 6, robe)
	fillRect(img, x+10, y+8, 20, 10, robe)
	fillRect(img, x+14, y+2, 16, 10, robe)
	fillRect(img, x+20, y-2, 12, 8, robe)
	if facing == "e" {
		fillRect(img, x+28, y-4, 8, 4, robe)
	} else if facing == "w" {
		fillRect(img, x+8, y-4, 8, 4, robe)
	} else {
		fillRect(img, x+24, y-4, 10, 4, robe)
	}
	fillRect(img, x+12, y+10, 12, 4, lighten(robe, 0.08))
	drawStar(img, x+27, y+7, 5, star)
	drawStar(img, x+13, y+19, 4, star)
}

func drawDuckHead(img *image.RGBA, x, y int, facing string, duck, duckShade, beak, beakShade, eye color.RGBA, state string) {
	fillRect(img, x+2, y+4, 24, 18, duck)
	fillRect(img, x+4, y+2, 20, 20, lighten(duck, 0.05))
	fillRect(img, x+1, y+15, 6, 7, duckShade)
	fillRect(img, x+21, y+15, 7, 7, duckShade)

	switch facing {
	case "n":
		fillRect(img, x+8, y+6, 12, 11, duck)
		fillRect(img, x+11, y+16, 8, 7, beak)
		fillRect(img, x+9, y+10, 4, 2, eye)
		fillRect(img, x+17, y+10, 4, 2, eye)
	case "e":
		fillRect(img, x+21, y+10, 14, 7, beak)
		fillRect(img, x+24, y+14, 12, 4, beakShade)
		fillRect(img, x+15, y+9, 6, 2, eye)
		fillRect(img, x+16, y+8, 5, 1, lighten(duckShade, 0.25))
		if state == "thinking" || state == "gazing" {
			fillRect(img, x+3, y+7, 4, 9, lighten(duck, 0.16))
		}
	case "w":
		fillRect(img, x-9, y+10, 14, 7, beak)
		fillRect(img, x-11, y+14, 12, 4, beakShade)
		fillRect(img, x+8, y+9, 6, 2, eye)
		fillRect(img, x+8, y+8, 5, 1, lighten(duckShade, 0.25))
		if state == "thinking" || state == "gazing" {
			fillRect(img, x+22, y+7, 4, 9, lighten(duck, 0.16))
		}
	default:
		fillRect(img, x+18, y+11, 17, 7, beak)
		fillRect(img, x+19, y+15, 15, 3, beakShade)
		fillRect(img, x+9, y+9, 5, 2, eye)
		fillRect(img, x+18, y+9, 5, 2, eye)
		fillRect(img, x+8, y+8, 6, 1, lighten(duckShade, 0.24))
		fillRect(img, x+18, y+8, 6, 1, lighten(duckShade, 0.24))
		if state == "thinking" || state == "gazing" {
			fillRect(img, x+2, y+7, 4, 9, lighten(duck, 0.16))
		}
	}
}

func drawWizardRobe(img *image.RGBA, x, y int, facing string, robe, robeShade, star, accent color.RGBA, state string) {
	fillRect(img, x+7, y, 22, 22, robe)
	fillRect(img, x+5, y+20, 28, 12, robe)
	fillRect(img, x+9, y+2, 18, 18, lighten(robe, 0.03))
	fillRect(img, x+9, y+23, 20, 7, robeShade)
	fillRect(img, x+16, y+7, 5, 12, lighten(robe, 0.08))
	drawStar(img, x+11, y+9, 4, star)
	drawStar(img, x+24, y+19, 4, star)
	if state == "working" || state == "reading" || state == "stirring" || state == "gazing" {
		fillRect(img, x+11, y+11, 13, 5, lighten(accent, 0.12))
	}
	if facing == "n" {
		fillRect(img, x+13, y+4, 10, 18, robeShade)
	}
	if facing == "e" || facing == "w" {
		fillRect(img, x+4, y+12, 5, 10, robeShade)
		fillRect(img, x+29, y+12, 5, 10, robeShade)
	}
}

func drawDuckFeet(img *image.RGBA, leftX, rightX, y int, beak, beakShade color.RGBA) {
	fillRect(img, leftX, y, 4, 10, beak)
	fillRect(img, rightX, y, 4, 10, beak)
	fillRect(img, leftX-3, y+9, 10, 3, beakShade)
	fillRect(img, rightX-3, y+9, 10, 3, beakShade)
}

func parseMiloKey(key string) (state, facing string) {
	parts := bytes.Split([]byte(key), []byte("_"))
	if len(parts) == 0 {
		return "idle", "s"
	}
	facing = "s"
	if len(parts) > 1 {
		last := string(parts[len(parts)-1])
		if last == "n" || last == "s" || last == "e" || last == "w" {
			facing = last
			return string(bytes.Join(parts[:len(parts)-1], []byte("_"))), facing
		}
	}
	return key, facing
}

func drawWallWindows(img *image.RGBA, roomID string, palette roomColors, w, h int) {
	if roomID == "archive" || roomID == "library" {
		return
	}
	windowColor := lighten(palette.glow, 0.22)
	drawTintGlow(img, 84, 124, 42, 84, darken(windowColor, 0.15))
	drawTintGlow(img, w-126, 124, 42, 84, darken(windowColor, 0.15))
	fillRect(img, 90, 130, 30, 72, windowColor)
	fillRect(img, w-120, 130, 30, 72, windowColor)
}

func drawRoomRunner(img *image.RGBA, roomID string, palette roomColors, w, h int) {
	switch roomID {
	case "main_hall":
		fillRect(img, w/2-54, h/2+102, 108, h/2-118, darken(palette.feature, 0.34))
		fillRect(img, w/2-34, h/2+102, 68, h/2-118, lighten(palette.feature, 0.04))
	case "library":
		fillRect(img, w/2-70, h/2+94, 140, h/2-118, darken(palette.feature, 0.3))
		fillRect(img, w/2-48, h/2+94, 96, h/2-118, lighten(palette.feature, 0.02))
	case "archive":
		fillRect(img, w/2-62, h/2+102, 124, h/2-126, darken(palette.feature, 0.24))
		fillRect(img, w/2-40, h/2+102, 80, h/2-126, lighten(palette.feature, 0.08))
	case "crystal_orb":
		fillRect(img, w/2-48, h/2+102, 96, h/2-126, darken(palette.feature, 0.24))
	}
}

func drawTintGlow(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	fillRect(img, x, y, w, h, c)
	fillRect(img, x+6, y+6, max(1, w-12), max(1, h-12), lighten(c, 0.08))
}

func drawStar(img *image.RGBA, cx, cy, size int, c color.RGBA) {
	fillRect(img, cx-size/2, cy-1, size, 3, c)
	fillRect(img, cx-1, cy-size/2, 3, size, c)
	fillRect(img, cx-2, cy-2, 5, 5, c)
}

func applySpriteOutline(img *image.RGBA, stroke color.RGBA) {
	src := image.NewRGBA(img.Bounds())
	draw.Draw(src, src.Bounds(), img, image.Point{}, draw.Src)

	for y := 1; y < img.Bounds().Dy()-1; y++ {
		for x := 1; x < img.Bounds().Dx()-1; x++ {
			if alphaAt(src, x, y) != 0 {
				continue
			}
			if hasOpaqueNeighbor(src, x, y) {
				img.SetRGBA(x, y, stroke)
			}
		}
	}
}

func hasOpaqueNeighbor(img *image.RGBA, x, y int) bool {
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			if alphaAt(img, x+dx, y+dy) != 0 {
				return true
			}
		}
	}
	return false
}

func alphaAt(img *image.RGBA, x, y int) uint8 {
	if !image.Pt(x, y).In(img.Bounds()) {
		return 0
	}
	return img.RGBAAt(x, y).A
}

func resolveColor(key string) color.RGBA {
	for prefix, c := range placeholderColors {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			return c
		}
	}
	return color.RGBA{R: 80, G: 80, B: 80, A: 255} // fallback gray
}

type roomColors struct {
	bgTop       color.RGBA
	bgBottom    color.RGBA
	floorTile   color.RGBA
	vignette    color.RGBA
	floorShadow color.RGBA
	glow        color.RGBA
	arch        color.RGBA
	feature     color.RGBA
	border      color.RGBA
}

func roomPalette(roomID string) roomColors {
	switch roomID {
	case "main_hall":
		// Main Hall is the default landing room and must stay visibly readable even when
		// we are forced onto procedural fallback on-device. Keep this palette brighter
		// and higher-contrast than the generic default.
		return roomColors{
			rgba(82, 80, 118, 255),  // bgTop
			rgba(44, 42, 70, 255),   // bgBottom
			rgba(126, 118, 168, 255), // floorTile
			rgba(10, 10, 18, 86),    // vignette
			rgba(12, 11, 18, 150),   // floorShadow
			rgba(196, 184, 255, 108), // glow
			rgba(112, 108, 156, 255), // arch
			rgba(176, 160, 224, 255), // feature
			rgba(30, 28, 48, 255),   // border
		}
	case "war_room":
		return roomColors{rgba(52, 36, 43, 255), rgba(32, 25, 30, 255), rgba(93, 64, 61, 255), rgba(17, 12, 16, 120), rgba(18, 12, 14, 180), rgba(118, 79, 82, 90), rgba(83, 54, 59, 255), rgba(126, 88, 76, 255), rgba(27, 18, 21, 255)}
	case "library":
		return roomColors{rgba(47, 56, 43, 255), rgba(27, 33, 29, 255), rgba(79, 90, 67, 255), rgba(14, 16, 12, 110), rgba(17, 18, 13, 180), rgba(145, 118, 84, 80), rgba(71, 78, 54, 255), rgba(114, 89, 58, 255), rgba(21, 25, 19, 255)}
	case "training_room":
		return roomColors{rgba(40, 49, 68, 255), rgba(24, 29, 43, 255), rgba(68, 82, 103, 255), rgba(12, 15, 24, 110), rgba(14, 16, 22, 180), rgba(102, 146, 190, 70), rgba(50, 59, 79, 255), rgba(91, 109, 141, 255), rgba(18, 22, 31, 255)}
	case "spellbook":
		return roomColors{rgba(56, 39, 72, 255), rgba(31, 21, 42, 255), rgba(88, 64, 112, 255), rgba(14, 10, 20, 120), rgba(16, 11, 22, 180), rgba(161, 113, 217, 80), rgba(78, 56, 106, 255), rgba(121, 85, 152, 255), rgba(22, 16, 30, 255)}
	case "cauldron":
		return roomColors{rgba(32, 58, 46, 255), rgba(16, 31, 24, 255), rgba(57, 95, 72, 255), rgba(9, 16, 12, 120), rgba(10, 17, 13, 180), rgba(84, 188, 121, 74), rgba(46, 76, 62, 255), rgba(86, 128, 89, 255), rgba(15, 24, 19, 255)}
	case "crystal_orb":
		return roomColors{rgba(24, 43, 79, 255), rgba(12, 19, 39, 255), rgba(48, 76, 116, 255), rgba(7, 10, 19, 120), rgba(9, 11, 20, 180), rgba(98, 168, 255, 84), rgba(35, 56, 91, 255), rgba(78, 109, 161, 255), rgba(13, 21, 39, 255)}
	case "baby_dragon":
		return roomColors{rgba(74, 48, 27, 255), rgba(43, 27, 16, 255), rgba(117, 84, 46, 255), rgba(20, 11, 7, 110), rgba(22, 14, 7, 180), rgba(220, 153, 96, 80), rgba(99, 65, 37, 255), rgba(166, 108, 57, 255), rgba(31, 19, 12, 255)}
	case "trophy":
		return roomColors{rgba(79, 65, 23, 255), rgba(46, 34, 14, 255), rgba(125, 104, 44, 255), rgba(20, 15, 8, 110), rgba(22, 16, 8, 180), rgba(238, 213, 125, 82), rgba(103, 84, 31, 255), rgba(186, 158, 69, 255), rgba(31, 24, 11, 255)}
	case "archive":
		return roomColors{rgba(42, 47, 64, 255), rgba(24, 27, 39, 255), rgba(69, 78, 101, 255), rgba(12, 13, 22, 118), rgba(13, 14, 24, 180), rgba(132, 182, 255, 68), rgba(58, 63, 82, 255), rgba(110, 121, 152, 255), rgba(18, 21, 30, 255)}
	default:
		return roomColors{rgba(44, 48, 66, 255), rgba(24, 27, 40, 255), rgba(73, 80, 104, 255), rgba(12, 13, 23, 112), rgba(14, 15, 26, 180), rgba(152, 126, 232, 74), rgba(59, 64, 86, 255), rgba(109, 98, 156, 255), rgba(19, 22, 33, 255)}
	}
}

func fillRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	r := image.Rect(x, y, x+w, y+h).Intersect(img.Bounds())
	if r.Empty() {
		return
	}
	draw.Draw(img, r, &image.Uniform{c}, image.Point{}, draw.Over)
}

func drawDiamond(img *image.RGBA, cx, cy, rw, rh int, c color.RGBA) {
	for dy := -rh; dy <= rh; dy++ {
		width := rw - (abs(dy) * rw / max(1, rh))
		for dx := -width; dx <= width; dx++ {
			x := cx + dx
			y := cy + dy
			if image.Pt(x, y).In(img.Bounds()) {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func drawBorder(img *image.RGBA, c color.RGBA) {
	b := img.Bounds()
	for x := b.Min.X; x < b.Max.X; x++ {
		img.SetRGBA(x, b.Min.Y, c)
		img.SetRGBA(x, b.Max.Y-1, c)
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		img.SetRGBA(b.Min.X, y, c)
		img.SetRGBA(b.Max.X-1, y, c)
	}
}

func darken(c color.RGBA, factor float64) color.RGBA {
	return color.RGBA{
		R: uint8(float64(c.R) * factor),
		G: uint8(float64(c.G) * factor),
		B: uint8(float64(c.B) * factor),
		A: c.A,
	}
}

func lighten(c color.RGBA, amount float64) color.RGBA {
	return color.RGBA{
		R: uint8(min(255, int(float64(c.R)+(255-float64(c.R))*amount))),
		G: uint8(min(255, int(float64(c.G)+(255-float64(c.G))*amount))),
		B: uint8(min(255, int(float64(c.B)+(255-float64(c.B))*amount))),
		A: c.A,
	}
}

func blend(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		R: uint8(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		G: uint8(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		B: uint8(float64(a.B) + (float64(b.B)-float64(a.B))*t),
		A: uint8(float64(a.A) + (float64(b.A)-float64(a.A))*t),
	}
}

func hasAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if len(value) >= len(needle) && bytes.Contains([]byte(value), []byte(needle)) {
			return true
		}
	}
	return false
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func rgba(r, g, b, a uint8) color.RGBA {
	return color.RGBA{R: r, G: g, B: b, A: a}
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
