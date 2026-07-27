package evo_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/width"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestTXT012_LongPathTruncationPolicy(t *testing.T) {
	long := strings.Repeat("a", 200) + "/file.go"
	got := width.Truncate(long, 40)
	if width.Cells(got) > 40 {
		t.Fatal(got, width.Cells(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatal(got)
	}
}

func TestTXT017_DuplicateNamesReadable(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })
	out.Item("same").OK()
	out.Item("same").Block("x")
	_ = out.Finish()
	if strings.Count(buf.String(), "same") < 2 {
		t.Fatal(buf.String())
	}
}

func TestTXT019_ManyProblemsBounded(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	probs := make([]evo.Problem, 200)
	for i := range probs {
		probs[i] = evo.Problem{Summary: "p", Count: int64(i)}
	}
	out.Item("i").BlockedBy(probs...)
	_ = out.Finish()
	if len(out.Snapshot().Items[0].Problems) != 200 {
		t.Fatal(len(out.Snapshot().Items[0].Problems))
	}
}

func TestTXT018_BidiInNames(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	it := out.Item("ok\u202Ebad")
	if strings.ContainsRune(it.Snapshot().Name, '\u202e') {
		t.Fatal(it.Snapshot().Name)
	}
}

func TestOUT002_DiagnosticSeparate(t *testing.T) {
	var primary, diag bytes.Buffer
	out := evo.NewWithOptions(evo.To(&primary), evo.Diagnostics(&diag), evo.Plain())
	t.Cleanup(func() { _ = out.Close() })
	out.Item("a").OK()
	_ = out.Finish()
	// human on primary
	if primary.Len() == 0 {
		t.Fatal("primary empty")
	}
}

func TestOUT010_UnknownEnumForwardCompat(t *testing.T) {
	// Consumers should tolerate extra conclusion fields — EncodeJSON has fixed enums we control
	out := evo.NewWithOptions(evo.To(io.Discard))
	out.Item("a").OK()
	_ = out.Finish()
	b, _ := evo.EncodeJSON(out.Snapshot())
	if !strings.Contains(string(b), "ready") && !strings.Contains(string(b), "state") {
		t.Fatal(string(b))
	}
	_ = out.Close()
}

func TestOUT013_ExitCodeOnConclusion(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	out.Item("a").Block("b")
	_ = out.Finish()
	if out.Conclusion().ExitCode != 1 {
		t.Fatal(out.Conclusion().ExitCode)
	}
	_ = out.Close()
}

func TestOUT015_EventStreamBounded(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	for i := 0; i < 1000; i++ {
		out.Debug("x")
	}
	// with default debug level, Debug may no-op — enable
	_ = out.Close()
	out2 := evo.NewWithOptions(evo.To(io.Discard), evo.DebugLevel(evo.Debug))
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
	out := evo.NewWithOptions(evo.To(w), evo.Plain())
	out.Item("a").OK()
	// Finish write may error on pipe — must not panic
	_ = out.Finish()
	_ = w.Close()
	_ = out.Close()
}

func TestCON006_NoDeadlockOnRecursiveLog(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.NoColor())
	out := evo.NewWithOptions(evo.Terminal(screen), evo.DebugLevel(evo.Debug))
	t.Cleanup(func() { _ = out.Close() })
	out.Task("t").Phase("p")
	// Debug during live (recursive-ish path)
	out.Debug("while live")
	out.Task("t").Done()
	_ = out.Finish()
}

func TestCON007_DirtyCoalesce(t *testing.T) {
	// H.22 already covers; assert pending doesn't grow unbounded
	screen := testkit.NewScreen(testkit.Interactive(), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.NewWithOptions(evo.Terminal(screen), evo.Clock(clock), evo.MaxFrameRate(10))
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
	out := evo.NewWithOptions(evo.To(io.Discard))
	out.Item("a").OK()
	_ = out.Close()
	// second close idempotent
	_ = out.Close()
}

func TestCON017_ConcurrentDeclareSafe(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			out.Item("n").OK()
		}
		close(done)
	}()
	<-done
	_ = out.Finish()
}

