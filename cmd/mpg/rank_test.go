// Copyright (c) 2026 Michael D Henderson. All rights reserved.

package main

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mdhender/mpg/internal/render"
)

func TestParseRankSpec(t *testing.T) {
	keys, err := parseRankSpec("usability.habitable_share*2, chokepoints.necks:min,rivers.touch_share:~0.25*0.5,land.met:max")
	if err != nil {
		t.Fatal(err)
	}
	want := []rankKey{
		{measure: "usability.habitable_share", dir: "max", weight: 2},
		{measure: "chokepoints.necks", dir: "min", weight: 1},
		{measure: "rivers.touch_share", dir: "~", target: 0.25, weight: 0.5},
		{measure: "land.met", dir: "max", weight: 1},
	}
	if !slices.Equal(keys, want) {
		t.Errorf("keys = %+v, want %+v", keys, want)
	}
	if got, w := rankSpecString(keys), "usability.habitable_share*2,chokepoints.necks:min,rivers.touch_share:~0.25*0.5,land.met"; got != w {
		t.Errorf("spec = %q, want %q", got, w)
	}

	def, err := parseRankSpec("default")
	if err != nil {
		t.Fatal(err)
	}
	if got := rankSpecString(def); got != defaultRankSpec {
		t.Errorf("default = %q, want %q", got, defaultRankSpec)
	}

	for _, bad := range []string{
		"",
		"usability.habitable_share,",
		"no.such_measure",
		"usability",
		"land.met,land.met:min",
		"land.met:up",
		"land.met:~",
		"land.met:~x",
		"land.met:~NaN",
		"land.met:~Inf",
		"land.met*0",
		"land.met*-1",
		"land.met*1001",
		"land.met*Inf",
		"land.met*",
		"landmasses.list",
		strings.Repeat("land.met,", 33),
	} {
		if _, err := parseRankSpec(bad); err == nil {
			t.Errorf("parseRankSpec(%q) succeeded", bad)
		}
	}
}

func TestPercentiles(t *testing.T) {
	vals := []float64{3, 1, 3, 2, 3}
	members := []int{0, 1, 2, 3, 4}
	got := percentiles(members, func(i int) float64 { return vals[i] })
	// 1 beats none; 2 beats one; the three 3s beat two and tie two: (2+1)/4.
	want := []float64{0.75, 0, 0.75, 0.25, 0.75}
	if !slices.Equal(got, want) {
		t.Errorf("percentiles = %v, want %v", got, want)
	}
	if got := percentiles([]int{7}, func(int) float64 { return 9 }); !slices.Equal(got, []float64{0.5}) {
		t.Errorf("single = %v, want [0.5]", got)
	}
	if got := percentiles([]int{0, 1}, func(int) float64 { return 1 }); !slices.Equal(got, []float64{0.5, 0.5}) {
		t.Errorf("all tied = %v, want [0.5 0.5]", got)
	}
}

