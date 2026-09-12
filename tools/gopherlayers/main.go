// gopherlayers splits the flattened gopher artwork into two PNG layers: the
// cards, with the area under the gopher filled in each card's own colour, and
// the gopher alone. The desktop app stacks them so the gopher can move on its
// own. Usage: go run ./tools/gopherlayers <gopher.png> <outdir>
package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

type px struct{ r, g, b, a float64 }

func clamp(x float64) float64 { return math.Max(0, math.Min(1, x)) }

func main() {
	f, _ := os.Open(os.Args[1])
	src, _ := png.Decode(f)
	b := src.Bounds()
	W, H := b.Dx(), b.Dy()
	get := func(x, y int) px {
		c := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA)
		return px{float64(c.R) / 255, float64(c.G) / 255, float64(c.B) / 255, float64(c.A) / 255}
	}
	neutral := func(p px) float64 {
		mx := math.Max(p.r, math.Max(p.g, p.b))
		mn := math.Min(p.r, math.Min(p.g, p.b))
		sat := 0.0
		if mx > 0 {
			sat = (mx - mn) / mx
		}
		lum := 0.2126*p.r + 0.7152*p.g + 0.0722*p.b
		// The cards are bright flat colours; anything dark and solid is the
		// gopher's line work. Card edges fade through partial alpha, so they
		// stay with the cards.
		dark := 0.0
		blue := p.b >= p.r && p.b >= p.g
		if p.a > 0.95 && !blue {
			dark = clamp((0.32 - lum) / 0.10)
		}
		return math.Max(clamp((0.40-sat)/0.25), dark)
	}
	// Card classification by hue: green, orange, blue.
	kind := func(p px) int {
		if p.a < 0.5 || neutral(p) > 0.5 {
			return -1
		}
		if p.g > p.r && p.g > p.b {
			return 2 // green (front)
		}
		if p.r > p.g && p.r > p.b {
			return 1 // orange
		}
		return 0 // blue (back)
	}
	// Bounding boxes and mean colors per card.
	type box struct {
		x0, y0, x1, y1 int
		sr, sg, sb     float64
		n              float64
	}
	boxes := [3]box{}
	for i := range boxes {
		boxes[i] = box{x0: W, y0: H, x1: -1, y1: -1}
	}
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			p := get(x, y)
			k := kind(p)
			if k < 0 {
				continue
			}
			bx := &boxes[k]
			if x < bx.x0 {
				bx.x0 = x
			}
			if y < bx.y0 {
				bx.y0 = y
			}
			if x > bx.x1 {
				bx.x1 = x
			}
			if y > bx.y1 {
				bx.y1 = y
			}
			bx.sr += p.r
			bx.sg += p.g
			bx.sb += p.b
			bx.n++
		}
	}
	// Gopher mask, then a copy grown by two pixels so the fringe where the
	// outline met a card is filled instead of kept.
	mask := make([]float64, W*H)
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			p := get(x, y)
			if p.a > 0 {
				mask[y*W+x] = neutral(p) * p.a
			}
		}
	}
	grown := func(x, y int) float64 {
		m := 0.0
		for dy := -2; dy <= 2; dy++ {
			for dx := -2; dx <= 2; dx++ {
				xx, yy := x+dx, y+dy
				if xx >= 0 && yy >= 0 && xx < W && yy < H {
					m = math.Max(m, mask[yy*W+xx])
				}
			}
		}
		return m
	}
	gopher := image.NewNRGBA(b)
	cards := image.NewNRGBA(b)
	for y := 0; y < H; y++ {
		for x := 0; x < W; x++ {
			p := get(x, y)
			w := neutral(p)
			if p.a == 0 {
				continue
			}
			// Gopher keeps its own colour with alpha scaled by how neutral it is.
			gopher.SetNRGBA(x, y, color.NRGBA{uint8(p.r*255 + .5), uint8(p.g*255 + .5), uint8(p.b*255 + .5), uint8(p.a * w * 255)})
			if grown(x, y) < 0.3 {
				cards.SetNRGBA(x, y, color.NRGBA{uint8(p.r*255 + .5), uint8(p.g*255 + .5), uint8(p.b*255 + .5), uint8(p.a * (1 - w) * 255)})
				continue
			}
			// Under the gopher: fill with the frontmost card whose box holds the pixel.
			for k := 2; k >= 0; k-- {
				bx := boxes[k]
				if x >= bx.x0 && x <= bx.x1 && y >= bx.y0 && y <= bx.y1 && bx.n > 0 {
					// The card is solid under the gopher, whatever alpha the
					// keyed outline pixel carries.
					cards.SetNRGBA(x, y, color.NRGBA{uint8(bx.sr/bx.n*255 + .5), uint8(bx.sg/bx.n*255 + .5), uint8(bx.sb/bx.n*255 + .5), 255})
					break
				}
			}
		}
	}
	out := "."
	if len(os.Args) > 2 {
		out = os.Args[2]
	}
	write := func(name string, img image.Image) {
		o, err := os.Create(out + "/" + name)
		if err != nil {
			panic(err)
		}
		if err := png.Encode(o, img); err != nil {
			panic(err)
		}
		if err := o.Close(); err != nil {
			panic(err)
		}
	}
	write("gopher-peek.png", gopher)
	write("gopher-cards.png", cards)
}
