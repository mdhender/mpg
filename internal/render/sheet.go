// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package render

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Downscale returns img resized to w × h pixels with a box filter: each
// output pixel is the integer mean of the source pixels whose indices fall in
// its share of the source, [x·sw/w, (x+1)·sw/w) by [y·sh/h, (y+1)·sh/h), and
// at least one pixel, so enlarging repeats pixels (nearest neighbor). Integer
// arithmetic only, so the result is the same on every machine. Colors are
// taken as 8-bit color.RGBA (alpha premultiplied). It panics if w or h is not
// positive or img is empty.
func Downscale(img image.Image, w, h int) *image.RGBA {
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if w < 1 || h < 1 || sw < 1 || sh < 1 {
		panic(fmt.Sprintf("render: Downscale %dx%d image to %dx%d", sw, sh, w, h))
	}
	src, ok := img.(*image.RGBA)
	if !ok {
		src = image.NewRGBA(image.Rect(0, 0, sw, sh))
		draw.Draw(src, src.Rect, img, b.Min, draw.Src)
		b = src.Rect
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	span := func(k, n, sn int) (int, int) {
		lo := k * sn / n
		return lo, max((k+1)*sn/n, lo+1)
	}
	for y := range h {
		y0, y1 := span(y, h, sh)
		for x := range w {
			x0, x1 := span(x, w, sw)
			var sum [4]int
			for sy := y0; sy < y1; sy++ {
				o := src.PixOffset(b.Min.X+x0, b.Min.Y+sy)
				for range x1 - x0 {
					for c := range 4 {
						sum[c] += int(src.Pix[o+c])
					}
					o += 4
				}
			}
			n := (x1 - x0) * (y1 - y0)
			o := dst.PixOffset(x, y)
			for c := range 4 {
				dst.Pix[o+c] = uint8((sum[c] + n/2) / n)
			}
		}
	}
	return dst
}

// Label metrics: the fixed 7 × 13 bitmap font from x/image/font/basicfont.
const (
	// LabelAdvance is the width of one label character in pixels.
	LabelAdvance = 7
	// LabelHeight is the height of one label line in pixels.
	LabelHeight = 13
)

// Label draws s in c on img, one line, with its top-left corner at r.Min and
// clipped to r: a string wider than r is cut and ends with "~". Characters
// outside printable ASCII draw as "?". Nothing is drawn outside r, so long
// labels and tiny rectangles are safe.
func Label(img *image.RGBA, r image.Rectangle, s string, c color.RGBA) {
	r = r.Intersect(img.Bounds())
	if r.Empty() {
		return
	}
	text := []byte(s)
	for k, ch := range text {
		if ch < 0x20 || ch > 0x7e {
			text[k] = '?'
		}
	}
	if fit := r.Dx() / LabelAdvance; len(text) > fit {
		if fit < 1 {
			return
		}
		text = append(text[:fit-1], '~')
	}
	d := font.Drawer{
		Dst:  img.SubImage(r).(*image.RGBA),
		Src:  image.NewUniform(c),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(r.Min.X, r.Min.Y+basicfont.Face7x13.Ascent),
	}
	d.DrawString(string(text))
}

// SheetRow is one row of a contact sheet.
type SheetRow struct {
	// Label is drawn in the left column, one line per entry.
	Label []string
	// TileHeight is the height of the row's tiles in pixels.
	TileHeight int
	// Tiles holds one tile per column, each TileWidth × TileHeight; a nil
	// image draws a placeholder.
	Tiles []SheetTile
}

// SheetTile is one cell of a contact sheet: an image and caption lines drawn
// under it.
type SheetTile struct {
	Image   *image.RGBA
	Caption []string
}

// Sheet is a contact sheet: a header row of column names, then rows of
// tiles, each row labeled on the left.
type Sheet struct {
	// Columns names the columns in the header row.
	Columns []string
	// TileWidth is the width of every tile in pixels.
	TileWidth int
	// CaptionLines is the number of caption lines reserved under every tile.
	CaptionLines int
	// LabelChars is the width of the row-label column in characters.
	LabelChars int
	Rows       []SheetRow
}

// Contact sheet colors and spacing.
var (
	sheetBackground  = rgb(0x1e1e1e)
	sheetText        = rgb(0xe6e6e6)
	sheetDim         = rgb(0xa0a0a0)
	sheetPlaceholder = rgb(0x3c3c3c)
)

const (
	sheetGap   = 8
	captionPad = 2 // between a tile and its caption
)

// Image draws the sheet. It panics if a tile is not TileWidth × TileHeight or
// a row has the wrong number of tiles.
func (s *Sheet) Image() *image.RGBA {
	labelW := max(s.LabelChars, 1) * LabelAdvance
	captionH := s.CaptionLines*LabelHeight + captionPad
	headerH := LabelHeight + sheetGap/2
	colX := func(k int) int { return sheetGap + labelW + sheetGap + k*(s.TileWidth+sheetGap) }
	width := colX(len(s.Columns)) // includes the trailing gap
	height := sheetGap + headerH
	for _, row := range s.Rows {
		height += row.TileHeight + captionH + sheetGap
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Rect, image.NewUniform(sheetBackground), image.Point{}, draw.Src)

	for k, name := range s.Columns {
		Label(img, image.Rect(colX(k), sheetGap, colX(k)+s.TileWidth, sheetGap+LabelHeight), name, sheetText)
	}
	y := sheetGap + headerH
	for n, row := range s.Rows {
		if len(row.Tiles) != len(s.Columns) {
			panic(fmt.Sprintf("render: sheet row %d has %d tiles for %d columns", n, len(row.Tiles), len(s.Columns)))
		}
		for l, line := range row.Label {
			Label(img, image.Rect(sheetGap, y+l*LabelHeight, sheetGap+labelW, y+(l+1)*LabelHeight), line, sheetText)
		}
		for k, tile := range row.Tiles {
			r := image.Rect(colX(k), y, colX(k)+s.TileWidth, y+row.TileHeight)
			if tile.Image == nil {
				draw.Draw(img, r, image.NewUniform(sheetPlaceholder), image.Point{}, draw.Src)
				Label(img, r.Add(image.Pt(4, 4)).Intersect(r), "no render", sheetDim)
			} else {
				if tile.Image.Rect.Size() != r.Size() {
					panic(fmt.Sprintf("render: sheet row %d tile %d is %v, want %v", n, k, tile.Image.Rect.Size(), r.Size()))
				}
				draw.Draw(img, r, tile.Image, tile.Image.Rect.Min, draw.Src)
			}
			for l, line := range tile.Caption[:min(len(tile.Caption), s.CaptionLines)] {
				top := r.Max.Y + captionPad + l*LabelHeight
				Label(img, image.Rect(r.Min.X, top, r.Max.X, top+LabelHeight), line, sheetDim)
			}
		}
		y += row.TileHeight + captionH + sheetGap
	}
	return img
}
