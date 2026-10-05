// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package basin

import (
	"image"
	"image/color"

	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/internal/render"
)

// The basins render's palette. Basins take Palette's colors by the id of
// their top-level basin, so neighbors usually differ, darkened by
// NestShade per level of nesting, so a nested basin shows inside its
// parent. Land outside every basin is a pale gray by altitude, so the
// basins stand out; depressions shallower than the minimum are a pale
// sand; the ocean a flat blue; the rim the ice sheet. Spill edges are drawn
// in SpillInk, and each spill corner marked with a SpillInk disc in a white
// ring.
var (
	Palette = []color.RGBA{
		{0xe6, 0x7e, 0x22, 0xff}, // orange
		{0x8e, 0x44, 0xad, 0xff}, // purple
		{0x27, 0xae, 0x60, 0xff}, // green
		{0xd6, 0x3c, 0x8c, 0xff}, // magenta
		{0x16, 0xa0, 0x85, 0xff}, // teal
		{0xc0, 0x9a, 0x10, 0xff}, // gold
		{0x8d, 0x5a, 0x3c, 0xff}, // brown
		{0x4b, 0x4f, 0xa8, 0xff}, // indigo
	}
	ShallowColor = color.RGBA{0xf0, 0xdc, 0xa0, 0xff}
	OceanColor   = color.RGBA{0x9c, 0xc3, 0xe0, 0xff}
	SpillInk     = color.RGBA{0xd0, 0x10, 0x10, 0xff}
	ringInk      = color.RGBA{0xff, 0xff, 0xff, 0xff}
	// LandRamp grays land outside basins by altitude above sea level.
	LandRamp = render.NewRamp(
		render.Stop{Value: 0, Color: color.RGBA{0xf2, 0xf2, 0xee, 0xff}},
		render.Stop{Value: 3000, Color: color.RGBA{0xa8, 0xa8, 0xa4, 0xff}},
	)
	// DepthRamp colors a basin cell in the "depth" variant by how far
	// below its innermost basin's spill level it lies, in meters.
	DepthRamp = render.NewRamp(
		render.Stop{Value: 0, Color: color.RGBA{0xd4, 0xee, 0xf5, 0xff}},
		render.Stop{Value: 100, Color: color.RGBA{0x7f, 0xc4, 0xdc, 0xff}},
		render.Stop{Value: 300, Color: color.RGBA{0x3a, 0x8f, 0xc0, 0xff}},
		render.Stop{Value: 1000, Color: color.RGBA{0x1d, 0x4a, 0x86, 0xff}},
		render.Stop{Value: 3000, Color: color.RGBA{0x0f, 0x1f, 0x4f, 0xff}},
	)
)

// NestShade is the fraction of brightness a basin's color loses per level
// of nesting.
const NestShade = 0.22

// BasinColor returns the basins render's fill for basin b of r: its
// top-level basin's Palette color, darkened by NestShade per level of
// nesting (to at most three levels).
func BasinColor(r *Result, b int) color.RGBA {
	t, k := b, 0
	for r.Basins[t].Parent != None {
		t = r.Basins[t].Parent
		k++
	}
	c := Palette[t%len(Palette)]
	s := 1 - float64(NestShade*float64(min(k, 3)))
	return color.RGBA{uint8(float64(c.R) * s), uint8(float64(c.G) * s), uint8(float64(c.B) * s), 0xff}
}

// Render draws the basins stage render over mesh.CellRender: basin cells by
// BasinColor of their innermost basin, cells of shallower depressions in
// ShallowColor, other land by LandRamp at its altitude above level, the
// sea (seed cells that are not rim) in OceanColor, and the rim as the ice
// sheet; then every basin's spill edge in SpillInk and its spill corner
// marked.
func Render(f *field.Field, m *mesh.Mesh, alt []float64, seed []bool, level float64, r *Result) *image.RGBA {
	img := mesh.CellRender(f, m, func(i int) color.RGBA {
		switch {
		case m.Cells[i].Rim:
			return mesh.IceColor
		case seed[i]:
			return OceanColor
		case r.Of[i] != None:
			return BasinColor(r, r.Of[i])
		case r.Depression[i] != None:
			return ShallowColor
		}
		return LandRamp.At(alt[i] - level)
	})
	drawSpills(img, f, m, r)
	return img
}

// DepthRender draws the basins stage's "depth" variant: each basin cell by
// DepthRamp at its innermost basin's spill level minus its altitude (the
// water over it were the basin full), otherwise as Render.
func DepthRender(f *field.Field, m *mesh.Mesh, alt []float64, seed []bool, level float64, r *Result) *image.RGBA {
	img := mesh.CellRender(f, m, func(i int) color.RGBA {
		switch {
		case m.Cells[i].Rim:
			return mesh.IceColor
		case seed[i]:
			return OceanColor
		case r.Of[i] != None:
			return DepthRamp.At(r.Basins[r.Of[i]].SpillM - alt[i])
		case r.Depression[i] != None:
			return ShallowColor
		}
		return LandRamp.At(alt[i] - level)
	})
	drawSpills(img, f, m, r)
	return img
}

