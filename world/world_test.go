// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package world_test

import (
	"encoding/json"
	"math"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/mdhender/mpg/world"
)

// TestPointJSON checks that a point writes as [x, y], formatted exactly as
// encoding/json formats float64, and reads back only from two numbers.
func TestPointJSON(t *testing.T) {
	for _, v := range []float64{0, 1, -1, 0.5, 2536.312345, -3.000001, 1e-6, 9.99e-7, 1e-7, -2.5e-9, 1e20, 1e21, 1.5e300, math.SmallestNonzeroFloat64, math.MaxFloat64} {
		got, err := json.Marshal(world.Point{X: v, Y: -v})
		if err != nil {
			t.Fatal(err)
		}
		want, _ := json.Marshal([]float64{v, -v})
		if string(got) != string(want) {
			t.Errorf("Point{%v} = %s, want %s", v, got, want)
		}
		var p world.Point
		if err := json.Unmarshal(got, &p); err != nil || p.X != v || p.Y != -v {
			t.Errorf("round trip of %s = %v, %v", got, p, err)
		}
	}
	for _, v := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := json.Marshal(world.Point{X: v}); err == nil {
			t.Errorf("Point{%v} marshaled", v)
		}
	}
	for _, in := range []string{`[]`, `[1]`, `[1,2,3]`, `{"x":1,"y":2}`, `"1,2"`, `[1,"2"]`, `null`} {
		var p world.Point
		if err := json.Unmarshal([]byte(in), &p); err == nil && in != "null" {
			t.Errorf("Unmarshal(%s) succeeded: %v", in, p)
		}
	}
}

// minimal is the smallest world document Decode accepts (it does not
// validate).
const minimal = `{"schema":1,"meta":{},"codebooks":{},"outcomes":{},"cells":[],"corners":[],"edges":[],"coastlines":[],"rivers":[]}`

func TestDecode(t *testing.T) {
	w, err := world.DecodeBytes([]byte(minimal + "\n"))
	if err != nil || w.Schema != world.SchemaVersion {
		t.Fatalf("Decode(minimal) = %v, %v", w, err)
	}
	for _, tc := range []struct{ in, msg string }{
		{strings.Replace(minimal, `"schema":1,`, ``, 1), "no schema"},
		{strings.Replace(minimal, `"schema":1`, `"schema":2`, 1), "schema 2 is newer than this reader (schema 1); update github.com/mdhender/mpg/world"},
		{strings.Replace(minimal, `"schema":1`, `"schema":-1`, 1), "schema -1 is not a version"},
		{strings.Replace(minimal, `"schema":1`, `"schema":"1"`, 1), "world:"},
		{strings.Replace(minimal, `"cells":[]`, `"cells":[],"extra":1`, 1), "unknown field"},
		{strings.Replace(minimal, `"meta":{}`, `"meta":{"widht_km":1}`, 1), "unknown field"},
		{minimal + `{}`, "after the top-level object"},
		{minimal[:20], "world:"},
		{``, "world:"},
	} {
		if _, err := world.DecodeBytes([]byte(tc.in)); err == nil || !strings.Contains(err.Error(), tc.msg) {
			t.Errorf("Decode(%q) = %v, want an error mentioning %q", tc.in, err, tc.msg)
		}
	}
}

// TestDecodeVersion0 checks that a pre-release (schema 0) world is rejected
// with a request to generate it again, also when its layout (here the
// outcomes' deferred list, and an edge's biome from an early version 0)
// would fail first, and that a newer schema's unknown fields do not hide
// its version either.
func TestDecodeVersion0(t *testing.T) {
	const want0 = "world: schema 0 is the pre-release layout, with no migration; regenerate the world with this mpg"
	v0 := strings.Replace(minimal, `"schema":1`, `"schema":0`, 1)
	for _, in := range []string{
		v0,
		strings.Replace(v0, `"outcomes":{}`, `"outcomes":{"deferred":[]}`, 1),
		strings.Replace(v0, `"edges":[]`, `"edges":[{"id":0,"biome":"desert"}]`, 1),
	} {
		if _, err := world.DecodeBytes([]byte(in)); err == nil || err.Error() != want0 {
			t.Errorf("Decode(%s) = %v, want %q", in, err, want0)
		}
	}
	v2 := strings.Replace(minimal, `"schema":1,`, `"schema":2,"new_field":1,`, 1)
	if _, err := world.DecodeBytes([]byte(v2)); err == nil || !strings.Contains(err.Error(), "schema 2 is newer") {
		t.Errorf("Decode(%s) = %v, want the newer-schema error", v2, err)
	}
	// The removed fields are unknown in version 1.
	for _, in := range []string{
		strings.Replace(minimal, `"outcomes":{}`, `"outcomes":{"deferred":[]}`, 1),
		strings.Replace(minimal, `"edges":[]`, `"edges":[{"id":0,"surface":"marshes"}]`, 1),
	} {
		if _, err := world.DecodeBytes([]byte(in)); err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Errorf("Decode(%s) = %v, want an unknown field", in, err)
		}
	}
}

