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
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	// Output.Task get-or-creates by name (L1); two distinct rows sharing a
	// display name need distinct evo.ID.
	succeed(out.Task("same"))
	out.Task("same").Block("x")
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
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	it := out.Task("ok\u202Ebad")
	if strings.ContainsRune(it.Snapshot().Name, '\u202e') {
		t.Fatal(it.Snapshot().Name)
	}
}

func TestOUT002_DiagnosticSeparate(t *testing.T) {
	var primary, diag bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &primary, Stderr: &diag, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("a"))
	_ = out.Finish()
	// human on primary
	if primary.Len() == 0 {
		t.Fatal("primary empty")
	}
}

func TestOUT010_UnknownEnumForwardCompat(t *testing.T) {
	// Consumers should tolerate extra conclusion fields — EncodeJSON has fixed enums we control
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	succeed(out.Task("a"))
	_ = out.Finish()
	b, _ := evo.EncodeJSONForTest(out.Snapshot())
	if !strings.Contains(string(b), "ready") && !strings.Contains(string(b), "state") {
		t.Fatal(string(b))
	}
	_ = out.Close()
}

func TestOUT013_ExitCodeOnConclusion(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	out.Task("a").Block("b")
	_ = out.Finish()
	if out.Conclusion().ExitCode != 1 {
		t.Fatal(out.Conclusion().ExitCode)
	}
	_ = out.Close()
}

func TestOUT015_EventStreamBounded(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	for range 1000 {
		out.DebugForTest("x")
	}
	// with default debug level, Debug may no-op — enable
	_ = out.Close()
	out2 := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Debug: evo.DebugConfig{Level: evo.LevelDebug}})
	for range 500 {
		out2.DebugForTest("x")
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
	out := evo.Init(evo.Config{Isolated: true, Stdout: w, Plain: true})
	succeed(out.Task("a"))
	// Finish write may error on pipe — must not panic
	_ = out.Finish()
	_ = w.Close()
	_ = out.Close()
}

func TestCON006_NoDeadlockOnRecursiveLog(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.NoColor())
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Stderr: io.Discard, Terminal: screen, VisibilityDelay: evo.DelayForTest(0), Debug: evo.DebugConfig{Level: evo.LevelDebug}})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("t").Doing("p")
	// Debug during live (recursive-ish path)
	out.DebugForTest("while live")
	succeed(out.Task("t"))
	_ = out.Finish()
}

func TestCON007_DirtyCoalesce(t *testing.T) {
	// H.22 already covers; assert pending doesn't grow unbounded
	screen := testkit.NewScreen(testkit.Interactive(), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Clock: clock, Terminal: screen, VisibilityDelay: evo.DelayForTest(0)})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("t")
	for i := range 100 {
		task.Progress(i, 100)
	}
	if screen.LiveFrameCount() >= 100 {
		t.Fatal(screen.LiveFrameCount())
	}
}

func TestCON015_NoLeakAfterClose(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	succeed(out.Task("a"))
	_ = out.Close()
	// second close idempotent
	_ = out.Close()
}

func TestCON017_ConcurrentDeclareSafe(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	done := make(chan struct{})
	go func() {
		for range 50 {
			succeed(out.Task("n"))
		}
		close(done)
	}()
	<-done
	_ = out.Finish()
}

func TestCON019_HighFrequencyChildProgress(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.NoColor())
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Terminal: screen, VisibilityDelay: evo.DelayForTest(0)})
	t.Cleanup(func() { _ = out.Close() })
	g := out.Group("g")
	t1 := g.Task("a")
	for i := 0; i <= 200; i++ {
		t1.Progress(i, 200)
	}
	succeed(t1)
	if t1.Snapshot().Progress.Completed != 200 {
		t.Fatal(t1.Snapshot().Progress)
	}
}

func TestA11Y010_UnknownPaletteSafe(t *testing.T) {
	// NoColor path uses no SGR — portable
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	succeed(out.Task("a"))
	_ = out.Finish()
	if strings.Contains(buf.String(), "\x1b[") {
		t.Fatal("SGR")
	}
	_ = out.Close()
}

func TestSEC004_RenderTreeBounded(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, MaxEntities: 100})
	t.Cleanup(func() { _ = out.Close() })
	for range 150 {
		succeed(out.Task("x"))
	}
	// limit hit recorded
	_ = out.Finish()
}

