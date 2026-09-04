package gates_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestFinish_AbandonedEachLoop_RendersPartialModifierOnBand is the red-first
// case for release-gate round 4 finding 1: Conclusion.Partial must reach the
// printed human conclusion band, not just the struct. Each is abandoned at
// 1/3 (never sealed), the run returns cleanly (no signal, no other
// failure/cancellation), so Finish reads the task Incomplete and the
// conclusion Partial — the band must carry that fact as the `[<outcome> ·
// partial]` modifier, asserted on rendered bytes (the round-3 blind spot:
// asserting only the Conclusion struct let this regress silently).
func TestFinish_AbandonedEachLoop_RendersPartialModifierOnBand(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.NoColor(), evo.Plain()}})

	items := []string{"one", "two", "three"}
	for range out.Task("install").Each(items) {
		break // abandoned at 1/3 — never sealed, never given a terminal verb
	}

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish() = %v, want nil (an abandoned loop is not misuse)", err)
	}
	conc := out.Conclusion()
	if !conc.Partial {
		t.Fatalf("Conclusion.Partial = false, want true (abandoned loop)")
	}

	rendered := buf.String()
	if !strings.Contains(rendered, "· partial]") {
		t.Fatalf("rendered band missing the partial modifier, got:\n%s", rendered)
	}
}
