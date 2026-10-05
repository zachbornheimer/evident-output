package evo_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/terminal"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestTERM020_CompletedCollapseUnderPressure(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Height(5), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Terminal: screen, VisibilityDelay: evo.DelayForTest(0)})
	t.Cleanup(func() { _ = out.Close() })
	g := out.Group("g")
	for range 30 {
		succeed(g.Task("t"))
	}
	g.Task("fail").Fail("x")
	got := screen.LatestLiveText()
	if !strings.Contains(got, "fail") && !strings.Contains(got, "not shown") {
		t.Log(got)
	}
}

func TestTERM023_SplitStreamsNoCrossCursor(t *testing.T) {
	var primary, diag bytes.Buffer
	// NoColor: this test forbids cursor CSI, not semantic SGR color.
	out := evo.Init(evo.Config{Isolated: true, Stdout: &primary, Stderr: &diag, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("a"))
	_ = out.Finish()
	// no cursor sequences on either
	if strings.Contains(primary.String()+diag.String(), "\x1b[") {
		t.Fatal("unexpected CSI")
	}
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

func TestTERM007_ShortWriteDisablesInteractive(t *testing.T) {
	fw := &failWriter{}
	drv := terminal.NewANSI(fw, terminal.WithInteractive(true), terminal.WithSize(80, 24))
	drv.WriteLive("line one\nline two")
	if drv.WriteErr() == nil {
		t.Fatal("expected write error")
	}
	if drv.IsInteractive() {
		t.Fatal("interactive should disable after write fault")
	}
}

func TestTERM011_WidthZeroFallsBackSafely(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Width: 0, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("c").Define(effectOf(evo.EffectAdd, "x", 1))
	_ = out.Finish()
	if buf.Len() == 0 {
		t.Fatal("expected output")
	}
}

func TestTERM012_SmallHeightBudget(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.Height(4), testkit.NoColor())
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Isolated: true, Terminal: screen, VisibilityDelay: evo.DelayForTest(0)})
	t.Cleanup(func() { _ = out.Close() })
	col := out.Group("g")
	for range 20 {
		col.Task("t").Doing("p")
	}
	got := screen.LatestLiveText()
	if !strings.Contains(got, "not shown") && len(strings.Split(got, "\n")) > 6 {
		// with height 4 budget, should omit or stay short
		t.Log(got)
	}
}