func TestRankItems(t *testing.T) {
	maxKey := rankKey{measure: "a", dir: "max", weight: 1}
	item := func(group int, pass bool, v ...float64) rankItem {
		return rankItem{group: group, values: v, pass: pass}
	}

	t.Run("directions and targets", func(t *testing.T) {
		items := []rankItem{item(0, true, 1), item(0, true, 5), item(0, true, 3)}
		for _, tc := range []struct {
			key   rankKey
			order []int
		}{
			{maxKey, []int{1, 2, 0}},
			{rankKey{measure: "a", dir: "min", weight: 1}, []int{0, 2, 1}},
			{rankKey{measure: "a", dir: "~", target: 2.9, weight: 1}, []int{2, 0, 1}},
			{rankKey{measure: "a", dir: "~", target: 4.5, weight: 1}, []int{1, 2, 0}},
		} {
			res := rankItems(items, []rankKey{tc.key})
			if !slices.Equal(res.order, tc.order) {
				t.Errorf("%s: order %v, want %v", tc.key, res.order, tc.order)
			}
			if res.score[res.order[0]] != 100 || res.score[res.order[2]] != 0 {
				t.Errorf("%s: scores %v, want best 100 and worst 0", tc.key, res.score)
			}
		}
	})

	t.Run("weights", func(t *testing.T) {
		// Item 0 wins key a, item 1 key b; the heavier key decides.
		items := []rankItem{item(0, true, 2, 1), item(0, true, 1, 2)}
		b := rankKey{measure: "b", dir: "max", weight: 1}
		heavyA := rankItems(items, []rankKey{{measure: "a", dir: "max", weight: 3}, b})
		if !slices.Equal(heavyA.order, []int{0, 1}) || heavyA.score[0] != 75 || heavyA.score[1] != 25 {
			t.Errorf("a*3: order %v scores %v", heavyA.order, heavyA.score)
		}
		heavyB := rankItems(items, []rankKey{maxKey, {measure: "b", dir: "max", weight: 3}})
		if !slices.Equal(heavyB.order, []int{1, 0}) {
			t.Errorf("b*3: order %v", heavyB.order)
		}
		even := rankItems(items, []rankKey{maxKey, b})
		if !slices.Equal(even.order, []int{0, 1}) || even.score[0] != 50 || even.score[1] != 50 {
			t.Errorf("even: order %v scores %v, want a tie kept in input order", even.order, even.score)
		}
	})

	t.Run("gates last", func(t *testing.T) {
		items := []rankItem{item(0, true, 1), item(0, false, 9), item(0, true, 2), item(0, false, 8)}
		res := rankItems(items, []rankKey{maxKey})
		if !slices.Equal(res.order, []int{2, 0, 1, 3}) || !slices.Equal(res.rank, []int{2, 3, 1, 4}) {
			t.Errorf("order %v ranks %v", res.order, res.rank)
		}
	})

	t.Run("scope", func(t *testing.T) {
		// Two blocks of two; group scope keeps blocks in first-appearance
		// order and ranks within each.
		items := []rankItem{item(5, true, 1), item(5, true, 2), item(3, true, 10), item(3, true, 20)}
		res := rankItems(items, []rankKey{maxKey})
		if !slices.Equal(res.order, []int{1, 0, 3, 2}) || !slices.Equal(res.rank, []int{2, 1, 2, 1}) {
			t.Errorf("group: order %v ranks %v", res.order, res.rank)
		}
		for i := range items {
			items[i].group = 0
		}
		res = rankItems(items, []rankKey{maxKey})
		if !slices.Equal(res.order, []int{3, 2, 1, 0}) || !slices.Equal(res.rank, []int{4, 3, 2, 1}) {
			t.Errorf("all: order %v ranks %v", res.order, res.rank)
		}
	})

	t.Run("ties and single rows", func(t *testing.T) {
		items := []rankItem{item(0, true, 4), item(0, true, 4), item(1, true, 7), item(0, true, 4)}
		res := rankItems(items, []rankKey{maxKey})
		if !slices.Equal(res.order, []int{0, 1, 3, 2}) {
			t.Errorf("order %v, want ties in input order", res.order)
		}
		for i, s := range res.score {
			if s != 50 || math.IsNaN(s) {
				t.Errorf("score[%d] = %v, want 50", i, s)
			}
		}
	})
}

func TestRankTable(t *testing.T) {
	keys := []rankKey{{measure: "usability.habitable_share", dir: "max", weight: 2}, {measure: "chokepoints.necks", dir: "min", weight: 1}}
	got := rankTable(keys, rankScopeGroup, "abc", []rankRow{
		{seed: 4, aspect: "square", preset: "continents", rank: 1, score: 75, pass: true, landMet: true, values: []float64{0.8987, 11}},
		{seed: 3, aspect: "square", preset: "continents", rank: 2, score: 18.75, pass: false, landMet: false, values: []float64{0.83, -0.0}},
	})
	want := "# mpg sweep ranking\n\n" +
		"- rank: `usability.habitable_share*2,chokepoints.necks:min`\n- scope: group\n- config: `abc`\n\n" +
		"| rank | seed | aspect | preset | score | pass | land met | usability.habitable_share*2 | chokepoints.necks:min |\n" +
		"| ---: | ---: | --- | --- | ---: | --- | --- | ---: | ---: |\n" +
		"| 1 | 4 | square | continents | 75.0 | yes | yes | 0.8987 | 11 |\n" +
		"| 2 | 3 | square | continents | 18.8 | no | no | 0.8300 | 0 |\n"
	if got != want {
		t.Errorf("table:\n%s\nwant:\n%s", got, want)
	}
}

