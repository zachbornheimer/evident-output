package live

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/project"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// projectionEpoch anchors every row's live-first-seen stamp, so elapsed
// suffixes render and the header's earliest-seen anchor is exercised.
var projectionEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// projectLive is s as the engine projects it for a live frame of rows
// rows: every collection, and the root Tasks, through LiveChildren.
func projectLive(s record.Snapshot, rows int) record.Snapshot {
	out := s
	cols := projectCollections(s.Collections, rows)
	out.Collections = cols.Kept()
	out = project.WithRootCollectionTally(out, cols.Tally())
	root := projectTasks(record.TasksSnapshot{Tasks: s.Tasks}, rows)
	out.Tasks = root.Tasks
	if tally, ok := project.ChildTallyOf(root); ok {
		out = project.WithRootTally(out, tally)
	}
	return out
}

func projectCollection(col record.TasksSnapshot, rows int) record.TasksSnapshot {
	return projectTasks(projectCollections(col.Collections, rows).Into(col), rows)
}

// projectCollections is cols through LiveCollections, as the engine
// feeds it: the reachable ones projected, the rest omitted whole.
func projectCollections(cols []record.TasksSnapshot, rows int) *LiveCollections {
	lc := NewLiveCollections(rows)
	for _, child := range cols {
		if lc.Admit(collectionRank(child)) {
			lc.Keep(projectCollection(child, rows))
		} else {
			var counts project.ChildCounts
			countSubtree(&counts, child)
			lc.Omit(counts, OwnRowName(child))
		}
	}
	return lc
}

// perItemGroups is a Group of n per-item Groups, two Tasks each, in the
// states a live frame ranks (E-091's nested shape).
func perItemGroups(name string, n int, summary string) record.TasksSnapshot {
	col := record.TasksSnapshot{Name: name, State: record.Running}
	for i := range n {
		item := record.TasksSnapshot{
			Name: fmt.Sprintf("item-%d", i), State: record.Running, Summary: summary,
			Tasks: mixedTasks(fmt.Sprintf("i%d", i), 2),
		}
		if i%4 == 0 {
			item.Tasks = []record.TaskSnapshot{seen(record.TaskSnapshot{Name: fmt.Sprintf("item-%d", i), State: record.Succeeded}, time.Duration(i)*time.Second)}
		}
		col.Collections = append(col.Collections, item)
	}
	return col
}

func projectTasks(col record.TasksSnapshot, rows int) record.TasksSnapshot {
	children := NewLiveChildren(col.Name, rows)
	for i := range col.Tasks {
		if children.Admit(&col.Tasks[i]) {
			children.Keep(col.Tasks[i])
		}
	}
	return children.Collection(col)
}

// countSubtree adds every Task at or below col to counts.
func countSubtree(counts *project.ChildCounts, col record.TasksSnapshot) {
	for i := range col.Tasks {
		counts.Add(&col.Tasks[i])
	}
	for _, child := range col.Collections {
		countSubtree(counts, child)
	}
}

// seen is t first painted at offset past projectionEpoch.
func seen(t record.TaskSnapshot, offset time.Duration) record.TaskSnapshot {
	return record.NewTaskSnapshot(t, projectionEpoch.Add(offset), false)
}

// mixedTasks is n children in every state a live frame ranks, in an
// interleaving that puts each attention rank past the frame's rows.
func mixedTasks(prefix string, n int) []record.TaskSnapshot {
	var tasks []record.TaskSnapshot
	for i := range n {
		t := record.TaskSnapshot{Name: fmt.Sprintf("%s-%d", prefix, i), State: record.Succeeded}
		switch {
		case i%17 == 0:
			t.State, t.Summary = record.Failed, "exit 1"
		case i%5 == 0:
			t.State, t.Phase = record.Running, "fetching"
		case i%3 == 0:
			t.State = record.Pending
		case i%7 == 0:
			t.State = record.Refused
		}
		tasks = append(tasks, seen(t, time.Duration(n-i)*time.Second))
	}
	return tasks
}

// items is n disposition items: each Excluded for one of two reasons and
// nothing else.
func items(prefix string, n int) []record.TaskSnapshot {
	var tasks []record.TaskSnapshot
	for i := range n {
		name := fmt.Sprintf("%s-%d", prefix, i)
		tasks = append(tasks, seen(excludedItem(name, []string{"protected", "merged"}[i%2]), time.Duration(i)*time.Second))
	}
	return tasks
}

func workPeers(n int) []record.TaskSnapshot {
	var tasks []record.TaskSnapshot
	for i := range n {
		tasks = append(tasks, record.TaskSnapshot{Name: fmt.Sprintf("peer-%d", i), State: record.Succeeded, Facts: []record.Fact{{Name: "files", Value: "3"}}})
	}
	return tasks
}

