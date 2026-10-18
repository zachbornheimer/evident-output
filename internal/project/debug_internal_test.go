package project

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
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
		{"DEBUG", SGRCyan},
		{"INFO", SGRCyan},
		{"WARN", SGRYellow},
		{"ERROR", SGRRed},
	}
	for _, tc := range cases {
		rec := record.DebugRecord{Level: tc.level, Message: "m"}
		line := FormatHistoryLine(rec, true)
		if !strings.Contains(line, tc.want) {
			t.Fatalf("level %s: want SGR %q in rendered line, got %q", tc.level, tc.want, line)
		}
	}
}
