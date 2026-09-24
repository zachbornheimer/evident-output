package render

import (
	"strings"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// frameRows is how many terminal rows a live frame occupies.
func frameRows(frame string) int {
	if frame == "" {
		return 0
	}
	return strings.Count(frame, "\n") + 1
}

// liveFrame renders s's live region at height with fixed test settings.
func liveFrame(s core.Snapshot, height int) string {
	return LiveRegion(s, height, 80, time.Time{}, false, txt.GlyphsUnicode)
}

// runningCategory is one of zq clean-repo's category Groups mid-run:
// running work children plus folded Kept and Skipped items whose records
// each carry a cause.
func runningCategory(name string, running int) core.TasksSnapshot {
	col := causedItems(running)
	col.Name = name
	return col
}

// zqCategories is zq's live shape: a running "categories" Group holding
// the branches, worktrees and remote-tracking category Groups.
func zqCategories() core.Snapshot {
	return core.Snapshot{Collections: []core.TasksSnapshot{{
		Name:  "categories",
		State: core.Running,
		Collections: []core.TasksSnapshot{
			runningCategory("branches", 6),
			runningCategory("worktrees", 6),
			runningCategory("remote-tracking", 6),
		},
	}}}
}

// zqFrameFloor is the smallest height the categories Group fits in: its
// header, one child row, and the "not shown" line. Below it the frame is
// taller than the terminal by design (see minLiveChildRows).
const zqFrameFloor = liveHeaderRows + minLiveChildRows

// roomyHeight fits every row of zq's categories frame, causes included.
const roomyHeight = 60

// TestLiveNestedCategories_StayWithinTheHeight: every nested category Group
// spends only what its parent has left, so zq's categories frame never
// paints past the terminal height.
func TestLiveNestedCategories_StayWithinTheHeight(t *testing.T) {
	t.Parallel()
	for height := zqFrameFloor; height <= roomyHeight; height++ {
		frame := liveFrame(zqCategories(), height)
		if rows := frameRows(frame); rows > height {
			t.Fatalf("height %d: live frame is %d rows:\n%s", height, rows, frame)
		}
	}
}

// TestLiveNestedCategories_ShowEveryCategoryWhenTheyFit: with room, every
// category keeps its header and nothing is hidden.
func TestLiveNestedCategories_ShowEveryCategoryWhenTheyFit(t *testing.T) {
	t.Parallel()
	frame := liveFrame(zqCategories(), roomyHeight)
	for _, name := range []string{"branches", "worktrees", "remote-tracking"} {
		if !strings.Contains(frame, name) {
			t.Fatalf("frame lacks category %q:\n%s", name, frame)
		}
	}
	if strings.Contains(frame, "not shown") {
		t.Fatalf("frame hid rows it had room for:\n%s", frame)
	}
}

// truncatingHeight fits the first category but not all three.
const truncatingHeight = 10

// TestLiveNestedCategories_CountWhatDoesNotFit: a category that does not
// fit is counted in the parent's "not shown" line, never dropped silently.
func TestLiveNestedCategories_CountWhatDoesNotFit(t *testing.T) {
	t.Parallel()
	frame := liveFrame(zqCategories(), truncatingHeight)
	if !strings.Contains(frame, "not shown") {
		t.Fatalf("truncated frame has no \"not shown\" line:\n%s", frame)
	}
}

// ownTaskWithActivity is a category that renders as its own Task's row
// while that Task runs with a bar and an activity child (two rows), plus
// caused Kept and Skipped items that fold into tallies.
func ownTaskWithActivity() core.TasksSnapshot {
	col := causedItems(0)
	col.Name = "branches"
	own := core.TaskSnapshot{
		Name:     "branches",
		State:    core.Running,
		Phase:    "feature/x",
		Progress: core.Progress{Kind: core.Determinate, Completed: 3, Total: 10},
	}
	col.Tasks = append([]core.TaskSnapshot{own}, col.Tasks...)
	return col
}

// ownTaskActivityHeight is a height the own Task's two rows plus both
// tallies with their cause lines (four rows) overrun by one.
const ownTaskActivityHeight = 5

// TestLiveOwnTask_TalliesCountTheActivityRow: the own Task's activity
// child is a row too, so the tallies drop their causes rather than
// overrun the height.
func TestLiveOwnTask_TalliesCountTheActivityRow(t *testing.T) {
	t.Parallel()
	s := core.Snapshot{Collections: []core.TasksSnapshot{ownTaskWithActivity()}}
	frame := liveFrame(s, ownTaskActivityHeight)
	if rows := frameRows(frame); rows > ownTaskActivityHeight {
		t.Fatalf("live frame is %d rows, over the %d-row height:\n%s", rows, ownTaskActivityHeight, frame)
	}
	for _, want := range []string{"feature/x", "- skipped 2 (pinned)", "! kept 2 (pinned)"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("live frame lacks %q:\n%s", want, frame)
		}
	}
}

// testLiveStyle is the fixed live paint settings internal tests render
// with: 80 columns, a fixed spinner, no color, Unicode glyphs.
var testLiveStyle = liveStyle{width: 80, spin: "⠋", profile: txt.GlyphsUnicode}
