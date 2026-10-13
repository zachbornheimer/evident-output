package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/review"
)

// supersededOptionsSource is development_3 as mcp/docs/development.md taught
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

// applyReplace applies a "replace OLD with NEW" suggestion to src. OLD is the
// first split that is lexically complete and occurs verbatim in src (" with "
// can also sit inside a literal).
func applyReplace(t *testing.T, src, suggestion string) string {
	t.Helper()
	rest, ok := strings.CutPrefix(suggestion, "replace ")
	if !ok {
		t.Fatalf("suggestion is not a rewrite: %q", suggestion)
	}
	for offset := 0; ; {
		i := strings.Index(rest[offset:], " with ")
		if i < 0 {
			t.Fatalf("suggestion %q does not name source text", suggestion)
		}
		cut := offset + i
		offset = cut + 1
		old, repl := rest[:cut], rest[cut+len(" with "):]
		if _, closed := leftPattern(old); closed && strings.Contains(src, old) {
			return strings.Replace(src, old, repl, 1)
		}
	}
}

// TestAPI032_OptionsRewriteKeepsEveryOptionAndCompiles pins E-088: the
// rewrite kept only VisibilityDelay, dropped Terminal/Clock/MaxFrameRate,
// and emitted &150 * time.Millisecond, which does not compile. Under 2.0
// the build step is replaced by a text check (see the comment below).
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
	// 2.0 removed Init and these Config fields (Facades.Terminal/Clock, no
	// VisibilityDelay or MaxFrameRate), so the 1.x rewrite can no longer be
	// built against the module; the E-088 compile defect is pinned textually.
	if strings.Contains(fixed, "&150") {
		t.Errorf("rewrite emitted an address-of literal that cannot compile:\n%s", fixed)
	}
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
func f(drv evo.TerminalDriver, extra interface{}) {
	_ = evo.Init(evo.Config{Options: []evo.Option{evo.Terminal(drv), extra}})
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