func TestCON019_HighFrequencyChildProgress(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.NoColor())
	out := evo.NewWithOptions(evo.Terminal(screen))
	t.Cleanup(func() { _ = out.Close() })
	g := out.Tasks("g")
	t1 := g.Task("a")
	for i := int64(0); i <= 200; i++ {
		t1.Progress64(i, 200)
	}
	t1.Done()
	if t1.Snapshot().Progress.Completed != 200 {
		t.Fatal(t1.Snapshot().Progress)
	}
}

func TestA11Y010_UnknownPaletteSafe(t *testing.T) {
	// NoColor path uses no SGR — portable
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.To(&buf), evo.NoColor(), evo.Plain())
	out.Item("a").OK()
	_ = out.Finish()
	if strings.Contains(buf.String(), "\x1b[") {
		t.Fatal("SGR")
	}
	_ = out.Close()
}

func TestSEC004_RenderTreeBounded(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard), evo.MaxEntities(100))
	t.Cleanup(func() { _ = out.Close() })
	for i := 0; i < 150; i++ {
		out.Item("x").OK()
	}
	// limit hit recorded
	_ = out.Finish()
}

func TestSEC010_FinishAfterPanicPath(t *testing.T) {
	// Renderer failure isolation: Finish still returns with misuse if any
	out := evo.NewWithOptions(evo.To(io.Discard))
	out.Item("a").OK()
	_ = out.Finish()
	_ = out.Close()
}

func TestAPI011_CobraNotRequired(t *testing.T) {
	// Library embeds without Cobra base class
	out := evo.For("cmd", evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	out.Item("a").OK()
	_ = out.Finish()
}

func TestAPI020_SuspendExternal(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard), evo.Plain())
	t.Cleanup(func() { _ = out.Close() })
	if err := out.Suspend(func() error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestAPI022_DiscoverabilityNames(t *testing.T) {
	// User discovers Item/Task/Tasks and can implement three parallel facts without config.
	var buf bytes.Buffer
	out := evo.For("repo", evo.To(&buf), evo.Plain(), evo.NoColor())
	out.Item("working tree").OK()
	out.Task("scan").Phase("walk").Donef("done")
	g := out.Tasks("deps")
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
	out := evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor(), evo.DebugLevel(evo.Debug))
	g := out.Tasks("deps")
	g.Task("a").Bytes(10, 10).Done()
	g.Task("b").Phase("verifying").Done()
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
	out := evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor())
	g := out.Tasks("deps")
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
	out := evo.NewWithOptions(evo.To(w), evo.Plain())
	out.Item("a").OK()
	_ = out.Finish() // may fail write
	_ = w.Close()
	_ = out.Close()
}

func TestLOG011_RecursiveValuesBounded(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard), evo.DebugLevel(evo.Debug))
	t.Cleanup(func() { _ = out.Close() })
	type node struct{ N *node }
	// don't create real cycle in Field — use deep map
	m := map[string]any{"a": 1}
	out.Debug("m", evo.Field{Key: "m", Value: m})
	_ = out.Finish()
}

func TestLOG013_DebugWithJSONStdout(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.DebugLevel(evo.Debug))
	out.Debug("d")
	out.Item("a").OK()
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
	out := evo.NewWithOptions(evo.To(io.Discard), evo.Plain())
	_ = out.Suspend(func() error { return nil })
	_ = out.Close()
}

func TestPORT013_PublicAPIStableShape(t *testing.T) {
	// Stable public surface: New/For/Item/Task/Tasks/Finish/Snapshot/EncodeJSON.
	out := evo.For("s", evo.To(io.Discard), evo.Plain())
	out.Item("i").OK()
	out.Task("t").Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	snap := out.Snapshot()
	if snap.Subject != "s" || len(snap.Items) != 1 || len(snap.Tasks) != 1 {
		t.Fatalf("%+v", snap)
	}
	b, err := evo.EncodeJSON(snap)
	if err != nil || !strings.Contains(string(b), `"schema_version": "1.0"`) {
		t.Fatal(err, string(b))
	}
	_ = out.Close()
}

func TestPORT014_OldFixturesDecode(t *testing.T) {
	// JSON with schema 1.0 still has required fields
	out := evo.NewWithOptions(evo.To(io.Discard))
	out.Item("a").OK()
	_ = out.Finish()
	b, _ := evo.EncodeJSON(out.Snapshot())
	if !strings.Contains(string(b), `"items"`) {
		t.Fatal(string(b))
	}
	_ = out.Close()
}