// projectionShapes are the collection shapes whose live frame a projection
// must not change: large enough that it leaves children out.
func projectionShapes() map[string]record.Snapshot {
	ownTask := record.TasksSnapshot{
		Name: "branches", State: record.Running,
		Tasks: append([]record.TaskSnapshot{seen(record.TaskSnapshot{Name: "branches", State: record.Running, Phase: "classify"}, time.Minute)}, items("tip", 400)...),
	}
	summarized := record.TasksSnapshot{
		Name: "packages", State: record.Running, Summary: "3 installed",
		Tasks: append(append(items("pkg", 400), workPeers(30)...), mixedTasks("dep", 60)...),
	}
	peers := record.TasksSnapshot{
		Name: "tags", State: record.Running,
		Tasks: append(append(workPeers(2), items("tag", 400)...), mixedTasks("t", 5)...),
	}
	onlyItems := record.TasksSnapshot{
		Name: "prune", State: record.Running,
		Tasks: append(items("wt", 400), seen(record.TaskSnapshot{Name: "wt-last", State: record.Running}, 0)),
	}
	return map[string]record.Snapshot{
		"mixed group":                     {Collections: []record.TasksSnapshot{{Name: "items", State: record.Running, Tasks: mixedTasks("item", 500)}}},
		"own task and items":              {Collections: []record.TasksSnapshot{ownTask}},
		"summary folds items beside work": {Collections: []record.TasksSnapshot{summarized}},
		"work peer keeps items as rows":   {Collections: []record.TasksSnapshot{peers}},
		"items and one running":           {Collections: []record.TasksSnapshot{onlyItems}},
		"nested categories": {Collections: []record.TasksSnapshot{{
			Name: "categories", State: record.Running,
			Collections: []record.TasksSnapshot{ownTask, summarized, {Name: "steps", State: record.Running, Sequential: true, Tasks: mixedTasks("step", 300)}},
		}}},
		"root tasks beside a group": {
			Tasks:       mixedTasks("root", 400),
			Collections: []record.TasksSnapshot{{Name: "small", State: record.Running, Tasks: mixedTasks("s", 4)}},
		},
		"finished flat group": {
			Tasks: []record.TaskSnapshot{{Name: "build", State: record.Failed, Summary: "exit 2"}},
			Collections: []record.TasksSnapshot{{Name: "done", State: record.Failed, Tasks: func() []record.TaskSnapshot {
				var ts []record.TaskSnapshot
				for i := range 500 {
					t := record.TaskSnapshot{Name: fmt.Sprintf("a-much-longer-finished-name-%d", i), State: record.Succeeded}
					if i%100 == 0 {
						t = record.TaskSnapshot{Name: "build", State: record.Failed, Summary: "exit 1"}
					}
					ts = append(ts, t)
				}
				return ts
			}()}},
		},
		"per-item groups":             {Collections: []record.TasksSnapshot{perItemGroups("items", 300, "")}},
		"per-item groups with header": {Collections: []record.TasksSnapshot{perItemGroups("items", 300, "checking")}},
		"root per-item groups":        {Collections: perItemGroups("items", 300, "").Collections},
		"pending queue": {Collections: []record.TasksSnapshot{{Name: "queue", State: record.Running, Tasks: func() []record.TaskSnapshot {
			var ts []record.TaskSnapshot
			for i := range 1000 {
				ts = append(ts, seen(record.TaskSnapshot{Name: fmt.Sprintf("q-%d", i), State: record.Pending}, time.Duration(i)*time.Millisecond))
			}
			return ts
		}()}}},
	}
}

// TestLiveProjection_PaintsTheSameFrame: a projection that keeps only the
// children a frame could show paints the frame the whole snapshot paints,
// at every height, so bounding frame work by the screen changes nothing a
// viewer sees.
func TestLiveProjection_PaintsTheSameFrame(t *testing.T) {
	t.Parallel()
	now := projectionEpoch.Add(10 * time.Minute)
	style := project.Style{Profile: project.GlyphsUnicode}
	for name, s := range projectionShapes() {
		for _, rows := range []int{4, 8, 24, 60} {
			want := LiveRegion(s, rows, 80, now, style)
			got := LiveRegion(projectLive(s, rows), rows, 80, now, style)
			if got != want {
				t.Errorf("%s at %d rows: projected frame differs\n--- projected\n%s\n--- whole\n%s", name, rows, got, want)
			}
		}
	}
}

// TestLiveProjection_KeepsWhatTheScreenShows: however many children a
// collection holds, its projection keeps a bounded number of them — the
// frame's rows for each attention rank plus one screen of the rest.
func TestLiveProjection_KeepsWhatTheScreenShows(t *testing.T) {
	t.Parallel()
	const rows = 24
	bound := (rows + 1) + attentionRankCount*rows + (rows + 1)
	for _, n := range []int{1000, 16000} {
		col := record.TasksSnapshot{Name: "items", Tasks: append(mixedTasks("item", n), items("it", n)...)}
		kept := len(projectTasks(col, rows).Tasks)
		if kept > bound {
			t.Errorf("projection of %d children kept %d; want at most %d whatever the run size", 2*n, kept, bound)
		}
	}
}

// TestLiveProjection_SmallCollectionIsWhole: a collection that fits keeps
// every child and carries no tally.
func TestLiveProjection_SmallCollectionIsWhole(t *testing.T) {
	t.Parallel()
	col := record.TasksSnapshot{Name: "items", Tasks: mixedTasks("item", 10)}
	got := projectTasks(col, 24)
	if _, partial := project.ChildTallyOf(got); partial || len(got.Tasks) != 10 {
		t.Errorf("small collection projected to %d children (partial=%v); want all 10, whole", len(got.Tasks), partial)
	}
	if !strings.Contains(LiveRegion(record.Snapshot{Collections: []record.TasksSnapshot{got}}, 24, 80, projectionEpoch, project.Style{Profile: project.GlyphsUnicode}), "item-9") {
		t.Errorf("small collection frame lost a child")
	}
}