// TestBytes checks the canonical encoding: compact, a trailing newline, the
// schema first, empty lists written as [], and a decode round trip.
func TestBytes(t *testing.T) {
	w := &world.World{Schema: world.SchemaVersion, Codebooks: world.DefaultCodebooks(), Coastlines: []world.Coastline{}, Rivers: []world.RiverPath{},
		Cells: []world.Cell{{Landform: world.Plains, Site: world.Point{X: 1.5, Y: 2}, Exits: []world.Exit{}}}}
	b, err := w.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.HasPrefix(s, `{"schema":1,"meta":{`) || !strings.HasSuffix(s, "}\n") || strings.Count(s, "\n") != 1 {
		t.Errorf("Bytes = %s", s)
	}
	for _, sub := range []string{`"site":[1.5,2]`, `"coastlines":[]`, `"rivers":[]`, `"landform":"plains"`} {
		if !strings.Contains(s, sub) {
			t.Errorf("Bytes lacks %s: %s", sub, s)
		}
	}
	for _, absent := range []string{`"depth"`, `"water"`, `"flags"`, `"deferred"`} {
		if strings.Contains(s, absent) {
			t.Errorf("Bytes has empty %s: %s", absent, s)
		}
	}
	back, err := world.DecodeBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := back.Bytes(); string(again) != s {
		t.Errorf("round trip changed the bytes:\n%s\n%s", s, again)
	}
	if _, err := (&world.World{Cells: []world.Cell{{AltitudeM: math.Inf(1)}}}).Bytes(); err == nil {
		t.Error("Bytes encoded an infinite altitude")
	}
}

func TestCodebooks(t *testing.T) {
	for k, d := range world.Directions {
		if d.Index() != k {
			t.Errorf("%s.Index() = %d, want %d", d, d.Index(), k)
		}
	}
	if world.Direction("NNE").Index() != -1 {
		t.Error("NNE is a compass point")
	}
	for _, l := range world.Landforms {
		if l.IsLand() == l.IsWater() {
			t.Errorf("%s: land %v, water %v", l, l.IsLand(), l.IsWater())
		}
	}
	if world.Landform("swamp").IsLand() || world.Landform("").IsWater() {
		t.Error("an unknown landform is land or water")
	}
	cb := world.DefaultCodebooks()
	cb.Landforms[0] = "changed"
	if world.Landforms[0] != world.SaltWater {
		t.Error("DefaultCodebooks shares the package's slices")
	}
}

func TestValidateEmpty(t *testing.T) {
	err := world.Validate(&world.World{})
	inv, ok := err.(*world.Invalid)
	if !ok || inv.Count == 0 || len(inv.Problems) == 0 {
		t.Fatalf("Validate(empty) = %v", err)
	}
	for _, s := range []string{"meta.generator", "codebooks", "meta size"} {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("Validate(empty) lacks %q: %v", s, err)
		}
	}
}

// TestNoFusedMultiplyAdd compiles this package for the architectures whose Go
// compilers fuse a*b+c into one instruction and fails if any fused
// multiply-add or multiply-subtract appears in its code.
func TestNoFusedMultiplyAdd(t *testing.T) {
	if testing.Short() {
		t.Skip("compiles for several architectures")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool not found")
	}
	fusedOp := regexp.MustCompile(`\tV?FN?M(ADD|SUB)`)
	const pkg = "github.com/mdhender/mpg/world"
	for _, tg := range []struct{ arch, env string }{{"arm64", ""}, {"amd64", "GOAMD64=v3"}} {
		t.Run(tg.arch, func(t *testing.T) {
			cmd := exec.Command(goTool, "build", "-gcflags="+pkg+"=-S", pkg)
			cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+tg.arch, "CGO_ENABLED=0")
			if tg.env != "" {
				cmd.Env = append(cmd.Env, tg.env)
			}
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("go build %s: %v\n%s", pkg, err, out)
			}
			if m := fusedOp.FindAllString(string(out), -1); len(m) != 0 {
				t.Errorf("%s: %d fused multiply-add instructions in %s", tg.arch, len(m), pkg)
			}
		})
	}
}
