package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// rootCategories is zq's categories declared straight at the root: three
// sibling category Groups, running, each with folded tallies.
func rootCategories() core.Snapshot {
	return core.Snapshot{Collections: []core.TasksSnapshot{
		runningCategory("branches", 6),
		runningCategory("worktrees", 6),
		runningCategory("remote-tracking", 6),
	}}
}

// rootTaskCount is how many standalone root Tasks sit beside a root Group
// in rootGroupAndTasks.
const rootTaskCount = 8

// rootGroupAndTasks is a running root Group beside standalone root Tasks.
func rootGroupAndTasks() core.Snapshot {
	s := core.Snapshot{Collections: []core.TasksSnapshot{runningCategory("branches", 6)}}
	for i := range rootTaskCount {
		s.Tasks = append(s.Tasks, core.TaskSnapshot{Name: fmt.Sprintf("step-%d", i), State: core.Running, Phase: "working"})
	}
	return s
}

// TestLiveRoot_SpendsOneBudgetAcrossRootCollectionsAndTasks: the root is a
// header-less body, so sibling root Groups and root Tasks share one height
// instead of each root Group getting all of it.
func TestLiveRoot_SpendsOneBudgetAcrossRootCollectionsAndTasks(t *testing.T) {
	t.Parallel()
	shapes := map[string]core.Snapshot{
		"three root category Groups":   rootCategories(),
		"root Group beside root Tasks": rootGroupAndTasks(),
	}
	for name, s := range shapes {
		for height := zqFrameFloor; height <= roomyHeight; height++ {
			frame := liveFrame(s, height)
			if rows := frameRows(frame); rows > height {
				t.Fatalf("%s, height %d: live frame is %d rows:\n%s", name, height, rows, frame)
			}
		}
	}
}

// TestLiveRoot_CountsWhatDoesNotFit: a root Group or Task left out of the
// frame is counted on one "not shown" line, never dropped silently.
func TestLiveRoot_CountsWhatDoesNotFit(t *testing.T) {
	t.Parallel()
	frame := liveFrame(rootCategories(), truncatingHeight)
	if !strings.Contains(frame, "not shown") {
		t.Fatalf("truncated root frame has no \"not shown\" line:\n%s", frame)
	}
}

// TestLiveRoot_ShowsEverythingWhenItFits: with room, every root Group and
// Task is painted and nothing is hidden.
func TestLiveRoot_ShowsEverythingWhenItFits(t *testing.T) {
	t.Parallel()
	frame := liveFrame(rootGroupAndTasks(), roomyHeight)
	for _, want := range []string{"branches", "step-0", fmt.Sprintf("step-%d", rootTaskCount-1)} {
		if !strings.Contains(frame, want) {
			t.Fatalf("frame lacks %q:\n%s", want, frame)
		}
	}
	if strings.Contains(frame, "not shown") {
		t.Fatalf("frame hid rows it had room for:\n%s", frame)
	}
}
