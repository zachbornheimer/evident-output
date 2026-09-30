package live

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/render"

	"github.com/zachbornheimer/evident-output/internal/core"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// projectionEpoch anchors every row's live-first-seen stamp, so elapsed
// suffixes render and the header's earliest-seen anchor is exercised.
var projectionEpoch = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// projectLive is s as the engine projects it for a live frame of rows
// rows: every collection, and the root Tasks, through LiveChildren.
func projectLive(s core.Snapshot, rows int) core.Snapshot {
	out := s
	cols := projectCollections(s.Collections, rows)
	out.Collections = cols.Kept()
	out = core.WithRootCollectionTally(out, cols.Tally())
	root := projectTasks(core.TasksSnapshot{Tasks: s.Tasks}, rows)
	out.Tasks = root.Tasks
	if tally, ok := core.ChildTallyOf(root); ok {
		out = core.WithRootTally(out, tally)
	}
	return out
}

func projectCollection(col core.TasksSnapshot, rows int) core.TasksSnapshot {
	return projectTasks(projectCollections(col.Collections, rows).Into(col), rows)
}

// projectCollections is cols through LiveCollections, as the engine
// feeds it: the reachable ones projected, the rest omitted whole.
func projectCollections(cols []core.TasksSnapshot, rows int) *LiveCollections {
	lc := NewLiveCollections(rows)
	for _, child := range cols {
		if lc.Admit(collectionRank(child)) {
			lc.Keep(projectCollection(child, rows))
		} else {
			var counts core.ChildCounts
			countSubtree(&counts, child)
			lc.Omit(counts, OwnRowName(child))
		}
	}
	return lc
}

// perItemGroups is a Group of n per-item Groups, two Tasks each, in the
// states a live frame ranks (E-091's nested shape).
func perItemGroups(name string, n int, summary string) core.TasksSnapshot {
	col := core.TasksSnapshot{Name: name, State: core.Running}
	for i := range n {
		item := core.TasksSnapshot{Name: fmt.Sprintf("item-%d", i), State: core.Running, Summary: summary,
			Tasks: mixedTasks(fmt.Sprintf("i%d", i), 2)}
		if i%4 == 0 {
			item.Tasks = []core.TaskSnapshot{seen(core.TaskSnapshot{Name: fmt.Sprintf("item-%d", i), State: core.Done}, time.Duration(i)*time.Second)}
		}
		col.Collections = append(col.Collections, item)
	}
	return col
}

func projectTasks(col core.TasksSnapshot, rows int) core.TasksSnapshot {
	children := NewLiveChildren(col.Name, rows)
	for i := range col.Tasks {
		if children.Admit(&col.Tasks[i]) {
			children.Keep(col.Tasks[i])
		}
	}
	return children.Collection(col)
}

// countSubtree adds every Task at or below col to counts.
func countSubtree(counts *core.ChildCounts, col core.TasksSnapshot) {
	for i := range col.Tasks {
		counts.Add(&col.Tasks[i])
	}
	for _, child := range col.Collections {
		countSubtree(counts, child)
	}
}

// seen is t first painted at offset past projectionEpoch.
func seen(t core.TaskSnapshot, offset time.Duration) core.TaskSnapshot {
	return core.NewTaskSnapshot(t, projectionEpoch.Add(offset), false)
}

// mixedTasks is n children in every state a live frame ranks, in an
// interleaving that puts each attention rank past the frame's rows.
func mixedTasks(prefix string, n int) []core.TaskSnapshot {
	var tasks []core.TaskSnapshot
	for i := range n {
		t := core.TaskSnapshot{Name: fmt.Sprintf("%s-%d", prefix, i), State: core.Done}
		switch {
		case i%17 == 0:
			t.State, t.Summary = core.Failed, "exit 1"
		case i%11 == 0:
			t.Warnings = []core.Problem{{Summary: "slow mirror"}}
		case i%5 == 0:
			t.State, t.Phase = core.Running, "fetching"
		case i%3 == 0:
			t.State = core.Pending
		case i%7 == 0:
			t.State = core.Blocked
		}
		tasks = append(tasks, seen(t, time.Duration(n-i)*time.Second))
	}
	return tasks
}

// items is n disposition items: each resolved Kept or Skipped and nothing
// else.
func items(prefix string, n int) []core.TaskSnapshot {
	var tasks []core.TaskSnapshot
	for i := range n {
		name := fmt.Sprintf("%s-%d", prefix, i)
		rec := []core.TaxonomyRecord{{Reason: []string{"protected", "merged"}[i%2], Name: name, Causes: []string{"cause of " + name}}}
		t := core.TaskSnapshot{Name: name, State: core.Done, Kept: rec}
		if i%3 == 0 {
			t = core.TaskSnapshot{Name: name, State: core.Skipped, Skipped: rec}
		}
		tasks = append(tasks, seen(t, time.Duration(i)*time.Second))
	}
	return tasks
}

