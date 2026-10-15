package evo

import (
	"bytes"
	"strings"
	"testing"
)

// TestEffectQuantity_AcceptsLenDirectly is beginner-6: EffectSpec.Quantity
// is int, not int64, so the natural caller shape
// `EffectSpec{..., Quantity: len(x)}` compiles without a manual
// conversion. This test's mere compilation is most of the proof; it also
// checks the recorded quantity renders correctly.
func TestEffectQuantity_AcceptsLenDirectly(t *testing.T) {
	items := []string{"a", "b", "c"}

	var buf bytes.Buffer
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: &buf}}, tuning{color: colorNever})
	out.Task("cleanup").Define(effectOf(Delete, "stale local branch", len(items)))
	_ = out.finish()

	if !strings.Contains(buf.String(), "3") {
		t.Fatalf("expected quantity 3 rendered, got:\n%s", buf.String())
	}
}
