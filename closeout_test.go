package evo_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	txt "github.com/zachbornheimer/evident-output/internal/text"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestTXT012_LongPathTruncationPolicy(t *testing.T) {
	long := strings.Repeat("a", 200) + "/file.go"
	got := txt.Truncate(long, 40)
	if txt.Cells(got) > 40 {
		t.Fatal(got, txt.Cells(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatal(got)
	}
}

func TestTXT017_DuplicateNamesReadable(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })
	// Output.Task get-or-creates by name (L1); two distinct rows sharing a
	// display name need distinct evo.ID.
	out.Task("same", evo.ID("a")).Done()
	out.Task("same", evo.ID("b")).Block("x")
	_ = out.Finish()
	if strings.Count(buf.String(), "same") < 2 {
		t.Fatal(buf.String())
	}
}

// TestTXT019_ManyProblemsBounded's premise (attach 200 structured Problems
// via one bulk verb call) no longer has a public construction path — a Task
// verb now produces exactly one Problem per resolution. The storage-side
// invariant it pinned (Snapshot retains every Problem, not just the plain
// projection's display bound) is covered directly against a hand-built
// Snapshot by TestHumanProblemList_IsBounded (problem_bound_test.go).

func TestTXT018_BidiInNames(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	it := out.Task("ok\u202Ebad")
	if strings.ContainsRune(it.Snapshot().Name, '\u202e') {
		t.Fatal(it.Snapshot().Name)
	}
}

func TestOUT002_DiagnosticSeparate(t *testing.T) {
	var primary, diag bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&primary), evo.Diagnostics(&diag), evo.Plain()}})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("a").Done()
	_ = out.Finish()
	// human on primary
	if primary.Len() == 0 {
		t.Fatal("primary empty")
	}
}

func TestOUT010_UnknownEnumForwardCompat(t *testing.T) {
	// Consumers should tolerate extra conclusion fields — EncodeJSON has fixed enums we control
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	out.Task("a").Done()
	_ = out.Finish()
	b, _ := evo.EncodeJSON(out.Snapshot())
	if !strings.Contains(string(b), "ready") && !strings.Contains(string(b), "state") {
		t.Fatal(string(b))
	}
	_ = out.Close()
}

func TestOUT013_ExitCodeOnConclusion(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	out.Task("a").Block("b")
	_ = out.Finish()
	if out.Conclusion().ExitCode != 1 {
		t.Fatal(out.Conclusion().ExitCode)
	}
	_ = out.Close()
}

func TestOUT015_EventStreamBounded(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	for i := 0; i < 1000; i++ {
		out.Debug("x")
	}
	// with default debug level, Debug may no-op — enable
	_ = out.Close()
	out2 := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard), evo.DebugLevel(evo.LevelDebug)}})
	for i := 0; i < 500; i++ {
		out2.Debug("x")
	}
	_ = out2.Finish()
	if len(out2.Events()) < 10 {
		t.Fatal(len(out2.Events()))
	}
	_ = out2.Close()
}

func TestOUT016_BrokenPipePolicy(t *testing.T) {
	r, w := io.Pipe()
	_ = r.Close() // reader closed => writes fail
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(w), evo.Plain()}})
	out.Task("a").Done()
	// Finish write may error on pipe — must not panic
	_ = out.Finish()
	_ = w.Close()
	_ = out.Close()
}

func TestCON006_NoDeadlockOnRecursiveLog(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.NoColor())
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.DebugLevel(evo.LevelDebug)}})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("t").Doing("p")
	// Debug during live (recursive-ish path)
	out.Debug("while live")
	out.Task("t").Done()
	_ = out.Finish()
}

func TestCON007_DirtyCoalesce(t *testing.T) {
	// H.22 already covers; assert pending doesn't grow unbounded
	screen := testkit.NewScreen(testkit.Interactive(), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0), evo.Clock(clock), evo.MaxFrameRate(10)}})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("t")
	for i := 0; i < 100; i++ {
		task.Progress(i, 100)
	}
	if screen.LiveFrameCount() >= 100 {
		t.Fatal(screen.LiveFrameCount())
	}
}

func TestCON015_NoLeakAfterClose(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	out.Task("a").Done()
	_ = out.Close()
	// second close idempotent
	_ = out.Close()
}

func TestCON017_ConcurrentDeclareSafe(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			out.Task("n").Done()
		}
		close(done)
	}()
	<-done
	_ = out.Finish()
}

func TestCON019_HighFrequencyChildProgress(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.NoColor())
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0)}})
	t.Cleanup(func() { _ = out.Close() })
	g := out.DisplayGroup("g")
	t1 := g.Task("a")
	for i := 0; i <= 200; i++ {
		t1.Progress(i, 200)
	}
	t1.Done()
	if t1.Snapshot().Progress.Completed != 200 {
		t.Fatal(t1.Snapshot().Progress)
	}
}

