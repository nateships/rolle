package main

import (
	"bytes"
	"testing"
)

func TestRecolorDotPaintsOnlyTheDot(t *testing.T) {
	out, err := recolorDot(trayIconColorActive, trayIconColor, amber)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := decodeNRGBA(out)
	active, _ := decodeNRGBA(trayIconColorActive)
	plain, _ := decodeNRGBA(trayIconColor)
	changed, kept := 0, 0
	for i := 0; i < len(got.Pix); i += 4 {
		isDot := got.Pix[i+3] != 0 && !bytes.Equal(active.Pix[i:i+4], plain.Pix[i:i+4])
		switch {
		case isDot:
			changed++
			if got.Pix[i] != amber.R || got.Pix[i+1] != amber.G || got.Pix[i+2] != amber.B || got.Pix[i+3] != active.Pix[i+3] {
				t.Fatalf("dot pixel %d = %v", i/4, got.Pix[i:i+4])
			}
		default:
			kept++
			if !bytes.Equal(got.Pix[i:i+4], active.Pix[i:i+4]) {
				t.Fatalf("mark pixel %d changed: %v", i/4, got.Pix[i:i+4])
			}
		}
	}
	if changed == 0 || kept == 0 {
		t.Fatalf("changed=%d kept=%d", changed, kept)
	}
}
