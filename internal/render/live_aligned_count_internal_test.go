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

// TestLiveRunningDetail_LoneCountTwoSpacesBeforeHeartbeat is the
// RED-then-GREEN regression for spec §18's own worked example
// ("install dependencies  [████        ]  14/40  — 7s"): a lone
// determinate row (no shared count column, cw == countWidths{}) gets the
// same two-space gap before "— Ns" as an aligned sibling row. The gap must
// not depend on whether a count column is shared — §18 draws no such
// distinction, and keying the gap off cw.done previously left a lone row
// one space short of the spec's own frame.
func TestLiveRunningDetail_LoneCountTwoSpacesBeforeHeartbeat(t *testing.T) {
	t.Parallel()
	firstSeen := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := firstSeen.Add(10 * time.Second)
	snap := core.NewTaskSnapshot(core.TaskSnapshot{
		Name:     "aa",
		State:    core.Running,
		Progress: core.Progress{Kind: core.Determinate, Completed: 14, Total: 40},
	}, firstSeen, false)
	st := liveStyle{Style: Style{Profile: txt.GlyphsUnicode}, width: 80, now: now}

	detail, elapsed := liveRunningDetail(snap, countWidths{}, st)
	if elapsed == "" {
		t.Fatalf("expected a heartbeat suffix 10s after first-seen")
	}
	if !strings.Contains(detail, "  — ") {
		t.Fatalf("lone count row must show a two-space gap before the heartbeat em dash, got %q", detail)
	}
}

// TestLiveRunningDetail_NarrowLoneCountTwoSpacesBeforeHeartbeat pins the
// same gap at a narrow terminal width, where liveCountDetail zeroes its own
// local cw to drop the bar/padding (evo-rec.md Problem 16/26's compact
// dialect). The heartbeat gap must not read the caller's un-zeroed cw to
// decide whether to add its space: at width 30 (below compactLayoutMaxWidth)
// a row sharing a count column with a sibling (cw.done != 0) rendered a
// two-space gap while an otherwise-identical lone row (cw.done == 0)
// rendered only one, even though both draw the same bare "N/M" once narrow.
func TestLiveRunningDetail_NarrowLoneCountTwoSpacesBeforeHeartbeat(t *testing.T) {
	t.Parallel()
	firstSeen := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := firstSeen.Add(10 * time.Second)
	newSnap := func() core.TaskSnapshot {
		return core.NewTaskSnapshot(core.TaskSnapshot{
			Name:     "aa",
			State:    core.Running,
			Progress: core.Progress{Kind: core.Determinate, Completed: 3, Total: 10},
		}, firstSeen, false)
	}
	st := liveStyle{Style: Style{Profile: txt.GlyphsUnicode}, width: 30, now: now}

	lone, _ := liveRunningDetail(newSnap(), countWidths{}, st)
	aligned, _ := liveRunningDetail(newSnap(), countWidths{done: 2, total: 2}, st)
	if lone != aligned {
		t.Fatalf("narrow lone and aligned rows of the same shape must render identically, got lone %q vs aligned %q", lone, aligned)
	}
	if !strings.Contains(lone, "  — ") {
		t.Fatalf("narrow lone count row must show a two-space gap before the heartbeat em dash, got %q", lone)
	}
}
