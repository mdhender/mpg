// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package river

import (
	"image"
	"image/color"

	"github.com/mdhender/mpg/internal/basin"
	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/mesh"
)

// The renders' inks: tree edges in TreeInk, edges across a flat or climbing
// out of a pit in FlatInk, lake outlets in OutletInk and dry sinks in
// basin.SinkInk, each in a white ring; land cells that drain to the sea in
// the catchments render in SeaCatchColor, cells whose destination differs
// from the cell-level model's dotted in DifferInk.
var (
	TreeInk       = color.RGBA{0x1e, 0x6b, 0x2a, 0xff}
	FlatInk       = color.RGBA{0xd0, 0x10, 0x10, 0xff}
	OutletInk     = color.RGBA{0xff, 0x80, 0x00, 0xff}
	SeaCatchColor = color.RGBA{0xee, 0xe8, 0xd8, 0xff}
	DifferInk     = color.RGBA{0x20, 0x20, 0x20, 0xff}
	ringInk       = color.RGBA{0xff, 0xff, 0xff, 0xff}
)

// lakeFill is a lake cell's fill: the lakes render's colors by kind.
func lakeFill(in Input, t *Tree, c int) color.RGBA {
	if t.Lakes[in.Lake[c]].Drain == Closed {
		return basin.SaltLakeColor
	}
	return basin.FreshLakeColor
}

// Render is the stage render: land by basin.LandRamp at its altitude above
// level, closed lakes in basin.SaltLakeColor and draining ones in
// basin.FreshLakeColor, the sea in basin.OceanColor and the rim as ice,
// with every tree edge drawn (in FlatInk where it crosses a flat or climbs
// out of a pit), each lake outlet and dry sink marked.
func Render(f *field.Field, in Input, t *Tree, level float64) *image.RGBA {
	m := in.Mesh
	img := mesh.CellRender(f, m, func(c int) color.RGBA {
		switch {
		case m.Cells[c].Rim:
			return mesh.IceColor
		case in.Land[c]:
			return basin.LandRamp.At(in.Altitude[c] - level)
		case in.Lake[c] != None:
			return lakeFill(in, t, c)
		}
		return basin.OceanColor
	})
	from := make([]int, len(m.Edges))
	for e := range from {
		from[e] = None
	}
	for k, e := range t.DownEdge {
		if e != None {
			from[e] = k
		}
	}
	w := max(1.5, float64(mesh.RenderScale(f, m))*0.75)
	mesh.DrawEdges(img, f, m, w, func(e int) (color.RGBA, bool) {
		k := from[e]
		if k == None {
			return color.RGBA{}, false
		}
		if d := t.Down[k]; t.Level[k] == t.Level[d] || t.Height[d] > t.Height[k] {
			return FlatInk, true
		}
		return TreeInk, true
	})
	marks(img, f, in, t)
	return img
}

// marks rings each lake outlet and dry sink.
func marks(img *image.RGBA, f *field.Field, in Input, t *Tree) {
	m := in.Mesh
	mark := func(k int, ink color.RGBA) {
		p := m.Corners[k].Point
		mesh.Mark(img, f, m, p, 0.3, 3, ringInk)
		mesh.Mark(img, f, m, p, 0.2, 2, ink)
	}
	for _, l := range t.Lakes {
		if l.Corner != None {
			mark(l.Corner, OutletInk)
		}
	}
	for _, k := range in.Sinks {
		mark(k, basin.SinkInk)
	}
}

// CatchmentRender is the "catchments" variant: each land cell filled by
// where its runoff ends (its lowest corner's Dest): SeaCatchColor for the
// sea, basin.Palette by lake id for a lake, basin.PlayaColor for a dry
// sink; water as in Render; with a dot on every land cell whose destination
// differs from the input's cell-level catchment, when it has one, and the
// outlets and sinks marked.
func CatchmentRender(f *field.Field, in Input, t *Tree) *image.RGBA {
	m := in.Mesh
	img := mesh.CellRender(f, m, func(c int) color.RGBA {
		switch {
		case m.Cells[c].Rim:
			return mesh.IceColor
		case in.Land[c]:
			d := t.Dest(t.Lowest(m, c))
			switch d.Kind {
			case Lake:
				return basin.Palette[d.ID%len(basin.Palette)]
			case Sink:
				return basin.PlayaColor
			}
			return SeaCatchColor
		case in.Lake[c] != None:
			return lakeFill(in, t, c)
		}
		return basin.OceanColor
	})
	if in.Catchment != nil {
		for c, land := range in.Land {
			if land && t.Dest(t.Lowest(m, c)) != in.Catchment[c] {
				mesh.Mark(img, f, m, m.Cells[c].Site, 0.15, 1.5, DifferInk)
			}
		}
	}
	marks(img, f, in, t)
	return img
}
