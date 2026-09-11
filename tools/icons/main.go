// Command icons renders Rolle's desktop mark: three credential cards with an r
// cutout. The Go gopher belongs to the README artwork, never the desktop icons.
// Run from the repository root: go run ./tools/icons [output-directory]
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Paths share a 512-unit canvas. Keep these as the single source for SVG and
// raster exports. The open r counter is geometry, not a font-dependent glyph.
var cards = []string{
	"M 110 98 L 276 98 Q 294 98 300 116 L 306 132 L 138 132 Q 120 132 120 150 L 120 336 L 102 330 Q 80 324 80 302 L 80 128 Q 80 98 110 98 Z",
	"M 164 146 L 328 146 Q 346 146 352 164 L 358 180 L 192 180 Q 174 180 174 198 L 174 370 L 156 364 Q 134 358 134 336 L 134 176 Q 134 146 164 146 Z",
	"M 216 194 L 402 194 Q 432 194 432 224 L 432 384 Q 432 414 402 414 L 312 414 L 312 346 Q 312 316 342 316 L 366 316 L 366 282 L 336 282 Q 264 282 264 354 L 264 414 L 216 414 Q 186 414 186 384 L 186 224 Q 186 194 216 194 Z",
}

const tile = "M 128 16 L 384 16 Q 496 16 496 128 L 496 384 Q 496 496 384 496 L 128 496 Q 16 496 16 384 L 16 128 Q 16 16 128 16 Z"

// cloudCards holds the Original Circuit palette from back to front: cobalt blue, orange, green.
var cloudCards = [3]color.NRGBA{{36, 76, 255, 255}, {255, 121, 0, 255}, {0, 206, 120, 255}}
var charcoal = color.NRGBA{24, 26, 30, 255}
var ivory = color.NRGBA{244, 240, 232, 255}
var templateCards = [3]color.NRGBA{{0, 0, 0, 255}, {0, 0, 0, 255}, {0, 0, 0, 255}}

func main() {
	out := "apps/desktop/build"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	must(os.MkdirAll(filepath.Join(out, "icons"), 0755))
	for _, variant := range []struct {
		name       string
		inks       [3]color.NRGBA
		background color.NRGBA
	}{
		{"appicon", cloudCards, charcoal},
		{"appicon-dark", cloudCards, charcoal},
		{"appicon-light", cloudCards, ivory},
		{"mark", cloudCards, color.NRGBA{}},
		{"trayicon", templateCards, color.NRGBA{}},
	} {
		must(os.WriteFile(filepath.Join(out, variant.name+".svg"), []byte(svg(variant.inks, variant.background)), 0644))
		size := 1024
		if variant.name == "trayicon" {
			size = 44
		}
		must(writePNG(filepath.Join(out, variant.name+".png"), render(size, variant.inks, variant.background)))
	}
	for _, size := range []int{16, 24, 32, 48, 64, 128, 256, 512, 1024} {
		must(writePNG(filepath.Join(out, "icons", fmt.Sprintf("icon-%d.png", size)), render(size, cloudCards, charcoal)))
	}
	must(writePNG(filepath.Join(out, "icons", "tray-22.png"), render(22, templateCards, color.NRGBA{})))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err = png.Encode(f, img); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func hex(c color.NRGBA) string { return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B) }

func svg(inks [3]color.NRGBA, background color.NRGBA) string {
	var b strings.Builder
	b.WriteString("<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"1024\" height=\"1024\" viewBox=\"0 0 512 512\" role=\"img\" aria-label=\"Rolle credential stack with r cutout\">\n")
	if background.A != 0 {
		fmt.Fprintf(&b, "  <path fill=\"%s\" d=\"%s\"/>\n", hex(background), tile)
	}
	for i, path := range cards {
		fmt.Fprintf(&b, "  <path fill=\"%s\" d=\"%s\"/>\n", hex(inks[i]), path)
	}
	b.WriteString("</svg>\n")
	return b.String()
}

type point struct{ x, y float64 }

// polygon flattens a path that uses the uppercase M, L, Q, and Z commands.
func polygon(path string) []point {
	tokens := strings.Fields(path)
	var points []point
	var current point
	i := 0
	number := func() float64 { n, err := strconv.ParseFloat(tokens[i], 64); must(err); i++; return n }
	for i < len(tokens) {
		command := tokens[i]
		i++
		switch command {
		case "M", "L":
			current = point{number(), number()}
			points = append(points, current)
		case "Q":
			control, end := point{number(), number()}, point{number(), number()}
			start := current
			for step := 1; step <= 64; step++ {
				t := float64(step) / 64
				u := 1 - t
				points = append(points, point{u*u*start.x + 2*u*t*control.x + t*t*end.x, u*u*start.y + 2*u*t*control.y + t*t*end.y})
			}
			current = end
		case "Z":
		default:
			panic("unsupported SVG path command: " + command)
		}
	}
	return points
}

// fill rasterizes a path with 4x scanline supersampling. It needs no extra
// libraries, fonts, network access, or platform drawing APIs.
func fill(img *image.NRGBA, path string, ink color.NRGBA) {
	points := polygon(path)
	scale := float64(img.Bounds().Dx()) / 512
	for i := range points {
		points[i].x *= scale
		points[i].y *= scale
	}
	for y := 0; y < img.Bounds().Dy(); y++ {
		py := float64(y) + 0.5
		var hits []float64
		previous := points[len(points)-1]
		for _, next := range points {
			if (previous.y <= py && next.y > py) || (next.y <= py && previous.y > py) {
				hits = append(hits, previous.x+(py-previous.y)*(next.x-previous.x)/(next.y-previous.y))
			}
			previous = next
		}
		sort.Float64s(hits)
		for i := 0; i+1 < len(hits); i += 2 {
			left := max(0, int(math.Ceil(hits[i]-0.5)))
			right := min(img.Bounds().Dx(), int(math.Ceil(hits[i+1]-0.5)))
			for x := left; x < right; x++ {
				img.SetNRGBA(x, y, ink)
			}
		}
	}
}

func render(size int, inks [3]color.NRGBA, background color.NRGBA) *image.NRGBA {
	const aa = 4
	high := image.NewNRGBA(image.Rect(0, 0, size*aa, size*aa))
	if background.A != 0 {
		fill(high, tile, background)
	}
	for i, path := range cards {
		fill(high, path, inks[i])
	}
	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var r, g, b, a uint32
			for dy := 0; dy < aa; dy++ {
				for dx := 0; dx < aa; dx++ {
					c := high.NRGBAAt(x*aa+dx, y*aa+dy)
					a += uint32(c.A)
					r += uint32(c.R) * uint32(c.A)
					g += uint32(c.G) * uint32(c.A)
					b += uint32(c.B) * uint32(c.A)
				}
			}
			if a != 0 {
				out.SetNRGBA(x, y, color.NRGBA{uint8(r / a), uint8(g / a), uint8(b / a), uint8((a + aa*aa/2) / (aa * aa))})
			}
		}
	}
	return out
}
