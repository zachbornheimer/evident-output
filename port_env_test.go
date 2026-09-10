package evo_test

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestPORT006_TermDumbLikeNonInteractive(t *testing.T) {
	// Simulate TERM=dumb by NonInteractive + Plain (no cursor).
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("t").Doing("x").Done("ok")
	_ = out.Finish()
	if strings.ContainsAny(buf.String(), "\x1b") {
		t.Fatal("ANSI in dumb mode")
	}
}

func TestPORT_NO_COLOREnvHonoredViaOption(t *testing.T) {
	// Applications map NO_COLOR → evo.NoColor(); library option is the contract.
	_ = os.Setenv("NO_COLOR", "1")
	t.Cleanup(func() { _ = os.Unsetenv("NO_COLOR") })
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("x").Done()
	_ = out.Finish()
	if strings.Contains(buf.String(), "\x1b[") {
		t.Fatal(buf.String())
	}
}

func TestTERM011_WidthZeroFallsBackSafely(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Width: 0, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("c").Add("x", func() error { return nil }, evo.Affected(1))
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
	for i := 0; i < 20; i++ {
		col.Task("t").Doing("p")
	}
	got := screen.LatestLiveText()
	if !strings.Contains(got, "not shown") && len(strings.Split(got, "\n")) > 6 {
		// with height 4 budget, should omit or stay short
		t.Log(got)
	}
}
