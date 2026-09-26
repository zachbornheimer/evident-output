package render

import (
	"strings"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// TestLiveRunningDetail_AlignedCountNoTrailingSpaceBeforeHeartbeat is the
// RED-then-GREEN regression for formatAlignedCount's unconditional trailing
// space: a determinate Running row sharing a count column (countWidths
// nonzero, from two-plus sibling rows) always got one padding space
// appended, meant to combine with heartbeatSuffix's own leading space into
// a two-space gap before "— Ns". Before elapsedAfter (5s) that suffix is
// still "", so the row's detail ended on a bare trailing space — an aligned
// sibling row would render "3/10 " (trailing whitespace) for its first 5s,
// while a lone unaligned row of the same shape never carried one.
func TestLiveRunningDetail_AlignedCountNoTrailingSpaceBeforeHeartbeat(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	snap := core.NewTaskSnapshot(core.TaskSnapshot{
		Name:     "aa",
		State:    core.Running,
		Progress: core.Progress{Kind: core.Determinate, Completed: 3, Total: 10},
	}, now, false)
	cw := countWidths{done: 2, total: 2} // shared column, as two+ siblings would produce
	st := liveStyle{Style: Style{Profile: txt.GlyphsUnicode}, width: 80, now: now}

	detail, elapsed := liveRunningDetail(snap, cw, st)
	if elapsed != "" {
		t.Fatalf("expected no heartbeat suffix yet (now == first-seen), got %q", elapsed)
	}
	if strings.HasSuffix(detail, " ") {
		t.Fatalf("aligned count detail must not end in trailing whitespace before the heartbeat suffix appears: %q", detail)
	}
}