// drawSpills draws every basin's spill edge and marks its spill corner.
func drawSpills(img *image.RGBA, f *field.Field, m *mesh.Mesh, r *Result) {
	spill := make([]bool, len(m.Edges))
	for _, b := range r.Basins {
		spill[b.SpillEdge] = true
	}
	w := max(2, float64(mesh.RenderScale(f, m)))
	mesh.DrawEdges(img, f, m, w, func(e int) (color.RGBA, bool) { return SpillInk, spill[e] })
	for _, b := range r.Basins {
		p := m.Corners[b.SpillCorner].Point
		mesh.Mark(img, f, m, p, 0.3, 3, ringInk)
		mesh.Mark(img, f, m, p, 0.2, 2, SpillInk)
	}
}

// The lakes render's palette: lakes by kind and salinity, playas, the land
// of basins that holds no water, and the marks.
var (
	FreshLakeColor = color.RGBA{0x3b, 0x82, 0xd6, 0xff}
	SaltLakeColor  = color.RGBA{0x4f, 0xc1, 0xa6, 0xff}
	FreshSeaColor  = color.RGBA{0x1c, 0x4c, 0x96, 0xff}
	SaltSeaColor   = color.RGBA{0x1f, 0x80, 0x70, 0xff}
	PlayaColor     = color.RGBA{0xf3, 0xd0, 0x5a, 0xff}
	DryBasinColor  = color.RGBA{0xe4, 0xd8, 0xc0, 0xff}
	SinkInk        = color.RGBA{0x7a, 0x3e, 0x0c, 0xff}
)

// LakeColor returns the lakes render's fill for lake k: FreshLakeColor or
// SaltLakeColor for a lake, FreshSeaColor or SaltSeaColor for an inland
// sea.
func LakeColor(k *Lake) color.RGBA {
	switch {
	case k.Kind == KindInlandSea && k.Salt:
		return SaltSeaColor
	case k.Kind == KindInlandSea:
		return FreshSeaColor
	case k.Salt:
		return SaltLakeColor
	}
	return FreshLakeColor
}

// LakesRender draws the basins stage's "lakes" variant over
// mesh.CellRender: lake cells by LakeColor, playa cells in PlayaColor,
// other basin cells in DryBasinColor, other land by LandRamp at its
// altitude above level, the sea in OceanColor and the rim as the ice sheet.
// It then marks each dry sink corner with a SinkInk disc in a white ring,
// draws the spill edge of every overflowing lake in SpillInk with its spill
// corner (the outlet) marked, and dots in SpillInk the path, cell by cell
// down the descent, by which a top-level basin's overflow reaches the basin
// downstream of it.
func LakesRender(f *field.Field, m *mesh.Mesh, alt []float64, seed []bool, level float64, r *Result, l *Lakes) *image.RGBA {
	playa := make([]bool, len(m.Cells))
	for _, p := range l.Playas {
		playa[p.Cell] = true
	}
	img := mesh.CellRender(f, m, func(i int) color.RGBA {
		switch {
		case m.Cells[i].Rim:
			return mesh.IceColor
		case seed[i]:
			return OceanColor
		case l.Lake[i] != None:
			return LakeColor(&l.Lakes[l.Lake[i]])
		case playa[i]:
			return PlayaColor
		case r.Of[i] != None:
			return DryBasinColor
		}
		return LandRamp.At(alt[i] - level)
	})
	spill := make([]bool, len(m.Edges))
	for _, k := range l.Lakes {
		if k.Outlet != None {
			spill[r.Basins[k.Basin].SpillEdge] = true
		}
	}
	w := max(2, float64(mesh.RenderScale(f, m)))
	mesh.DrawEdges(img, f, m, w, func(e int) (color.RGBA, bool) { return SpillInk, spill[e] })
	for _, x := range l.Water {
		if x.Overflow <= 0 || x.OverflowTo == None || x.OverflowVia == None {
			continue
		}
		for c := x.OverflowVia; c != None && !seed[c]; c = l.Down[c] {
			mesh.Mark(img, f, m, m.Cells[c].Site, 0.12, 1.5, SpillInk)
			if l.Lake[c] != None {
				break
			}
		}
	}
	for _, k := range l.Lakes {
		if k.Outlet != None {
			p := m.Corners[k.Outlet].Point
			mesh.Mark(img, f, m, p, 0.3, 3, ringInk)
			mesh.Mark(img, f, m, p, 0.2, 2, SpillInk)
		}
	}
	for _, p := range l.Playas {
		q := m.Corners[p.Corner].Point
		mesh.Mark(img, f, m, q, 0.3, 3, ringInk)
		mesh.Mark(img, f, m, q, 0.2, 2, SinkInk)
	}
	return img
}