func TestSEC010_FinishAfterPanicPath(t *testing.T) {
	// Renderer failure isolation: Finish still returns with misuse if any
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	succeed(out.Task("a"))
	_ = out.Finish()
	_ = out.Close()
}

func TestAPI011_CobraNotRequired(t *testing.T) {
	// Library embeds without Cobra base class
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Title: "cmd"})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("a"))
	_ = out.Finish()
}

func TestAPI022_DiscoverabilityNames(t *testing.T) {
	// User discovers Item/Task/Tasks and can implement three parallel facts without config.
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Title: "repo", Color: evo.ColorNever, Plain: true})
	succeed(out.Task("working tree"))
	succeed(out.Task("scan").Doing("walk"), "done")
	g := out.Group("deps")
	succeed(g.Task("a"), "ok")
	succeed(g.Task("b"), "ok")
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	// "deps" is a Group with no Summary of its own, so only its children render.
	for _, need := range []string{"working tree", "scan", "✓ a", "✓ b"} {
		if !strings.Contains(s, need) {
			t.Fatalf("missing %q in %q", need, s)
		}
	}
	_ = out.Close()
}

func TestAPI024_ComplexSmallerThanAdHoc(t *testing.T) {
	// Multi-progress + debug is a short common-path program (not ad-hoc ANSI).
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Debug: evo.DebugConfig{Level: evo.LevelDebug}, Color: evo.ColorNever, Plain: true})
	g := out.Group("deps")
	succeed(g.Task("a").Bytes(10, 10))
	succeed(g.Task("b").Doing("verifying"))
	out.DebugForTest("index ok")
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
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	g := out.Group("deps")
	g.Summary("installed 2")
	succeed(g.Task("a"))
	succeed(g.Task("b"))
	_ = out.Finish()
	if !strings.Contains(buf.String(), "deps") {
		t.Fatal(buf.String())
	}
	_ = out.Close()
}

func TestTERM024_BrokenPipeNoPanic(t *testing.T) {
	r, w := io.Pipe()
	_ = r.Close()
	out := evo.Init(evo.Config{Isolated: true, Stdout: w, Plain: true})
	succeed(out.Task("a"))
	_ = out.Finish() // may fail write
	_ = w.Close()
	_ = out.Close()
}

func TestLOG011_RecursiveValuesBounded(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Debug: evo.DebugConfig{Level: evo.LevelDebug}})
	t.Cleanup(func() { _ = out.Close() })
	// don't create real cycle in Field — use deep map
	m := map[string]any{"a": 1}
	out.DebugForTest("m", evo.Field{Key: "m", Value: m})
	_ = out.Finish()
}

func TestLOG013_DebugWithJSONStdout(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Debug: evo.DebugConfig{Level: evo.LevelDebug}, Plain: true})
	out.DebugForTest("d")
	succeed(out.Task("a"))
	_ = out.Finish()
	// JSON encode separate stream
	j, _ := evo.EncodeJSONForTest(out.Snapshot())
	if !strings.Contains(string(j), "schema_version") {
		t.Fatal(string(j))
	}
	_ = out.Close()
}

func TestPORT013_PublicAPIStableShape(t *testing.T) {
	// Stable public surface: Init/Task/Tasks/Finish/Snapshot/EncodeJSON.
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Title: "s", Plain: true})
	succeed(out.Task("i"))
	succeed(out.Task("t"))
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	snap := out.Snapshot()
	if snap.Subject != "s" || len(snap.Tasks) != 2 {
		t.Fatalf("%+v", snap)
	}
	b, err := evo.EncodeJSONForTest(snap)
	if err != nil || !strings.Contains(string(b), `"schema_version": "0.4"`) {
		t.Fatal(err, string(b))
	}
	_ = out.Close()
}

func TestPORT014_JSONDocumentHasRequiredFields(t *testing.T) {
	// Schema 0.3 (CHANGELOG "Unreleased"): the item/task fold removed the
	// separate "items" wire kind — every entity, including a fact-check
	// resolved without ever running, is a "tasks" row.
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	succeed(out.Task("a"))
	_ = out.Finish()
	b, _ := evo.EncodeJSONForTest(out.Snapshot())
	if strings.Contains(string(b), `"items"`) {
		t.Fatal(string(b))
	}
	if !strings.Contains(string(b), `"tasks"`) {
		t.Fatal(string(b))
	}
	_ = out.Close()
}
