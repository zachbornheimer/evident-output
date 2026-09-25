package review_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// optionsFixtureDir is a doc fixture whose main.go the rewrite test
// overlays, so the rewritten source builds against the real module.
const optionsFixtureDir = "../../docexamples/fixtures/development_3"

// supersededOptionsSource is development_3 as docs/development.md taught
// it before E-088: Config.Options with four Options, every one of which
// has a Config field.
const supersededOptionsSource = `package main

import (
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func main() {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	clock := testkit.NewClock()
	out := evo.Init(evo.Config{Options: []evo.Option{
		evo.Terminal(screen),
		evo.Clock(clock),
		evo.VisibilityDelay(150 * time.Millisecond),
		evo.MaxFrameRate(20),
	}})
	_ = out
}
`

// applyReplace applies a "replace OLD with NEW" suggestion to src.
func applyReplace(t *testing.T, src, suggestion string) string {
	t.Helper()
	rest, ok := strings.CutPrefix(suggestion, "replace ")
	if !ok {
		t.Fatalf("suggestion is not a rewrite: %q", suggestion)
	}
	old, repl, ok := strings.Cut(rest, " with ")
	if !ok || !strings.Contains(src, old) {
		t.Fatalf("suggestion %q does not name source text", suggestion)
	}
	return strings.Replace(src, old, repl, 1)
}

// TestAPI032_OptionsRewriteKeepsEveryOptionAndCompiles pins E-088: the
// rewrite kept only VisibilityDelay, dropped Terminal/Clock/MaxFrameRate,
// and emitted &150 * time.Millisecond, which does not compile.
func TestAPI032_OptionsRewriteKeepsEveryOptionAndCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the rewritten source")
	}
	found := findAPI032(review.GoSource("main.go", supersededOptionsSource))
	if len(found) != 1 {
		t.Fatalf("want one API-032 finding for Config.Options, got %d: %q", len(found), joinSuggestions(found))
	}
	fixed := applyReplace(t, supersededOptionsSource, found[0].Suggestion)
	for _, field := range []string{"Terminal: screen", "Clock: clock", "VisibilityDelay: evo.Delay(150 * time.Millisecond)", "MaxFrameRate: 20"} {
		if !strings.Contains(fixed, field) {
			t.Errorf("rewrite dropped %q:\n%s", field, fixed)
		}
	}
	buildOverlay(t, fixed)
	if again := findAPI032(review.GoSource("main.go", fixed)); len(again) != 0 {
		t.Errorf("rewritten source still has API-032: %q", joinSuggestions(again))
	}
}

// TestAPI032_OptionsWithoutFieldOffersNoRewrite proves an Option with no
// one-to-one Config field gets a message, never a lossy or guessed
// rewrite (the old fallback swapped in "Stdout: w, Plain: true").
func TestAPI032_OptionsWithoutFieldOffersNoRewrite(t *testing.T) {
	src := `package p
import evo "github.com/zachbornheimer/evident-output"
func f(drv evo.TerminalDriver) {
	_ = evo.Init(evo.Config{Options: []evo.Option{evo.Terminal(drv), evo.DebugPane()}})
}
`
	found := findAPI032(review.GoSource("p.go", src))
	if len(found) != 1 {
		t.Fatalf("want one API-032 finding, got %d: %q", len(found), joinSuggestions(found))
	}
	if sug := found[0].Suggestion; strings.HasPrefix(sug, "replace ") || strings.Contains(sug, "Plain: true") {
		t.Fatalf("suggestion rewrites an Option with no Config field: %q", sug)
	}
}

// buildOverlay builds optionsFixtureDir with its main.go replaced by src.
func buildOverlay(t *testing.T, src string) {
	t.Helper()
	dir, err := filepath.Abs(optionsFixtureDir)
	if err != nil {
		t.Fatalf("resolve fixture: %v", err)
	}
	tmp := t.TempDir()
	mainGo := filepath.Join(tmp, "main.go")
	if err := os.WriteFile(mainGo, []byte(src), 0o600); err != nil {
		t.Fatalf("write rewritten source: %v", err)
	}
	overlay, err := json.Marshal(map[string]map[string]string{
		"Replace": {filepath.Join(dir, "main.go"): mainGo},
	})
	if err != nil {
		t.Fatalf("encode overlay: %v", err)
	}
	overlayPath := filepath.Join(tmp, "overlay.json")
	if err := os.WriteFile(overlayPath, overlay, 0o600); err != nil {
		t.Fatalf("write overlay: %v", err)
	}
	cmd := exec.Command("go", "build", "-buildvcs=false", "-overlay", overlayPath, "-o", os.DevNull, ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("rewritten source does not build: %v\n%s\n%s", err, out, src)
	}
}

// TestAPI032_OptionsCollidingWithSetFieldsOfferNoRewrite pins E-095: a
// rewrite of Options whose fields the Config literal already sets, or
// that repeat within the slice, emitted duplicate struct fields, which do
// not compile. It now gets the message-only form.
func TestAPI032_OptionsCollidingWithSetFieldsOfferNoRewrite(t *testing.T) {
	for name, cfg := range map[string]string{
		"already set": `evo.Config{Stdout: os.Stdout, Title: "t", Options: []evo.Option{evo.To(os.Stderr), evo.Title("u")}}`,
		"repeated":    `evo.Config{Options: []evo.Option{evo.Title("t"), evo.Title("u")}}`,
	} {
		src := "package p\nimport (\n\t\"os\"\n\tevo \"github.com/zachbornheimer/evident-output\"\n)\nvar _ = os.Stdout\nfunc f() { _ = evo.Init(" + cfg + ") }\n"
		found := findAPI032(review.GoSource("p.go", src))
		if len(found) != 1 {
			t.Fatalf("%s: want one API-032 finding, got %d: %q", name, len(found), joinSuggestions(found))
		}
		if sug := found[0].Suggestion; strings.HasPrefix(sug, "replace ") {
			t.Errorf("%s: suggestion rewrites into duplicate Config fields: %q", name, sug)
		}
	}
}
