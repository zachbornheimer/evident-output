package live

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// shortTerminalRows is a live height the header, the folded tally, one
// running child and the "not shown" line fill exactly.
const shortTerminalRows = 4

// excludedItems is a Group of running children plus Excluded item children
// that fold into one tally.
func excludedItems(running int) record.TasksSnapshot {
	var tasks []record.TaskSnapshot
	for i := range running {
		tasks = append(tasks, record.TaskSnapshot{Name: fmt.Sprintf("package-%d", i), State: record.Running, Phase: "downloading"})
	}
	for i := range 4 {
		tasks = append(tasks, excludedItem(fmt.Sprintf("item-%d", i), "pinned"))
	}
	return record.TasksSnapshot{Name: "packages", State: record.Running, Tasks: tasks}
}

// TestLiveTally_StaysWithinAShortHeight: the folded tally is one row the
// frame counts, so a live Group never paints past the terminal height.
func TestLiveTally_StaysWithinAShortHeight(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	writeLiveCollection(&b, excludedItems(3), shortTerminalRows, testLiveStyle)
	frame := b.String()
	if rows := strings.Count(frame, "\n"); rows > shortTerminalRows {
		t.Fatalf("live frame is %d rows, over the %d-row height:\n%s", rows, shortTerminalRows, frame)
	}
	for _, want := range []string{"○ excluded 4 (pinned)", "package-0"} {
		if !strings.Contains(frame, want) {
			t.Fatalf("live frame lacks %q:\n%s", want, frame)
		}
	}
}
