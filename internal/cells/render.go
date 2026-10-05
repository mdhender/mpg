// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package cells

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"

	"github.com/mdhender/mpg/internal/field"
	"github.com/mdhender/mpg/internal/fmath"
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

// The search trace render's inks.
var (
	traceBackground = color.RGBA{0xff, 0xff, 0xff, 0xff}
	traceAxis       = color.RGBA{0x40, 0x40, 0x40, 0xff}
	traceBand       = color.RGBA{0xcf, 0xec, 0xc4, 0xff}
	traceTarget     = color.RGBA{0x2e, 0x7d, 0x32, 0xff}
	traceLine       = color.RGBA{0x90, 0x90, 0x90, 0xff}
	traceText       = color.RGBA{0x20, 0x20, 0x20, 0xff}
	traceBest       = color.RGBA{0xd3, 0x2f, 0x2f, 0xff}
	// TraceMethodColors colors a probe's point by its method, in Method
	// order: estimate, newton, bisect, gallop.
	TraceMethodColors = [...]color.RGBA{
		{0x1f, 0x4e, 0x9c, 0xff},
		{0x00, 0x89, 0x7b, 0xff},
		{0xef, 0x8f, 0x00, 0xff},
		{0x8e, 0x24, 0xaa, 0xff},
	}
)

// TraceRender draws a sea-level search's trace as a chart, width × height
// pixels: probe number (1, 2, …) across, land cells up, the tolerance band
// N ± Tolerance shaded green with N as a line, and the probes joined in
// order, each a disc colored by its method (TraceMethodColors) and labeled
// with its level in meters and, when the search measured lakes, its lake
// cells. The best probe is ringed in red. The title gives N, the result,
// its deviation, the reason, and the probe count. Probes outside the band
// are drawn at their counts; the vertical range covers every probe and
// twice the band.
func TraceRender(r *Record, width, height int) *image.RGBA {
	width, height = max(width, 160), max(height, 120)
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Rect, image.NewUniform(traceBackground), image.Point{}, draw.Src)
	const left, right, top, bottom = 72, 24, 40, 30
	pw, ph := float64(width-left-right), float64(height-top-bottom)
	lo, hi := r.Target-2*r.Tolerance-1, r.Target+2*r.Tolerance+1
	lakes := false
	for _, p := range r.Trace {
		lo, hi = min(lo, p.Land), max(hi, p.Land)
		lakes = lakes || p.Lake > 0
	}
	pad := max((hi-lo)/20, 1)
	lo, hi = lo-pad, hi+pad
	n := len(r.Trace)
	x := func(k int) float64 { // probe k (0-based)
		if n == 1 {
			return float64(left) + float64(pw/2)
		}
		return float64(left) + fmath.Mul(pw, float64(k))/float64(n-1)
	}
	y := func(v int) float64 { return float64(top) + fmath.Mul(ph, float64(hi-v))/float64(hi-lo) }
	// The tolerance band and the target.
	render.FillPolygon(img, []render.Pt{
		{X: float64(left), Y: y(r.Target + r.Tolerance)}, {X: float64(width - right), Y: y(r.Target + r.Tolerance)},
		{X: float64(width - right), Y: y(r.Target - r.Tolerance)}, {X: float64(left), Y: y(r.Target - r.Tolerance)},
	}, traceBand)
	render.Line(img, render.Pt{X: float64(left), Y: y(r.Target)}, render.Pt{X: float64(width - right), Y: y(r.Target)}, 1.5, traceTarget)
	// Axes.
	render.Line(img, render.Pt{X: float64(left), Y: float64(top)}, render.Pt{X: float64(left), Y: float64(height - bottom)}, 1, traceAxis)
	render.Line(img, render.Pt{X: float64(left), Y: float64(height - bottom)}, render.Pt{X: float64(width - right), Y: float64(height - bottom)}, 1, traceAxis)
	label := func(px, py float64, s string, c color.RGBA) {
		x0, y0 := int(px), int(py)
		render.Label(img, image.Rect(x0, y0, x0+render.LabelAdvance*len(s)+1, y0+render.LabelHeight), s, c)
	}
	for _, v := range []int{hi, r.Target, lo} {
		s := fmt.Sprintf("%d", v)
		label(float64(left-6-render.LabelAdvance*len(s)), y(v)-render.LabelHeight/2, s, traceText)
	}
	// The probes.
	for k := 1; k < n; k++ {
		render.Line(img, render.Pt{X: x(k - 1), Y: y(r.Trace[k-1].Land)}, render.Pt{X: x(k), Y: y(r.Trace[k].Land)}, 1.5, traceLine)
	}
	for k, p := range r.Trace {
		c := traceAxis
		if int(p.Method) < len(TraceMethodColors) {
			c = TraceMethodColors[p.Method]
		}
		if k == r.Best {
			render.Disc(img, render.Pt{X: x(k), Y: y(p.Land)}, 8, traceBest)
			render.Disc(img, render.Pt{X: x(k), Y: y(p.Land)}, 6, traceBackground)
		}
		render.Disc(img, render.Pt{X: x(k), Y: y(p.Land)}, 4.5, c)
		s := fmt.Sprintf("%.1f m", p.Level)
		if lakes {
			s += fmt.Sprintf(", %d lake", p.Lake)
		}
		tx := x(k) + 8
		if tx+float64(render.LabelAdvance*len(s)) > float64(width) {
			tx = x(k) - 8 - float64(render.LabelAdvance*len(s))
		}
		ty := y(p.Land) - render.LabelHeight - 4
		if k%2 == 1 {
			ty = y(p.Land) + 4
		}
		label(tx, ty, s, traceText)
		ns := fmt.Sprintf("%d", k+1)
		label(x(k)-float64(render.LabelAdvance*len(ns)/2), float64(height-bottom+6), ns, traceText)
	}
	best := r.Result()
	title := fmt.Sprintf("N %d +/- %d: land %d (%+d) %s, %d probes", r.Target, r.Tolerance, best.Land, best.Land-r.Target, r.Reason, n)
	if lakes {
		title += fmt.Sprintf(", %d lake cells", best.Lake)
	}
	label(float64(left), 12, title, traceText)
	return img
}
