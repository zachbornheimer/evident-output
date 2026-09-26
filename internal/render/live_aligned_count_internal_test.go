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

// TestFormatAlignedCount_OwnsNoTrailingSpace is the RED-then-GREEN
// regression for moving the aligned count column's fixed gap out of
// formatAlignedCount and into liveRunningDetail's composition: the count
// field itself, aligned or not, is exactly "N/M" with no padding of its
// own. The prior form encoded the two-space-before-heartbeat gap as an
// unconditional trailing space on the field, which is what forced the
// TrimRight-after-the-fact workaround this slice replaces.
func TestFormatAlignedCount_OwnsNoTrailingSpace(t *testing.T) {
	t.Parallel()
	if got := formatAlignedCount(3, 10, countWidths{done: 2, total: 2}); got != " 3/10" {
		t.Fatalf("formatAlignedCount must not pad a trailing space onto the aligned field, got %q", got)
	}
	if got := formatAlignedCount(3, 10, countWidths{}); got != "3/10" {
		t.Fatalf("formatAlignedCount's unaligned field must stay bare, got %q", got)
	}
}

// TestLiveRunningDetail_AlignedCountTwoSpacesBeforeHeartbeat is the
// RED-then-GREEN regression pinning the far side of the same gap: once
// elapsedAfter has passed and heartbeatSuffix contributes its own leading
// space, an aligned count column (countWidths nonzero, no Phase) must still
// read as a two-space gap before "— Ns" — the composition now owns adding
// that second space, rather than the field padding it in advance.
func TestLiveRunningDetail_AlignedCountTwoSpacesBeforeHeartbeat(t *testing.T) {
	t.Parallel()
	firstSeen := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := firstSeen.Add(10 * time.Second)
	snap := core.NewTaskSnapshot(core.TaskSnapshot{
		Name:     "aa",
		State:    core.Running,
		Progress: core.Progress{Kind: core.Determinate, Completed: 3, Total: 10},
	}, firstSeen, false)
	cw := countWidths{done: 2, total: 2}
	st := liveStyle{Style: Style{Profile: txt.GlyphsUnicode}, width: 80, now: now}

	detail, elapsed := liveRunningDetail(snap, cw, st)
	if elapsed == "" {
		t.Fatalf("expected a heartbeat suffix 10s after first-seen")
	}
	if !strings.Contains(detail, "  — ") {
		t.Fatalf("aligned count row must show a two-space gap before the heartbeat em dash, got %q", detail)
	}
	if strings.Contains(detail, "   — ") {
		t.Fatalf("aligned count row must not show a three-space gap before the heartbeat em dash, got %q", detail)
	}
}
