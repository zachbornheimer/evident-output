package engine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/render"
	"github.com/zachbornheimer/evident-output/internal/scaletest"
)

// centralizeEffect is one package's Effect in the zq shape.
func centralizeEffect(verb EffectVerb, object string) func(context.Context) error {
	return effectOf(verb, object, 1)
}

// declareCentralize declares the zq shape: Group "centralize packages" with
// one Group per manager and npm/composer package Tasks under each.
func declareCentralize(out *Output, perManager int, work func(manager string, i int) func(context.Context) error) {
	category := out.Group("centralize packages")
	for _, manager := range []string{"npm", "composer"} {
		g := category.Group(manager)
		for i := range perManager {
			g.Task(fmt.Sprintf("%s pkg %d", manager, i)).Define(work(manager, i))
		}
	}
}

func runPlain(t *testing.T, cfg Config, declare func(*Output)) (string, *Output) {
	t.Helper()
	var buf strings.Builder
	cfg.Isolated, cfg.Plain, cfg.Color, cfg.Stdout, cfg.Stderr = true, true, ColorNever, &buf, io.Discard
	out := Init(cfg)
	declare(out)
	_ = out.Close()
	return buf.String(), out
}

func TestLedgerFoldsZqShapeToOneCategoryRow(t *testing.T) {
	rendered, out := runPlain(t, Config{}, func(o *Output) {
		declareCentralize(o, 3, func(string, int) func(context.Context) error { return centralizeEffect(EffectUpdate, "package") })
	})
	if want := "[changed] centralize packages  updated 6 packages\n"; !strings.Contains(rendered, want) {
		t.Fatalf("missing %q in:\n%s", want, rendered)
	}
	if n := strings.Count(rendered, "[changed] "); n != 1 {
		t.Errorf("%d [changed] rows, want 1:\n%s", n, rendered)
	}
	if got := len(out.Snapshot().Changes); got != 6 {
		t.Errorf("model Changes = %d, want every per-item section (6)", got)
	}
}

func TestLedgerFoldsDryRunToPlannedRow(t *testing.T) {
	rendered, _ := runPlain(t, Config{DryRun: true}, func(o *Output) {
		declareCentralize(o, 2, func(string, int) func(context.Context) error { return centralizeEffect(EffectUpdate, "package") })
	})
	if want := "[planned] centralize packages  update 4 packages\n"; !strings.Contains(rendered, want) {
		t.Fatalf("missing %q in:\n%s", want, rendered)
	}
}

func TestLedgerFoldKeepsMixedVerbsAsSeparateRows(t *testing.T) {
	rendered, _ := runPlain(t, Config{}, func(o *Output) {
		declareCentralize(o, 2, func(manager string, _ int) func(context.Context) error {
			if manager == "npm" {
				return centralizeEffect(EffectUpdate, "package")
			}
			return centralizeEffect(EffectDelete, "lockfile")
		})
	})
	for _, want := range []string{"[changed] npm       updated 2 packages", "[changed] composer  deleted 2 lockfiles"} {
		if !strings.Contains(rendered, want) {
			t.Errorf("missing %q in:\n%s", want, rendered)
		}
	}
}

func TestLedgerFoldLeavesASingleItemAsToday(t *testing.T) {
	rendered, _ := runPlain(t, Config{}, func(o *Output) {
		o.Group("solo").Task("only").Define(centralizeEffect(EffectUpdate, "package"))
	})
	if want := "[changed] only  updated 1 package"; !strings.Contains(rendered, want) {
		t.Fatalf("missing %q in:\n%s", want, rendered)
	}
}

func TestLedgerFoldCountsOnlyCommittedItemsAndKeepsFailuresVisible(t *testing.T) {
	boom := errors.New("registry unreachable")
	rendered, _ := runPlain(t, Config{}, func(o *Output) {
		declareCentralize(o, 3, func(manager string, i int) func(context.Context) error {
			if manager == "npm" && i == 1 {
				return func(context.Context) error { return boom }
			}
			return centralizeEffect(EffectUpdate, "package")
		})
	})
	if want := "[changed] centralize packages  updated 5 packages"; !strings.Contains(rendered, want) {
		t.Errorf("missing %q in:\n%s", want, rendered)
	}
	if !strings.Contains(rendered, "npm pkg 1") || !strings.Contains(rendered, "registry unreachable") {
		t.Errorf("failed item hidden by the fold:\n%s", rendered)
	}
}