// TestSweepRank is S36's done-when: a ranked sweep writes the ranked sheet
// and the rank table, rows reordered by rank within each block.
func TestSweepRank(t *testing.T) {
	dir, cfg := t.TempDir(), smallConfig(t, 1, nil)
	out := filepath.Join(dir, "ranked.png")
	// Rank by fewest landmasses in each block (land cells barely count). A layout column only: --rank
	// runs the pipeline through the measures stage anyway.
	code, stdout, stderr := sweep(t, "--seeds", "1-3", "--stage", "layout", "--aspect", "square,cinematic",
		"--config", cfg, "--tile", "32", "--rank", "landmasses.count:min*100,land.cells", "--output", out)
	if code != 0 {
		t.Fatalf("exit %d; stderr %q", code, stderr)
	}
	table := filepath.Join(dir, "ranked.rank.md")
	if !strings.Contains(stdout, "table   "+table+"\n") || !strings.Contains(stdout, "rank    landmasses.count:min*100,land.cells (scope group)\n") {
		t.Errorf("stdout %q", stdout)
	}
	b, err := os.ReadFile(table)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s", b)
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	if lines[6] != "| rank | seed | aspect | preset | score | pass | land met | landmasses.count:min*100 | land.cells |" {
		t.Errorf("header %q", lines[6])
	}
	rows := lines[8:]
	if len(rows) != 6 {
		t.Fatalf("table has %d rows, want 6:\n%s", len(rows), b)
	}
	type trow struct {
		rank, seed, aspect, count string
	}
	var got []trow
	for _, r := range rows {
		f := strings.Split(strings.Trim(r, "| "), " | ")
		got = append(got, trow{f[0], f[1], f[2], f[7]})
	}
	// Each block holds ranks 1-3 in order, its own seeds, and landmass
	// counts that do not decrease down the block.
	for blk, aspect := range []string{"square", "cinematic"} {
		var seeds []string
		prev := -1
		for k, r := range got[3*blk : 3*blk+3] {
			if r.aspect != aspect || r.rank != strconv.Itoa(k+1) {
				t.Errorf("block %s row %d = %+v", aspect, k, r)
			}
			n, err := strconv.Atoi(r.count)
			if err != nil || n < prev {
				t.Errorf("block %s row %d: landmasses %q after %d", aspect, k, r.count, prev)
			}
			prev = n
			seeds = append(seeds, r.seed)
		}
		slices.Sort(seeds)
		if !slices.Equal(seeds, []string{"1", "2", "3"}) {
			t.Errorf("block %s seeds %v", aspect, seeds)
		}
	}

	// The sheet's metadata records the spec and the rows in ranked order,
	// matching the table.
	png, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := render.ReadMeta(bytes.NewReader(png))
	if err != nil {
		t.Fatal(err)
	}
	extra := map[string]string{}
	for _, e := range meta.Extra {
		extra[e.Key] = e.Value
	}
	if extra["mpg:rank"] != "landmasses.count:min*100,land.cells scope group" {
		t.Errorf("mpg:rank = %q", extra["mpg:rank"])
	}
	metaRows := strings.Split(extra["mpg:rows"], "\n")
	for k, r := range got {
		if !strings.HasPrefix(metaRows[k], r.seed+" "+r.aspect+" ") {
			t.Errorf("mpg:rows[%d] = %q, table row %+v", k, metaRows[k], r)
		}
	}

	// Twice gives the same table; --table moves it.
	other := filepath.Join(dir, "sub", "t.md")
	code, _, stderr = sweep(t, "--seeds", "1-3", "--stage", "layout", "--aspect", "square,cinematic",
		"--config", cfg, "--tile", "32", "--rank", "landmasses.count:min*100,land.cells", "--output", filepath.Join(dir, "again.png"), "--table", other)
	if code != 0 {
		t.Fatalf("exit %d; stderr %q", code, stderr)
	}
	if b2, err := os.ReadFile(other); err != nil || !bytes.Equal(b, b2) {
		t.Errorf("second table differs (err %v)", err)
	}
}

// TestSweepRankScopeAll checks that scope all ranks across blocks.
func TestSweepRankScopeAll(t *testing.T) {
	dir, cfg := t.TempDir(), smallConfig(t, 1, nil)
	code, _, stderr := sweep(t, "--seeds", "1,2", "--stage", "layout", "--aspect", "square,cinematic",
		"--config", cfg, "--tile", "32", "--rank", "landmasses.count", "--rank-scope", "all", "--output", filepath.Join(dir, "s.png"))
	if code != 0 {
		t.Fatalf("exit %d; stderr %q", code, stderr)
	}
	b, err := os.ReadFile(filepath.Join(dir, "s.rank.md"))
	if err != nil {
		t.Fatal(err)
	}
	rows := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")[8:]
	for k, r := range rows {
		if !strings.HasPrefix(r, "| "+strconv.Itoa(k+1)+" |") {
			t.Errorf("row %d = %q, want rank %d over all rows", k, r, k+1)
		}
	}
}

func TestSweepRankErrors(t *testing.T) {
	out := filepath.Join(t.TempDir(), "x.png")
	base := []string{"--seeds", "1", "--stage", "layout", "--output", out}
	for _, args := range [][]string{
		{"--rank", ""},
		{"--rank", "no.such"},
		{"--rank", "land.met,land.met"},
		{"--rank", "land.met*0"},
		{"--rank", "default", "--rank-scope", "world"},
		{"--rank-scope", "all"},
		{"--table", "t.md"},
	} {
		if code, _, stderr := sweep(t, append(slices.Clone(base), args...)...); code != 2 {
			t.Errorf("%v: exit %d, want 2; stderr %q", args, code, stderr)
		}
	}
}
