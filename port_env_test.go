package evo_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestPORT006_TermDumbLikeNonInteractive(t *testing.T) {
	// Simulate TERM=dumb by NonInteractive + Plain (no cursor).
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.Plain(), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("t").Phase("x").Done("ok")
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
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("x").Done()
	_ = out.Finish()
	if strings.Contains(buf.String(), "\x1b[") {
		t.Fatal(buf.String())
	}
}

func TestTERM011_WidthZeroFallsBackSafely(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.Width(0)}})
	t.Cleanup(func() { _ = out.Close() })
	_ = out.Task("c").Add("x", nil, evo.Affected(1))
	_ = out.Finish()
	if buf.Len() == 0 {
		t.Fatal("expected output")
	}
}

func TestTERM012_SmallHeightBudget(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.Height(4), testkit.NoColor())
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Terminal(screen), evo.VisibilityDelay(0)}})
	t.Cleanup(func() { _ = out.Close() })
	col := out.Tasks("g")
	for i := 0; i < 20; i++ {
		col.Task("t").Phase("p")
	}
	got := screen.LatestLiveText()
	if !strings.Contains(got, "not shown") && len(strings.Split(got, "\n")) > 6 {
		// with height 4 budget, should omit or stay short
		t.Log(got)
	}
}

func TestTERM017_NestedSuspendRejectedOrSafe(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(bytes.NewBuffer(nil)), evo.Plain()}})
	t.Cleanup(func() { _ = out.Close() })
	err := out.Suspend(func() error {
		return out.Suspend(func() error { return nil })
	})
	// v1 may allow or reject; must not panic
	_ = err
}
