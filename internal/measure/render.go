// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package measure

import (
	"image"
	"image/color"

	"github.com/mdhender/mpg/internal/config"
	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/world"
)

// Map is what the landmass and chokepoint render draws: the cell graph,
// the landmasses and their classes, the mountain chains, and the straits,
// necks and passes. Analyze returns it.
type Map struct {
	g      *graph
	lm     landmasses
	cm     chokeMap
	inland []bool
	rim    []bool
	class  config.Landmass
}

// The landmass and chokepoint render's palette. Landmasses take a color by
// class, alternating among three shades by landmass id so neighbors
// usually differ: earthy shades for continents, lighter ones for islands,
// and a dark gray for islets. Mountain chains are darkened to ChainShade
// of their landmass's color (other mountain cells to MountainShade). The
// chokepoints are drawn over them: strait water in StraitInk (StraitWithinInk
// for a strait within one landmass), neck cells in NeckInk, pass cells in
// PassInk. The ocean is a pale blue, lakes and inland seas a deeper blue,
// and the rim the ice sheet.
var (
	ContinentColors = []color.RGBA{{0xc8, 0xc0, 0x8c, 0xff}, {0x9c, 0xc8, 0xa8, 0xff}, {0xd0, 0xa8, 0x8c, 0xff}}
	IslandColors    = []color.RGBA{{0xec, 0xd6, 0x9c, 0xff}, {0xbc, 0xdc, 0x96, 0xff}, {0xdc, 0xc0, 0xdc, 0xff}}
	IsletColor      = color.RGBA{0x50, 0x50, 0x50, 0xff}
	OceanColor      = color.RGBA{0xb0, 0xcc, 0xe8, 0xff}
	InlandColor     = color.RGBA{0x60, 0x90, 0xd0, 0xff}
	StraitInk       = color.RGBA{0xd0, 0x10, 0x10, 0xff}
	StraitWithinInk = color.RGBA{0x10, 0x90, 0x10, 0xff}
	NeckInk         = color.RGBA{0xe0, 0x00, 0xa0, 0xff}
	PassInk         = color.RGBA{0xff, 0x80, 0x00, 0xff}
)

// ChainShade and MountainShade are the brightness kept by a chain cell and
// by a mountain cell outside every chain.
const (
	ChainShade    = 0.55
	MountainShade = 0.8
)

// Render draws the measures stage render, the landmass and chokepoint map,
// over mesh.CellRender at f's size: every cell by the palette above.
func (mp *Map) Render(f *field.Field, m *mesh.Mesh) *image.RGBA {
	const (
		none = iota
		strait
		within
		neck
		pass
	)
	mark := make([]uint8, mp.g.cells())
	for _, c := range mp.cm.straits {
		for _, x := range c.cells {
			if c.a != c.b && mark[x] != strait {
				mark[x] = strait
			} else if mark[x] == none {
				mark[x] = within
			}
		}
	}
	for _, c := range mp.cm.necks {
		for _, x := range c.cells {
			mark[x] = neck
		}
	}
	for _, c := range mp.cm.passes {
		for _, x := range c.cells {
			mark[x] = pass
		}
	}
	return mesh.CellRender(f, m, func(i int) color.RGBA {
		switch {
		case mp.rim[i]:
			return mesh.IceColor
		case mark[i] == strait:
			return StraitInk
		case mark[i] == within:
			return StraitWithinInk
		case mark[i] == neck:
			return NeckInk
		case mark[i] == pass:
			return PassInk
		case !mp.g.land[i] && mp.inland[i]:
			return InlandColor
		case !mp.g.land[i]:
			return OceanColor
		}
		id := mp.lm.id[i]
		var c color.RGBA
		switch landmassClass(mp.lm.size[id], mp.class) {
		case world.ClassContinent:
			c = ContinentColors[id%len(ContinentColors)]
		case world.ClassIsland:
			c = IslandColors[id%len(IslandColors)]
		default:
			c = IsletColor
		}
		switch {
		case mp.cm.chain[i] >= 0:
			return shade(c, ChainShade)
		case mp.g.mountain[i]:
			return shade(c, MountainShade)
		}
		return c
	})
}

// shade scales c's channels by s in [0, 1], rounding down.
func shade(c color.RGBA, s float64) color.RGBA {
	return color.RGBA{uint8(float64(c.R) * s), uint8(float64(c.G) * s), uint8(float64(c.B) * s), c.A}
}
