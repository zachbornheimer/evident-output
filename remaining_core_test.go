package evo_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestDOM030_CollectionWarning is updated for P2: Warn annotates a task
// instead of resolving it, so a task that only ever calls Warn stays
// non-terminal (Pending) until Finish's amnesty resolves it — before
// Finish, the collection reads Incomplete (one unresolved child), and the
// warning itself lives on that child's Warnings field.
func TestDOM030_CollectionWarning(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	g := out.DisplayGroup("g")
	g.Task("a").Done()
	g.Task("b").Warn("soft")
	snap := g.Snapshot()
	if snap.State != evo.Incomplete {
		t.Fatalf("state = %v, want Incomplete (Warn no longer resolves its task)", snap.State)
	}
	if warnings := snap.Tasks[1].Warnings; len(warnings) != 1 || warnings[0].Summary != "soft" {
		t.Fatalf("child warnings = %+v, want one warning %q", warnings, "soft")
	}
}

// TestDOM030b_CollectionWarningDetailIsRendered guards against a regression
// where writeCollection only special-cased Failed children: a group glyph
// like "!" rendered with no explanation of which child warned or why,
// because the Warn() message was recorded but never printed under the
// group summary line.
func TestDOM030b_CollectionWarningDetailIsRendered(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	g := out.DisplayGroup("capture")
	g.Task("Brewfile").Done()
	g.Task("Zen").Warn("skipped — zen-bootstrap not available")
	_ = out.Finish()
	_ = out.Close()

	rendered := buf.String()
	if !strings.Contains(rendered, "Zen") {
		t.Fatalf("rendered output missing warned child name %q: %s", "Zen", rendered)
	}
	if !strings.Contains(rendered, "skipped — zen-bootstrap not available") {
		t.Fatalf("rendered output missing warned child's reason: %s", rendered)
	}
}

func TestDOM031_CollectionAllDone(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	g := out.DisplayGroup("g")
	g.Summary("all good")
	g.Task("a").Done()
	g.Task("b").Done()
	if g.Snapshot().State != evo.Done {
		t.Fatal(g.Snapshot().State)
	}
	if g.Snapshot().Summary != "all good" {
		t.Fatal(g.Snapshot().Summary)
	}
}

// TestDOM035_UnresolvedChildInCollection pins release-gate round 4 finding
// 3: an unresolved child with no problems, on a clean finish, reads as an
// honest Partial outcome, never misuse — Finish returns nil.
func TestDOM035_UnresolvedChildInCollection(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	g := out.DisplayGroup("g")
	g.Task("a").Done()
	g.Task("hanging")
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (clean finish, no amnesty-defeating problems)", err)
	}
	if !out.Conclusion().Partial {
		t.Fatal("want Conclusion.Partial = true for the unresolved hanging child")
	}
}

func TestDOM049_OutputFail(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	out.Failf("stopped: %w", errors.New("disk"))
	_ = out.Finish()
	if out.Conclusion().State != evo.StateFailed {
		t.Fatal(out.Conclusion().State)
	}
}

func TestDOM048_BlockedWithNilErrorReturn(t *testing.T) {
	// Pattern from spec: presentation negative, callback returns nil
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	var ret error
	func() {
		out.Task("branches").Block("local-only")
		ret = nil
	}()
	if ret != nil {
		t.Fatal(ret)
	}
	_ = out.Finish()
	if out.Conclusion().State != evo.StateBlocked {
		t.Fatal(out.Conclusion().State)
	}
}

func TestLOG014_WarnMessageDistinctFromItemWarn(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })
	out.Println("log warning")
	out.Task("i").Warn("item warning")
	_ = out.Finish()
	s := buf.String()
	if !strings.Contains(s, "log warning") || !strings.Contains(s, "item warning") {
		t.Fatal(s)
	}
}

func TestLOG008_ConcurrentDebugWriters(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard), evo.DebugLevel(evo.LevelDebug)}})
	t.Cleanup(func() { _ = out.Close() })
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := out.DebugWriter()
			_, _ = w.Write([]byte("line\n"))
			_ = w.Close()
		}(i)
	}
	wg.Wait()
	_ = out.Finish()
}

func TestOUT007_DeterministicJSONWithFixedClock(t *testing.T) {
	// same semantic state → same conclusion fields (IDs differ by construction)
	mk := func() evo.Conclusion {
		out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
		out.Task("a").Done()
		out.Task("b").Block("x")
		_ = out.Finish()
		c := out.Conclusion()
		_ = out.Close()
		return c
	}
	a, b := mk(), mk()
	if a.State != b.State || a.ExitCode != b.ExitCode {
		t.Fatalf("%+v vs %+v", a, b)
	}
}

func TestOUT011_EventTimestampsPresent(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("a").Done()
	_ = out.Finish()
	for _, e := range out.Events() {
		if e.Timestamp.IsZero() {
			t.Fatal("zero timestamp")
		}
		if e.SchemaVersion != "0.3" {
			t.Fatal(e.SchemaVersion)
		}
	}
}

func TestCON005_CloseDuringUpdates(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out.Task("x").Done()
		}()
	}
	wg.Wait()
	_ = out.Close()
	_ = out.Close()
}

func TestAPI010_DonefFormatting(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("t").Done("n=%d", 3)
	s := out.Snapshot()
	found := false
	for _, tsk := range s.Tasks {
		if tsk.Summary == "n=3" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a task with Summary %q, got %+v", "n=3", s.Tasks)
	}
	// before finish
	if len(s.Tasks) == 0 {
		t.Fatal("no tasks")
	}
	if s.Tasks[0].Summary != "n=3" {
		t.Fatal(s.Tasks[0].Summary)
	}
}
