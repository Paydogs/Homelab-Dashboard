// Command genicons renders the dashboard's PWA icon PNGs into
// web/static/icons/. Dev-only asset tool, like the fonts dropped into style
// folders: run it once and commit the PNGs — nothing on the serving path
// depends on it. Regenerate with `go run ./cmd/genicons [outdir]`.
//
// The glyph mirrors web/static/icons/globe.svg in a 24-unit view space:
// circle ring + equator lines + two meridian arcs, stroked in the Resurrect
// 64 palette colors used by main.css and the styles.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

// Resurrect 64: background matches main.css :root --bg, stroke matches
// globe.svg.
var (
	bg     = hexColor("#2e222f")
	stroke = hexColor("#9babb2")
)

// halfStroke is the stroke half-width in view units (stroke-width 1.5 on a
// 24-unit canvas, as in globe.svg).
const halfStroke = 0.75

func hexColor(s string) color.RGBA {
	var r, g, b uint8
	fmt.Sscanf(s, "#%02x%02x%02x", &r, &g, &b)
	return color.RGBA{r, g, b, 255}
}

// inGlyph reports whether a point in the 24-unit view space lies on one of
// globe.svg's stroked primitives (exactly reproducible as circle/line
// distance bands).
func inGlyph(x, y float64) bool {
	band := func(d float64) bool { return math.Abs(d) <= halfStroke }
	switch {
	case band(dist(x, y, 12, 12) - 9): // outer ring
		return true
	case band(y-9) && math.Abs(x-12) <= 8.4: // upper equator line
		return true
	case band(y-15) && math.Abs(x-12) <= 8.4: // lower equator line
		return true
	case band(dist(x, y, 0, 12)-15) && x >= 12: // right meridian arc
		return true
	case band(dist(x, y, 24, 12)-15) && x <= 12: // left meridian arc
		return true
	}
	return false
}

func dist(x1, y1, x2, y2 float64) float64 {
	return math.Hypot(x1-x2, y1-y2)
}

// mix composites the glyph color over the background at coverage a.
func mix(bg, glyph color.RGBA, a float64) color.RGBA {
	lerp := func(bg, glyph uint8) uint8 {
		return uint8(float64(bg)*(1-a) + float64(glyph)*a)
	}
	return color.RGBA{lerp(bg.R, glyph.R), lerp(bg.G, glyph.G), lerp(bg.B, glyph.B), 255}
}

// drawIcon renders one square icon. glyphFrac is the globe's diameter as a
// fraction of the canvas; opaque adds a full-bleed background (for maskable
// and apple-touch icons, which the OS crops to its mask edges).
func drawIcon(size int, glyphFrac float64, opaque bool) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	// view-space units per pixel, so the 18-unit glyph spans glyphFrac*size
	upp := 18 / (glyphFrac * float64(size))
	const ss = 4 // 4x4 supersamples per pixel for anti-aliasing
	for py := 0; py < size; py++ {
		for px := 0; px < size; px++ {
			hits := 0
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					x := 12 + (float64(px)+(float64(sx)+0.5)/ss-float64(size)/2)*upp
					y := 12 + (float64(py)+(float64(sy)+0.5)/ss-float64(size)/2)*upp
					if inGlyph(x, y) {
						hits++
					}
				}
			}
			a := float64(hits) / (ss * ss)
			var c color.RGBA
			if opaque {
				c = mix(bg, stroke, a)
			} else {
				c = color.RGBA{stroke.R, stroke.G, stroke.B, uint8(a * 255)}
			}
			img.SetRGBA(px, py, c)
		}
	}
	return img
}

func main() {
	dir := "web/static/icons"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fatal(err)
	}
	icons := []struct {
		name   string
		size   int
		frac   float64
		opaque bool
	}{
		{"icon-192.png", 192, 0.85, false},
		{"icon-512.png", 512, 0.85, false},
		// maskable crops to the OS mask edges: full-bleed background, glyph
		// shrunk well into the safe zone
		{"icon-maskable-512.png", 512, 0.62, true},
		{"apple-touch-icon.png", 180, 0.62, true},
	}
	for _, ic := range icons {
		path := filepath.Join(dir, ic.name)
		f, err := os.Create(path)
		if err != nil {
			fatal(err)
		}
		if err := png.Encode(f, drawIcon(ic.size, ic.frac, ic.opaque)); err != nil {
			f.Close()
			fatal(err)
		}
		if err := f.Close(); err != nil {
			fatal(err)
		}
		fmt.Println("wrote", path)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "genicons:", err)
	os.Exit(1)
}