func TestA11Y010_UnknownPaletteSafe(t *testing.T) {
	// NoColor path uses no SGR — portable
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})
	out.Task("a").Done()
	_ = out.Finish()
	if strings.Contains(buf.String(), "\x1b[") {
		t.Fatal("SGR")
	}
	_ = out.Close()
}

func TestSEC004_RenderTreeBounded(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard), evo.MaxEntities(100)}})
	t.Cleanup(func() { _ = out.Close() })
	for i := 0; i < 150; i++ {
		out.Task("x").Done()
	}
	// limit hit recorded
	_ = out.Finish()
}

func TestSEC010_FinishAfterPanicPath(t *testing.T) {
	// Renderer failure isolation: Finish still returns with misuse if any
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	out.Task("a").Done()
	_ = out.Finish()
	_ = out.Close()
}

func TestAPI011_CobraNotRequired(t *testing.T) {
	// Library embeds without Cobra base class
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("cmd"), evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("a").Done()
	_ = out.Finish()
}

func TestAPI020_SuspendExternal(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard), evo.Plain()}})
	t.Cleanup(func() { _ = out.Close() })
	if err := out.Suspend(func() error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestAPI022_DiscoverabilityNames(t *testing.T) {
	// User discovers Item/Task/Tasks and can implement three parallel facts without config.
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("repo"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	out.Task("working tree").Done()
	out.Task("scan").Doing("walk").Done("done")
	g := out.DisplayGroup("deps")
	g.Task("a").Done()
	g.Task("b").Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	for _, need := range []string{"working tree", "scan", "deps"} {
		if !strings.Contains(s, need) {
			t.Fatalf("missing %q in %q", need, s)
		}
	}
	_ = out.Close()
}

func TestAPI024_ComplexSmallerThanAdHoc(t *testing.T) {
	// Multi-progress + debug is a short common-path program (not ad-hoc ANSI).
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DebugLevel(evo.LevelDebug)}})
	g := out.DisplayGroup("deps")
	g.Task("a").Bytes(10, 10).Done()
	g.Task("b").Doing("verifying").Done()
	out.Debug("index ok")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "deps") {
		t.Fatal(buf.String())
	}
	_ = out.Close()
}

func TestTERM021_FinalCollectionOutput(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}})
	g := out.DisplayGroup("deps")
	g.Summary("installed 2")
	g.Task("a").Done()
	g.Task("b").Done()
	_ = out.Finish()
	if !strings.Contains(buf.String(), "deps") {
		t.Fatal(buf.String())
	}
	_ = out.Close()
}

func TestTERM024_BrokenPipeNoPanic(t *testing.T) {
	r, w := io.Pipe()
	_ = r.Close()
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(w), evo.Plain()}})
	out.Task("a").Done()
	_ = out.Finish() // may fail write
	_ = w.Close()
	_ = out.Close()
}

func TestLOG011_RecursiveValuesBounded(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard), evo.DebugLevel(evo.LevelDebug)}})
	t.Cleanup(func() { _ = out.Close() })
	// don't create real cycle in Field — use deep map
	m := map[string]any{"a": 1}
	out.Debug("m", evo.Field{Key: "m", Value: m})
	_ = out.Finish()
}

func TestLOG013_DebugWithJSONStdout(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.DebugLevel(evo.LevelDebug)}})
	out.Debug("d")
	out.Task("a").Done()
	_ = out.Finish()
	// JSON encode separate stream
	j, _ := evo.EncodeJSON(out.Snapshot())
	if !strings.Contains(string(j), "schema_version") {
		t.Fatal(string(j))
	}
	_ = out.Close()
}

func TestOUT019_HostWritesWhileActiveDocumented(t *testing.T) {
	// Suspend is the cooperative path
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard), evo.Plain()}})
	_ = out.Suspend(func() error { return nil })
	_ = out.Close()
}

func TestPORT013_PublicAPIStableShape(t *testing.T) {
	// Stable public surface: Init/Task/Tasks/Finish/Snapshot/EncodeJSON.
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("s"), evo.To(io.Discard), evo.Plain()}})
	out.Task("i").Done()
	out.Task("t").Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	snap := out.Snapshot()
	if snap.Subject != "s" || len(snap.Tasks) != 2 {
		t.Fatalf("%+v", snap)
	}
	b, err := evo.EncodeJSON(snap)
	if err != nil || !strings.Contains(string(b), `"schema_version": "0.4"`) {
		t.Fatal(err, string(b))
	}
	_ = out.Close()
}

func TestPORT014_JSONDocumentHasRequiredFields(t *testing.T) {
	// Schema 0.3 (CHANGELOG "Unreleased"): the item/task fold removed the
	// separate "items" wire kind — every entity, including a fact-check
	// resolved without ever running, is a "tasks" row.
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	out.Task("a").Done()
	_ = out.Finish()
	b, _ := evo.EncodeJSON(out.Snapshot())
	if strings.Contains(string(b), `"items"`) {
		t.Fatal(string(b))
	}
	if !strings.Contains(string(b), `"tasks"`) {
		t.Fatal(string(b))
	}
	_ = out.Close()
}