func TestLedgerFoldLeavesJSONLEveryPerItemEffect(t *testing.T) {
	var stdout strings.Builder
	out := Init(Config{Isolated: true, Format: FormatJSONL, Stdout: &stdout, Stderr: io.Discard})
	declareCentralize(out, 3, func(string, int) func(context.Context) error { return centralizeEffect(EffectUpdate, "package") })
	_ = out.Close()
	if got := strings.Count(stdout.String(), `"effect.committed"`); got != 6 {
		t.Errorf("JSONL carries %d effect.committed events, want 6", got)
	}
}

// TestLedgerFoldTerminalRowAlignsToFoldedSubject proves the terminal
// projection (render.WriteLedger over a snapshot) folds and aligns the same
// way: the fold row and an unfolded row share one subject column.
func TestLedgerFoldTerminalRowAlignsToFoldedSubject(t *testing.T) {
	_, out := runPlain(t, Config{}, func(o *Output) {
		declareCentralize(o, 2, func(string, int) func(context.Context) error { return centralizeEffect(EffectUpdate, "package") })
		o.Task("cleanup").Define(centralizeEffect(EffectDelete, "cache"))
	})
	var b strings.Builder
	render.WriteLedger(&b, out.Snapshot(), 100, render.Style{})
	want := "[changed] centralize packages  updated 4 packages\n[changed] cleanup              deleted 1 cache\n"
	if b.String() != want {
		t.Errorf("WriteLedger =\n%q\nwant\n%q", b.String(), want)
	}
}

func TestLedgerGroupsThousandsOnEveryCountedQuantity(t *testing.T) {
	var b strings.Builder
	for _, sec := range render.FoldEffectSections("changed", 80, foldSources(1663)) {
		render.WriteEffects(&b, sec, render.Style{})
	}
	if want := "[changed] all  updated 1,663 packages\n"; b.String() != want {
		t.Errorf("folded = %q, want %q", b.String(), want)
	}
	b.Reset()
	render.WriteEffects(&b, render.EffectSection{Kind: "changed", Subject: "one", Records: []core.EffectRecord{{Verb: "updated", Object: "package", Quantity: 1663, HasQty: true}}}, render.Style{})
	if want := "[changed] one  updated 1,663 packages\n"; b.String() != want {
		t.Errorf("unfolded = %q, want %q", b.String(), want)
	}
	b.Reset()
	multi := []core.EffectRecord{{Verb: "updated", Object: "package", Quantity: 1663, HasQty: true}, {Verb: "deleted", Object: "lockfile", Quantity: 12, HasQty: true}}
	render.WriteEffects(&b, render.EffectSection{Kind: "changed", Subject: "two", Records: multi, Width: 80}, render.Style{})
	if want := "[changed]  two\n  updated  1,663 packages\n  deleted     12 lockfiles\n"; b.String() != want {
		t.Errorf("aligned = %q, want %q", b.String(), want)
	}
}

func foldSources(n int) []render.SectionSource {
	path := core.ContainerPath{{ID: "g", Name: "items"}, {ID: "root", Name: "all"}}
	sources := make([]render.SectionSource, n)
	for i := range sources {
		sources[i] = render.SectionSource{
			Subject: "item", Containers: path,
			Records: []core.EffectRecord{{Verb: "updated", Object: "package", Quantity: 1, HasQty: true}},
		}
	}
	return sources
}

// TestLedgerFoldScalesLinearlyAndBoundsRowMemory proves folding n per-item
// Effects costs O(n) and leaves one row with one record, however large n is.
func TestLedgerFoldScalesLinearlyAndBoundsRowMemory(t *testing.T) {
	const small, large = 10_000, 100_000
	time1 := func(n int) time.Duration {
		sources := foldSources(n)
		return scaletest.Fastest(scaletest.CheapSamples, func() time.Duration {
			runtime.GC()
			var rows []render.EffectSection
			cost := scaletest.CPUElapsed(func() {
				rows = render.FoldEffectSections("changed", 80, sources)
				var b strings.Builder
				for _, r := range rows {
					render.WriteEffects(&b, r, render.Style{})
				}
			})
			if len(rows) != 1 || len(rows[0].Records) != 1 || rows[0].Records[0].Quantity != int64(n) {
				t.Fatalf("n=%d folded to %d rows, want 1 row of 1 record counting %d: %+v", n, len(rows), n, rows)
			}
			return cost
		})
	}
	ts, tl := time1(small), time1(large)
	t.Logf("n=%d %s, n=%d %s", small, ts, large, tl)
	if tl > time.Minute {
		t.Fatalf("fold of %d sections took %s", large, tl)
	}
	if ratio := float64(tl) / float64(max(ts, time.Microsecond)); ratio > 20 {
		t.Errorf("10x input cost %.1fx, want roughly linear (<=20x)", ratio)
	}
}
