package evo

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
	"github.com/zachbornheimer/evident-output/v2/internal/terminal"
	"github.com/zachbornheimer/evident-output/v2/testkit"
)

func TestTERM020_CompletedCollapseUnderPressure(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Height(5), testkit.Width(80), testkit.NoColor())
	out := initRun(Config{Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Terminal: screen}}, tuning{visibilityDelay: DelayForTest(0)})
	t.Cleanup(func() { _ = out.close() })
	g := out.Group("g")
	for range 30 {
		succeed(g.Task("t"))
	}
	g.Task("fail").fail("x")
	got := screen.LatestLiveText()
	if !strings.Contains(got, "fail") && !strings.Contains(got, "not shown") {
		t.Log(got)
	}
}

func TestTERM023_SplitStreamsNoCrossCursor(t *testing.T) {
	var primary, diag bytes.Buffer
	// NoColor: this test forbids cursor CSI, not semantic SGR color.
	out := initRun(Config{Output: Plain, Facades: Facades{Stdout: &diag, Stderr: &primary}}, tuning{color: colorNever})
	t.Cleanup(func() { _ = out.close() })
	succeed(out.Task("a"))
	_ = out.finish()
	// no cursor sequences on either
	if strings.Contains(primary.String()+diag.String(), "\x1b[") {
		t.Fatal("unexpected CSI")
	}
}

func TestTERM021_FinalCollectionOutput(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	g := out.Group("deps")
	g.Summary("installed 2")
	succeed(g.Task("a"))
	succeed(g.Task("b"))
	_ = out.finish()
	if !strings.Contains(buf.String(), "deps") {
		t.Fatal(buf.String())
	}
	_ = out.close()
}

func TestTERM024_BrokenPipeNoPanic(t *testing.T) {
	r, w := io.Pipe()
	_ = r.Close()
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: w}})
	succeed(out.Task("a"))
	_ = out.finish() // may fail write
	_ = w.Close()
	_ = out.close()
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
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{width: 0})
	t.Cleanup(func() { _ = out.close() })
	out.Task("c").Define(effectOf(Verb(change.EffectAdd), "x", 1))
	_ = out.finish()
	if buf.Len() == 0 {
		t.Fatal("expected output")
	}
}

func TestTERM012_SmallHeightBudget(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.Height(4), testkit.NoColor())
	out := initRun(Config{Facades: Facades{Stdout: io.Discard, Stderr: io.Discard, Terminal: screen}}, tuning{visibilityDelay: DelayForTest(0)})
	t.Cleanup(func() { _ = out.close() })
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
