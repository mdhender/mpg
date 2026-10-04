// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package render

import (
	"image"
	"image/color"
	"strings"
	"testing"
)

func TestDownscale(t *testing.T) {
	// 4 × 2 → 2 × 1: each output pixel is the mean of a 2 × 2 block.
	src := image.NewRGBA(image.Rect(0, 0, 4, 2))
	for y := range 2 {
		for x := range 4 {
			src.SetRGBA(x, y, color.RGBA{uint8(10 * (x + 4*y)), 0, 255, 255})
		}
	}
	got := Downscale(src, 2, 1)
	// (0 + 10 + 40 + 50)/4 = 25, (20 + 30 + 60 + 70)/4 = 45
	if got.Bounds() != image.Rect(0, 0, 2, 1) || got.RGBAAt(0, 0).R != 25 || got.RGBAAt(1, 0).R != 45 || got.RGBAAt(1, 0).B != 255 {
		t.Errorf("Downscale = %v %v", got.RGBAAt(0, 0), got.RGBAAt(1, 0))
	}

	// Enlarging repeats pixels.
	up := Downscale(src, 8, 4)
	for y := range 4 {
		for x := range 8 {
			if up.RGBAAt(x, y) != src.RGBAAt(x/2, y/2) {
				t.Fatalf("enlarged (%d, %d) = %v, want %v", x, y, up.RGBAAt(x, y), src.RGBAAt(x/2, y/2))
			}
		}
	}

	// Same size is a copy; a sub-image and a non-RGBA image work too.
	if PixelHash(Downscale(src, 4, 2)) != PixelHash(src) {
		t.Error("same-size Downscale changed the image")
	}
	sub := src.SubImage(image.Rect(2, 0, 4, 2))
	if g := Downscale(sub, 1, 1); g.RGBAAt(0, 0).R != 45 {
		t.Errorf("sub-image Downscale = %v", g.RGBAAt(0, 0))
	}
	gray := image.NewGray(image.Rect(0, 0, 3, 3))
	gray.Pix[4] = 90
	if g := Downscale(gray, 1, 1); g.RGBAAt(0, 0) != (color.RGBA{10, 10, 10, 255}) {
		t.Errorf("gray Downscale = %v", g.RGBAAt(0, 0))
	}

	defer func() {
		if recover() == nil {
			t.Error("Downscale to 0 × 1 did not panic")
		}
	}()
	Downscale(src, 0, 1)
}

func TestLabelClips(t *testing.T) {
	white := color.RGBA{255, 255, 255, 255}
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	r := image.Rect(5, 5, 26, 18) // three characters wide
	Label(img, r, strings.Repeat("seed 18446744073709551615 ", 20)+"\x00\xff", white)
	drawn := 0
	for y := range 30 {
		for x := range 40 {
			if img.RGBAAt(x, y).A != 0 {
				if !(image.Pt(x, y).In(r)) {
					t.Fatalf("label drew outside its rectangle at (%d, %d)", x, y)
				}
				drawn++
			}
		}
	}
	if drawn == 0 {
		t.Error("label drew nothing")
	}
	// Degenerate rectangles draw nothing and do not panic.
	for _, r := range []image.Rectangle{{}, image.Rect(0, 0, 3, 13), image.Rect(35, 25, 80, 80), image.Rect(-50, -50, -1, -1)} {
		before := PixelHash(img)
		Label(img, r, "label", white)
		if r.Dx() < LabelAdvance && PixelHash(img) != before {
			t.Errorf("label in %v drew", r)
		}
	}
}

func TestSheetLayout(t *testing.T) {
	tile := func(w, h int) *image.RGBA {
		m := image.NewRGBA(image.Rect(0, 0, w, h))
		for k := range m.Pix {
			m.Pix[k] = 200
		}
		return m
	}
	s := Sheet{
		Columns:      []string{"noise", "a-very-long-stage-name-that-does-not-fit"},
		TileWidth:    40,
		CaptionLines: 2,
		LabelChars:   6,
		Rows: []SheetRow{
			{Label: []string{"seed 1", "cinematic"}, TileHeight: 17, Tiles: []SheetTile{{Image: tile(40, 17), Caption: []string{"one", "two", "dropped"}}, {}}},
			{Label: []string{"seed 2"}, TileHeight: 40, Tiles: []SheetTile{{Image: tile(40, 40)}, {Image: tile(40, 40)}}},
		},
	}
	img := s.Image()
	capH := 2*LabelHeight + captionPad
	wantW := sheetGap + 6*LabelAdvance + sheetGap + 2*(40+sheetGap)
	wantH := sheetGap + LabelHeight + sheetGap/2 + (17 + capH + sheetGap) + (40 + capH + sheetGap)
	if img.Rect != image.Rect(0, 0, wantW, wantH) {
		t.Fatalf("sheet is %v, want %d × %d", img.Rect, wantW, wantH)
	}
	// The first tile sits after the label column, under the header.
	x0, y0 := sheetGap+6*LabelAdvance+sheetGap, sheetGap+LabelHeight+sheetGap/2
	if c := img.RGBAAt(x0+20, y0+8); c.R != 200 {
		t.Errorf("tile pixel = %v", c)
	}
	if c := img.RGBAAt(x0+40+sheetGap+39, y0+16); c != sheetPlaceholder {
		t.Errorf("placeholder pixel = %v", c)
	}
	if PixelHash(img) != PixelHash(s.Image()) {
		t.Error("sheet is not deterministic")
	}

	defer func() {
		if recover() == nil {
			t.Error("a wrongly sized tile did not panic")
		}
	}()
	s.Rows[1].Tiles[0].Image = tile(39, 40)
	s.Image()
}
