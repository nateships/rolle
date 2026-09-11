// Command icons renders the Rolle mark as PNG files: the app icon and a
// monochrome template icon for the system tray.
package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

func main() {
	out := "apps/desktop/build"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	must(write(filepath.Join(out, "appicon.png"), render(1024, false)))
	must(write(filepath.Join(out, "trayicon.png"), render(44, true)))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func write(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// render draws three concentric dashed rings and a centre dot. Template icons
// are black on transparent; the app icon sits on a rounded dark tile.
func render(size int, template bool) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	s := float64(size)
	c := s / 2
	if !template {
		fillRoundedRect(img, s*0.22, color.RGBA{9, 9, 11, 255})
	}
	ink := color.RGBA{0, 0, 0, 255}
	if !template {
		ink = color.RGBA{245, 158, 11, 255}
	}
	rings := []struct{ r, w, gap, start float64 }{
		{0.40, 0.045, 0.30, 0.10},
		{0.28, 0.045, 0.40, 0.55},
		{0.16, 0.055, 0.45, 0.20},
	}
	for i, ring := range rings {
		alpha := 1.0 - float64(i)*0.22
		drawRing(img, c, c, ring.r*s, ring.w*s, ring.gap, ring.start, fade(ink, alpha))
	}
	drawDisc(img, c, c, s*0.045, ink)
	return img
}

func fade(c color.RGBA, a float64) color.RGBA {
	return color.RGBA{c.R, c.G, c.B, uint8(float64(c.A) * a)}
}

func drawDisc(img *image.RGBA, cx, cy, r float64, col color.RGBA) {
	drawRing(img, cx, cy, r/2, r, 0, 0, col)
}

// drawRing paints an anti-aliased ring of radius r and stroke width w. gap is the
// fraction of the circumference left blank, starting at angle start (turns).
func drawRing(img *image.RGBA, cx, cy, r, w, gap, start float64, col color.RGBA) {
	b := img.Bounds()
	inner, outer := r-w/2, r+w/2
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			d := math.Hypot(dx, dy)
			if d < inner-1 || d > outer+1 {
				continue
			}
			cover := clamp(math.Min(d-inner, outer-d)+0.5, 0, 1)
			if gap > 0 {
				turn := math.Mod(math.Atan2(dy, dx)/(2*math.Pi)+1-start, 1)
				if turn > 1-gap {
					continue
				}
			}
			blend(img, x, y, col, cover)
		}
	}
}

func fillRoundedRect(img *image.RGBA, radius float64, col color.RGBA) {
	b := img.Bounds()
	w, h := float64(b.Dx()), float64(b.Dy())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			qx := math.Max(math.Abs(px-w/2)-(w/2-radius), 0)
			qy := math.Max(math.Abs(py-h/2)-(h/2-radius), 0)
			d := math.Hypot(qx, qy) - radius
			blend(img, x, y, col, clamp(0.5-d, 0, 1))
		}
	}
}

func blend(img *image.RGBA, x, y int, col color.RGBA, cover float64) {
	if cover <= 0 {
		return
	}
	a := float64(col.A) / 255 * cover
	dst := img.RGBAAt(x, y)
	da := float64(dst.A) / 255
	outA := a + da*(1-a)
	mix := func(s, d uint8) uint8 {
		if outA == 0 {
			return 0
		}
		return uint8((float64(s)*a + float64(d)*da*(1-a)) / outA)
	}
	img.SetRGBA(x, y, color.RGBA{mix(col.R, dst.R), mix(col.G, dst.G), mix(col.B, dst.B), uint8(outA * 255)})
}

func clamp(v, lo, hi float64) float64 { return math.Max(lo, math.Min(hi, v)) }
