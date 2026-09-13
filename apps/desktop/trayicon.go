package main

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
)

// amber marks a tray state that needs attention soon.
var amber = color.NRGBA{R: 0xF5, G: 0xA5, B: 0x24, A: 0xFF}

// recolorDot returns the active icon with its status dot painted in c. The
// dot is every pixel where the active icon differs from the plain one, so
// the mark's own green stays as it is. Each dot pixel keeps its alpha, so
// the anti-aliased edge stays soft.
func recolorDot(active, plain []byte, c color.NRGBA) ([]byte, error) {
	a, err := decodeNRGBA(active)
	if err != nil {
		return nil, err
	}
	p, err := decodeNRGBA(plain)
	if err != nil {
		return nil, err
	}
	if a.Bounds() != p.Bounds() {
		return nil, errors.New("tray icons differ in size")
	}
	for i := 0; i < len(a.Pix); i += 4 {
		if a.Pix[i+3] == 0 || bytes.Equal(a.Pix[i:i+4], p.Pix[i:i+4]) {
			continue
		}
		a.Pix[i], a.Pix[i+1], a.Pix[i+2] = c.R, c.G, c.B
	}
	var out bytes.Buffer
	if err := png.Encode(&out, a); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func decodeNRGBA(data []byte) (*image.NRGBA, error) {
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	img := image.NewNRGBA(b)
	draw.Draw(img, b, src, b.Min, draw.Src)
	return img, nil
}
