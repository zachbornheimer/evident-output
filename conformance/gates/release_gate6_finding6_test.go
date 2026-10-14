//go:build ignore

package gates_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// TestFinish_MisuseHint_UsesWarnGlyphStyling is release-gate round 6 finding
// 6: the misuse hint row's "!" glyph must render with the same warn (yellow)
// styling every other "!" row uses (writeTaxonomy's skipped/kept lines,
// writeAlreadyMutated) — asserted on rendered bytes, not left unstyled.
func TestFinish_MisuseHint_UsesWarnGlyphStyling(t *testing.T) {
	var buf bytes.Buffer
	out := (*evo.Run)(nil)
	_ = evo.Execute(context.Background(), evo.Config{Output: evo.Plain, Facades: evo.Facades{Stdout: &buf}}, func(ctx context.Context, r *evo.Run) error { return nil })

	task := out.Task("branches")
	task.Block("local-only branch")
	task.Block("second call ignored") // already resolved — misuse

	if err := error(nil); err == nil {
		t.Fatal("Finish() = nil, want the recorded misuse")
	}

	rendered := buf.String()
	const wantStyledGlyph = "\x1b[33m!\x1b[0m"
	if !strings.Contains(rendered, wantStyledGlyph) {
		t.Fatalf("misuse hint glyph missing warn styling %q; got:\n%q", wantStyledGlyph, rendered)
	}
}
