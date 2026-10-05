// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package cells

import (
	"image"
	"image/color"

	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/mesh"
	"github.com/mdhender/mpg/internal/render"
)

// ReliefRamp colors a cell by its relief in meters: render.Viridis's five
// colors placed at 0, 75, 200, 500 and 1,200 m, a roughly logarithmic
// spacing, so the low relief of most land still reads beside the steep
// continental slopes. The scale is fixed, so sweep tiles compare.
var ReliefRamp = viridisAt(0, 75, 200, 500, 1200)

// viridisAt returns render.Viridis with its stops moved to values.
func viridisAt(values ...float64) render.Ramp {
	stops := render.Viridis.Stops()
	for k := range stops {
		stops[k].Value = values[k]
	}
	return render.NewRamp(stops...)
}

// AltitudeColor returns the hypsometric color of a cell at altitude meters
// against seaLevel, as render.Hypsometric colors a sample: render.Land at
// its height above sea level when strictly above it, render.Water at its
// depth below otherwise.
func AltitudeColor(altitude, seaLevel float64) color.RGBA {
	if altitude > seaLevel {
		return render.Land.At(altitude - seaLevel)
	}
	return render.Water.At(seaLevel - altitude)
}

// AltitudeRender draws the cell statistics stage render: every playable
// cell of m filled with AltitudeColor of its altitude against seaLevel,
// the rim cells as the ice sheet, with blended outlines and the ice front
// (mesh.CellRender), at the mesh render's size over f.
func AltitudeRender(f *field.Field, m *mesh.Mesh, s *Stats, seaLevel float64) *image.RGBA {
	return mesh.CellRender(f, m, func(i int) color.RGBA {
		if m.Cells[i].Rim {
			return mesh.IceColor
		}
		return AltitudeColor(s.Altitude[i], seaLevel)
	})
}

// ReliefRender draws the cell statistics stage's relief render: every
// playable cell filled by its relief on ReliefRamp, the rim as the ice
// sheet, otherwise as AltitudeRender.
func ReliefRender(f *field.Field, m *mesh.Mesh, s *Stats) *image.RGBA {
	return mesh.CellRender(f, m, func(i int) color.RGBA {
		if m.Cells[i].Rim {
			return mesh.IceColor
		}
		return ReliefRamp.At(s.Relief[i])
	})
}

// BasinColor fills a dry basin floor in the sea level render: a playable
// cell at or below sea level that the ocean does not reach. It is land
// until the basin stage decides it, and the orange keeps it apart from
// both the green lowlands and the blue sea.
var BasinColor = color.RGBA{0xe0, 0x70, 0x20, 0xff}

// SeaLevelRender draws the sea level stage render: land cells by
// AltitudeColor against the sea level (render.Land at their height above
// it), ocean cells by render.Water at their depth below it, dry basin
// floors in BasinColor, and the rim as the ice sheet, over mesh.CellRender.
func SeaLevelRender(f *field.Field, m *mesh.Mesh, s *Stats, sl *SeaLevel) *image.RGBA {
	return mesh.CellRender(f, m, func(i int) color.RGBA {
		switch {
		case m.Cells[i].Rim:
			return mesh.IceColor
		case sl.Basin[i]:
			return BasinColor
		}
		return AltitudeColor(s.Altitude[i], sl.Level)
	})
}
