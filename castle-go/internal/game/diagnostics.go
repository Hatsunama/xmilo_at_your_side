package game

import (
	"image"
	"image/color"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
)

// Diagnostic draw sequencer.
//
// Purpose: eliminate content-vs-presentation uncertainty by proving:
// 1) whether the active room background texture is non-black at draw time
// 2) whether draw primitives (tint rects) are visible at draw time
//
// This file is intentionally small and time-bounded so it can be removed
// after Main Hub approves a final cure patch.

const diagEnabled = false

type diagPhase int

const (
	diagPhaseBackgroundBlit diagPhase = iota + 1
	diagPhasePrimitivesOnly
)

func diagPhaseForFrame(frame int) (diagPhase, bool) {
	// Keep the diagnostic surface time-bounded so normal rendering resumes.
	switch {
	case frame >= 1 && frame <= 30:
		return diagPhaseBackgroundBlit, true
	case frame >= 31 && frame <= 60:
		return diagPhasePrimitivesOnly, true
	default:
		return 0, false
	}
}

func diagDrawPhaseName(phase diagPhase) string {
	switch phase {
	case diagPhaseBackgroundBlit:
		return "background_blit"
	case diagPhasePrimitivesOnly:
		return "primitives_only"
	default:
		return "unknown"
	}
}

func diagDraw(screen *ebiten.Image, frame int, roomBackground *ebiten.Image, pixels *[]byte) bool {
	if !diagEnabled || screen == nil {
		return false
	}
	phase, ok := diagPhaseForFrame(frame)
	if !ok {
		return false
	}

	sw, sh := screen.Bounds().Dx(), screen.Bounds().Dy()
	ensureDrawPrimitives()

	switch phase {
	case diagPhaseBackgroundBlit:
		// Draw order: clear black, then blit the active room background (if present),
		// then draw two bright primitive squares as canaries.
		screen.Fill(color.RGBA{R: 0, G: 0, B: 0, A: 255})
		if roomBackground != nil {
			bgOp := &ebiten.DrawImageOptions{}
			bgW, bgH := roomBackground.Bounds().Dx(), roomBackground.Bounds().Dy()
			if bgW > 0 && bgH > 0 {
				bgOp.GeoM.Scale(float64(sw)/float64(bgW), float64(sh)/float64(bgH))
				screen.DrawImage(roomBackground, bgOp)
			}
		}
		drawTintRect(screen, 0, 0, 48, 48, color.RGBA{R: 255, G: 255, B: 255, A: 255}) // white canary

		drawTintRect(screen, 0, sh-12, sw, 12, color.RGBA{R: 0, G: 0, B: 0, A: 255}) // black bar
	case diagPhasePrimitivesOnly:
		// Prove primitives survive even when textures do not.
		screen.Fill(color.RGBA{R: 12, G: 12, B: 12, A: 255})
		halfW := sw / 2
		halfH := sh / 2
		drawTintRect(screen, 0, 0, halfW, halfH, color.RGBA{R: 255, G: 64, B: 64, A: 255})                // red
		drawTintRect(screen, halfW, 0, sw-halfW, halfH, color.RGBA{R: 64, G: 255, B: 64, A: 255})         // green
		drawTintRect(screen, 0, halfH, halfW, sh-halfH, color.RGBA{R: 64, G: 96, B: 255, A: 255})         // blue
		drawTintRect(screen, halfW, halfH, sw-halfW, sh-halfH, color.RGBA{R: 255, G: 230, B: 80, A: 255}) // yellow
	}

	// Readback truth probe: log a few sample points once per phase (frame 1 and 31).
	if frame == 1 || frame == 31 {
		want := 4 * sw * sh
		if pixels == nil {
			tmp := make([]byte, want)
			pixels = &tmp
		}
		if *pixels == nil || len(*pixels) != want {
			*pixels = make([]byte, want)
		}
		screen.ReadPixels(*pixels)

		sample := func(x, y int) (byte, byte, byte, byte) {
			if x < 0 {
				x = 0
			}
			if y < 0 {
				y = 0
			}
			if x >= sw {
				x = sw - 1
			}
			if y >= sh {
				y = sh - 1
			}
			i := 4 * (y*sw + x)
			return (*pixels)[i], (*pixels)[i+1], (*pixels)[i+2], (*pixels)[i+3]
		}

		cx, cy := sw/2, sh/2
		r1, g1, b1, a1 := sample(cx, cy)
		r2, g2, b2, a2 := sample(24, 24)
		r3, g3, b3, a3 := sample(sw-24, 24)
		log.Printf(
			"diag: frame=%d phase=%s screen=%dx%d center_rgba=%d,%d,%d,%d tl_rgba=%d,%d,%d,%d tr_rgba=%d,%d,%d,%d",
			frame,
			diagDrawPhaseName(phase),
			sw,
			sh,
			r1, g1, b1, a1,
			r2, g2, b2, a2,
			r3, g3, b3, a3,
		)
	}

	return true
}

func drawGoSignatureProbe(screen *ebiten.Image, frame int, buildLogged *bool) bool {
	if screen == nil {
		return false
	}

	if frame <= 5 {
		log.Printf("diag: signature_probe frame=%d", frame)
		if buildLogged != nil {
			*buildLogged = true
		}
	}

	bounds := screen.Bounds()
	sw, sh := bounds.Dx(), bounds.Dy()
	if sw <= 0 || sh <= 0 {
		return true
	}

	halfW := sw / 2
	halfH := sh / 2

	fillProbeRect(screen, 0, 0, halfW, halfH, color.RGBA{R: 255, G: 0, B: 0, A: 255})
	fillProbeRect(screen, halfW, 0, sw-halfW, halfH, color.RGBA{R: 0, G: 255, B: 0, A: 255})
	fillProbeRect(screen, 0, halfH, halfW, sh-halfH, color.RGBA{R: 0, G: 0, B: 255, A: 255})
	fillProbeRect(screen, halfW, halfH, sw-halfW, sh-halfH, color.RGBA{R: 255, G: 255, B: 0, A: 255})

	markerThickness := sw / 12
	if markerThickness < 24 {
		markerThickness = 24
	}
	markerHeight := (sh * 4) / 5
	if markerHeight < markerThickness {
		markerHeight = markerThickness
	}
	markerWidth := (sw * 3) / 5
	if markerWidth < markerThickness {
		markerWidth = markerThickness
	}
	markerX := sw / 14
	markerY := sh / 10

	fillProbeRect(screen, markerX, markerY, markerThickness, markerHeight, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	fillProbeRect(screen, markerX, markerY, markerWidth, markerThickness, color.RGBA{R: 255, G: 255, B: 255, A: 255})

	return true
}

func fillProbeRect(screen *ebiten.Image, x, y, w, h int, clr color.Color) {
	if screen == nil || w <= 0 || h <= 0 {
		return
	}
	bounds := screen.Bounds()
	if x < bounds.Min.X {
		w -= bounds.Min.X - x
		x = bounds.Min.X
	}
	if y < bounds.Min.Y {
		h -= bounds.Min.Y - y
		y = bounds.Min.Y
	}
	if x+w > bounds.Max.X {
		w = bounds.Max.X - x
	}
	if y+h > bounds.Max.Y {
		h = bounds.Max.Y - y
	}
	if w <= 0 || h <= 0 {
		return
	}
	screen.SubImage(image.Rect(x, y, x+w, y+h)).(*ebiten.Image).Fill(clr)
}