func workPeers(n int) []core.TaskSnapshot {
	var tasks []core.TaskSnapshot
	for i := range n {
		tasks = append(tasks, core.TaskSnapshot{Name: fmt.Sprintf("peer-%d", i), State: core.Done, Facts: []core.Fact{{Name: "files", Value: "3"}}})
	}
	return tasks
}

// projectionShapes are the collection shapes whose live frame a projection
// must not change: large enough that it leaves children out.
func projectionShapes() map[string]core.Snapshot {
	ownTask := core.TasksSnapshot{Name: "branches", State: core.Running,
		Tasks: append([]core.TaskSnapshot{seen(core.TaskSnapshot{Name: "branches", State: core.Running, Phase: "classify"}, time.Minute)}, items("tip", 400)...)}
	summarized := core.TasksSnapshot{Name: "packages", State: core.Running, Summary: "3 installed",
		Tasks: append(append(items("pkg", 400), workPeers(30)...), mixedTasks("dep", 60)...)}
	peers := core.TasksSnapshot{Name: "tags", State: core.Running,
		Tasks: append(append(workPeers(2), items("tag", 400)...), mixedTasks("t", 5)...)}
	onlyItems := core.TasksSnapshot{Name: "prune", State: core.Running,
		Tasks: append(items("wt", 400), seen(core.TaskSnapshot{Name: "wt-last", State: core.Running}, 0))}
	return map[string]core.Snapshot{
		"mixed group":                     {Collections: []core.TasksSnapshot{{Name: "items", State: core.Running, Tasks: mixedTasks("item", 500)}}},
		"own task and items":              {Collections: []core.TasksSnapshot{ownTask}},
		"summary folds items beside work": {Collections: []core.TasksSnapshot{summarized}},
		"work peer keeps items as rows":   {Collections: []core.TasksSnapshot{peers}},
		"items and one running":           {Collections: []core.TasksSnapshot{onlyItems}},
		"nested categories": {Collections: []core.TasksSnapshot{{Name: "categories", State: core.Running,
			Collections: []core.TasksSnapshot{ownTask, summarized, {Name: "steps", State: core.Running, Sequential: true, Tasks: mixedTasks("step", 300)}}}}},
		"root tasks beside a group": {Tasks: mixedTasks("root", 400),
			Collections: []core.TasksSnapshot{{Name: "small", State: core.Running, Tasks: mixedTasks("s", 4)}}},
		"finished flat group": {Tasks: []core.TaskSnapshot{{Name: "build", State: core.Failed, Summary: "exit 2"}},
			Collections: []core.TasksSnapshot{{Name: "done", State: core.Failed, Tasks: func() []core.TaskSnapshot {
				var ts []core.TaskSnapshot
				for i := range 500 {
					t := core.TaskSnapshot{Name: fmt.Sprintf("a-much-longer-finished-name-%d", i), State: core.Done}
					if i%100 == 0 {
						t = core.TaskSnapshot{Name: "build", State: core.Failed, Summary: "exit 1"}
					}
					ts = append(ts, t)
				}
				return ts
			}()}}},
		"per-item groups":             {Collections: []core.TasksSnapshot{perItemGroups("items", 300, "")}},
		"per-item groups with header": {Collections: []core.TasksSnapshot{perItemGroups("items", 300, "checking")}},
		"root per-item groups":        {Collections: perItemGroups("items", 300, "").Collections},
		"pending queue": {Collections: []core.TasksSnapshot{{Name: "queue", State: core.Running, Tasks: func() []core.TaskSnapshot {
			var ts []core.TaskSnapshot
			for i := range 1000 {
				ts = append(ts, seen(core.TaskSnapshot{Name: fmt.Sprintf("q-%d", i), State: core.Pending}, time.Duration(i)*time.Millisecond))
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
	style := render.Style{Profile: txt.GlyphsUnicode}
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
		col := core.TasksSnapshot{Name: "items", Tasks: append(mixedTasks("item", n), items("it", n)...)}
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
	col := core.TasksSnapshot{Name: "items", Tasks: mixedTasks("item", 10)}
	got := projectTasks(col, 24)
	if _, partial := core.ChildTallyOf(got); partial || len(got.Tasks) != 10 {
		t.Errorf("small collection projected to %d children (partial=%v); want all 10, whole", len(got.Tasks), partial)
	}
	if !strings.Contains(LiveRegion(core.Snapshot{Collections: []core.TasksSnapshot{got}}, 24, 80, projectionEpoch, render.Style{Profile: txt.GlyphsUnicode}), "item-9") {
		t.Errorf("small collection frame lost a child")
	}
}
