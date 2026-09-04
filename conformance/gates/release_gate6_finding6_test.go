package gates_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestFinish_MisuseHint_UsesWarnGlyphStyling is release-gate round 6 finding
// 6: the misuse hint row's "!" glyph must render with the same warn (yellow)
// styling every other "!" row uses (writeTaxonomy's skipped/kept lines,
// writeAlreadyMutated) — asserted on rendered bytes, not left unstyled.
func TestFinish_MisuseHint_UsesWarnGlyphStyling(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(&buf), evo.Plain()}})

	task := out.Task("branches")
	task.Block("local-only branch")
	task.Block("second call ignored") // already resolved — misuse

	if err := out.Finish(); err == nil {
		t.Fatal("Finish() = nil, want the recorded misuse")
	}

	rendered := buf.String()
	const wantStyledGlyph = "\x1b[33m!\x1b[0m"
	if !strings.Contains(rendered, wantStyledGlyph) {
		t.Fatalf("misuse hint glyph missing warn styling %q; got:\n%q", wantStyledGlyph, rendered)
	}
}
