// Package voronoi computes Voronoi diagrams with Fortune's sweep-line
// algorithm, clipped to a bounding box.
//
// # Provenance
//
// This is a vendored copy of github.com/pzsz/voronoi at commit 4314be88c79f,
// by Przemyslaw Szczepaniak, itself a port of Raymond Hill's
// Javascript-Voronoi (github.com/gorhill/Javascript-Voronoi). It came to mpg
// through github.com/mdhender/wgvc (internal/voronoi), which had already made
// its arithmetic safe from fused multiply-add. It is kept under the original
// MIT license, in LICENSE.md beside this file. The unused utils subpackage is
// not included.
//
// # Changes from upstream
//
//   - Floating-point arithmetic (from wgvc): every product that feeds an
//     addition or subtraction is wrapped in an explicit float64 conversion,
//     and the angle uses fmath.Atan2. The Go specification lets a compiler
//     fuse x*y + z into one multiply-add with a single rounding; arm64 does
//     and amd64 (below GOAMD64=v3) does not, so upstream produced vertices
//     whose low-order bits differed between architectures. The conversions
//     force each product to round on its own, which makes the diagram
//     bit-identical everywhere. TestNoFusedMultiplyAdd checks the compiled
//     code.
//   - Ordering (mpg): ComputeDiagram no longer sorts the caller's slice. It
//     orders site events by y, then x, then input index, a total order
//     (upstream sorted by y alone with an unstable sort), and each Cell
//     records its site's input Index. Half-edges are sorted by angle with a
//     stable sort. The result therefore does not depend on the sort
//     algorithm of the Go release.
//   - No map iteration (mpg): the pass that linked each vertex to its edges
//     through a map keyed by vertex is removed, with EdgeVertex.Edges.
//   - Errors (mpg): ComputeDiagram returns an error where upstream would loop
//     forever, when closeCells meets an open cell end that is not on the
//     bounding box.
//
// Package mesh is the only user: it runs the sweep over the cylinder's sites
// and their ghost copies and builds its own graph from the result.
package voronoi
