package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// at returns the pixel at (x, y) in 512-unit canvas coordinates for an image
// of the given size.
func at(img *image.NRGBA, size, ux, uy int) color.NRGBA {
	return img.NRGBAAt(ux*size/512, uy*size/512)
}

func TestRenderPaintsTileAndCardsInPaletteOrder(t *testing.T) {
	const size = 128
	img := render(size, cloudCards, charcoal)
	if b := img.Bounds(); b.Dx() != size || b.Dy() != size {
		t.Fatalf("bounds = %v, want %dx%d", b, size, size)
	}
	cases := []struct {
		name   string
		ux, uy int
		want   color.NRGBA
	}{
		{"outside the rounded corner is transparent", 0, 0, color.NRGBA{}},
		{"tile above the cards is charcoal", 256, 50, charcoal},
		{"back card is cobalt", 100, 250, cloudCards[0]},
		{"middle card is orange", 150, 300, cloudCards[1]},
		{"front card is green", 400, 230, cloudCards[2]},
		{"r counter shows the tile", 300, 300, charcoal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := at(img, size, tc.ux, tc.uy); got != tc.want {
				t.Fatalf("pixel at (%d,%d) = %v, want %v", tc.ux, tc.uy, got, tc.want)
			}
		})
	}
}

func TestRenderWithoutBackgroundIsTransparentAroundTheCards(t *testing.T) {
	img := render(64, templateCards, color.NRGBA{})
	if got := at(img, 64, 256, 50); got.A != 0 {
		t.Fatalf("tile area painted without a background: %v", got)
	}
	if got := at(img, 64, 400, 230); got != templateCards[2] {
		t.Fatalf("front card = %v, want black template ink", got)
	}
}

func TestRenderAntialiasesEdges(t *testing.T) {
	img := render(64, cloudCards, charcoal)
	partial := 0
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			if a := img.NRGBAAt(x, y).A; a != 0 && a != 255 {
				partial++
			}
		}
	}
	if partial == 0 {
		t.Fatal("no partially covered edge pixels; supersampling is not applied")
	}
}

func TestWithDotDrawsGreenDotWithCharcoalRing(t *testing.T) {
	img := withDot(render(44, cloudCards, color.NRGBA{}))
	if got := img.NRGBAAt(34, 34); got != cloudCards[2] {
		t.Fatalf("dot centre = %v, want green", got)
	}
	if got := img.NRGBAAt(34, 24); got != charcoal {
		t.Fatalf("dot ring = %v, want charcoal", got)
	}
	if got := img.NRGBAAt(2, 2); got.A != 0 {
		t.Fatalf("far corner = %v, want untouched", got)
	}
}

func TestSVGListsBackgroundThenCards(t *testing.T) {
	s := svg(cloudCards, charcoal)
	if !strings.HasPrefix(s, "<svg xmlns=\"http://www.w3.org/2000/svg\"") || !strings.HasSuffix(s, "</svg>\n") {
		t.Fatalf("svg envelope:\n%s", s)
	}
	if strings.Count(s, "<path ") != 4 {
		t.Fatalf("path count = %d, want tile plus three cards", strings.Count(s, "<path "))
	}
	order := []string{`fill="#181A1E" d="` + tile, `fill="#244CFF"`, `fill="#FF7900"`, `fill="#00CE78"`}
	last := -1
	for _, want := range order {
		i := strings.Index(s, want)
		if i < 0 || i < last {
			t.Fatalf("%q missing or out of order:\n%s", want, s)
		}
		last = i
	}
	mark := svg(cloudCards, color.NRGBA{})
	if strings.Count(mark, "<path ") != 3 || strings.Contains(mark, tile) {
		t.Fatalf("transparent variant must omit the tile:\n%s", mark)
	}
}

func TestPolygonFlattensCurves(t *testing.T) {
	if got := polygon("M 0 0 L 10 0 L 10 10 Z"); len(got) != 3 || got[2] != (point{10, 10}) {
		t.Fatalf("straight path = %v", got)
	}
	curve := polygon("M 0 0 Q 10 0 10 10 Z")
	if len(curve) != 1+64 || curve[64] != (point{10, 10}) {
		t.Fatalf("curve has %d points, last %v", len(curve), curve[len(curve)-1])
	}
	mid := curve[32]
	if mid.x <= 5 || mid.x >= 10 || mid.y <= 0 || mid.y >= 5 {
		t.Fatalf("midpoint %v is not on the curve side of the chord", mid)
	}
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("unsupported command did not panic")
		}
	}()
	polygon("M 0 0 C 1 1 2 2 3 3")
}

func TestHex(t *testing.T) {
	if got := hex(color.NRGBA{1, 171, 255, 0}); got != "#01ABFF" {
		t.Fatalf("hex = %q", got)
	}
}

func TestWritePNGRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "icon.png")
	if err := writePNG(path, render(16, cloudCards, charcoal)); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	decoded, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if b := decoded.Bounds(); b.Dx() != 16 || b.Dy() != 16 {
		t.Fatalf("decoded bounds = %v", b)
	}
	if err := writePNG(filepath.Join(t.TempDir(), "missing", "icon.png"), render(4, cloudCards, charcoal)); err == nil {
		t.Fatal("expected an error for a missing directory")
	}
}

func TestMustPanicsOnError(t *testing.T) {
	must(nil)
	defer func() {
		if recover() == nil {
			t.Fatal("must did not panic")
		}
	}()
	must(os.ErrNotExist)
}
