package evo

import (
	"strings"
	"testing"

	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// TestFormatHistoryLine_WarnErrorReuseEntityGlyphPalette is release-gate
// round 9 finding 3: the debug journal's level token colored every level
// the same cyan, so a WARN or ERROR record read no differently than an INFO
// one in a scrollback that mixes all four. WARN and ERROR now reuse
// stateColor's Warning/Failed colors (sgrYellow/sgrRed) — the same severity
// vocabulary a Task's own row already uses. DEBUG/INFO keep the journal's
// existing cyan.
func TestFormatHistoryLine_WarnErrorReuseEntityGlyphPalette(t *testing.T) {
	cases := []struct {
		level string
		want  string
	}{
		{"DEBUG", txt.SGRCyan},
		{"INFO", txt.SGRCyan},
		{"WARN", txt.SGRYellow},
		{"ERROR", txt.SGRRed},
	}
	for _, tc := range cases {
		rec := debugRecord{Level: tc.level, Message: "m"}
		line := formatHistoryLine(rec, true)
		if !strings.Contains(line, tc.want) {
			t.Fatalf("level %s: want SGR %q in rendered line, got %q", tc.level, tc.want, line)
		}
	}
}
