package live

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// testRun is a run a Board presents, with the smallest Source that answers
// it: each Task's truth, name and collection, and a header per collection.
type testRun struct {
	run   *record.Run
	board *Board
	tasks map[record.TaskID]*record.Task
	names map[record.TaskID]string
	in    map[record.TaskID]string
	group map[string]string
	ids   []record.TaskID
}

func newTestRun() *testRun {
	r := &testRun{
		run:   record.NewRun(),
		tasks: map[record.TaskID]*record.Task{},
		names: map[record.TaskID]string{},
		in:    map[record.TaskID]string{},
		group: map[string]string{},
	}
	r.board = NewBoard(r.run, r)
	return r
}

func (r *testRun) declareGroup(id, name, parent string) {
	r.group[id] = name
	r.board.DeclareGroup(id, name, parent)
}

func (r *testRun) declareTask(name, in string) record.TaskID {
	id := record.TaskID(fmt.Sprintf("task_%d", len(r.ids)+1))
	rec := r.run.NewTask(record.TaskInit{ID: id, State: record.Pending})
	r.tasks[id], r.names[id], r.in[id] = rec, name, in
	r.ids = append(r.ids, id)
	r.board.DeclareTask(id, name, rec, in)
	return id
}

func (r *testRun) Pull() {
	for _, id := range r.run.TakeChanged(nil) {
		r.board.Follow(id)
	}
}

func (r *testRun) View(id record.TaskID) record.TaskSnapshot {
	truth := r.tasks[id].Truth()
	base := record.TaskSnapshot{
		ID: string(id), Name: r.names[id], State: truth.State, Phase: truth.Phase,
		Progress: truth.Progress, Summary: truth.Summary, Problems: truth.Problems,
		Warnings: truth.Warnings, Skipped: truth.Skipped, Kept: truth.Kept, Collection: r.in[id],
	}
	return record.WithLiveTail(record.NewTaskSnapshot(base, r.board.FirstSeen(id), false), r.board.Tail(id))
}

func (r *testRun) Snapshot(id record.TaskID) record.TaskSnapshot { return r.View(id) }

func (r *testRun) Header(id string) record.TasksSnapshot {
	states := r.board.States(id)
	state := record.Succeeded
	switch {
	case states.Running > 0:
		state = record.Running
	case states.Failed > 0:
		state = record.Failed
	case states.Unresolved > 0:
		state = record.Incomplete
	}
	return record.TasksSnapshot{ID: id, Name: r.group[id], State: state}
}

func (r *testRun) Emitted(record.TaskID) bool         { return false }
func (r *testRun) HeldBack(*record.TaskSnapshot) bool { return false }

// walkFrame is the frame of g a walk of every child builds.
func walkFrame(g *group, rows int, now time.Time) record.TasksSnapshot {
	b := g.board
	children := NewLiveChildren(g.name, rows)
	for _, r := range g.tasks {
		if view := b.source.View(r.id); children.Admit(&view) {
			children.Keep(b.source.Snapshot(r.id))
		}
	}
	return children.Collection(b.collections(g.groups, rows, now).Into(b.source.Header(g.id)))
}

// TestChildIndexFrameEqualsAWalkOfEveryChild drives a nested run through
// random transitions and checks every frame a collection builds from its
// child index against the frame a walk of every child builds.
func TestChildIndexFrameEqualsAWalkOfEveryChild(t *testing.T) {
	const tasks, steps, rows = 80, 250, 12
	r := newTestRun()
	r.declareGroup("g_1", "items", "")
	r.declareGroup("g_2", "nested", "g_1")
	r.declareGroup("g_3", "other", "")
	groups := []string{"g_1", "g_2", "g_3", ""}
	rng := rand.New(rand.NewPCG(9, 1))
	for i := range tasks {
		r.declareTask(fmt.Sprintf("item %d", i), groups[rng.IntN(len(groups))])
	}
	audited := 0
	r.board.audit = func(g *group, rows int, now time.Time, got record.TasksSnapshot, rev uint64) {
		want := walkFrame(g, rows, now)
		if r.run.Revision() != rev {
			return
		}
		audited++
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("index frame of %q differs from a walk:\n got %+v\nwant %+v", g.name, got, want)
		}
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	states := []record.State{record.Running, record.Succeeded, record.Failed, record.Pending, record.NotStarted}
	for step := range steps {
		task := r.tasks[r.ids[rng.IntN(len(r.ids))]]
		switch rng.IntN(4) {
		case 0:
			task.SetPhase(fmt.Sprintf("phase %d", step))
		case 1:
			task.AppendWarning(record.Problem{Summary: "careful"})
		default:
			task.Transition(states[rng.IntN(len(states))])
		}
		now = now.Add(time.Second)
		r.board.Frame(rows, now)
	}
	if audited == 0 {
		t.Fatal("no frame was audited")
	}
}

// walkCensus counts every Task at or below g the slow way.
func walkCensus(g *group, c *census) {
	for _, r := range g.tasks {
		c.total++
		state := r.rec.State()
		c.count(state, 1)
		if r.rec.WarningCount() > 0 {
			c.warned++
		}
		if r.unstampedIn(state) {
			c.unstamped++
		}
		c.nameWidth = max(c.nameWidth, len([]rune(r.name)))
	}
	for _, child := range g.groups {
		walkCensus(child, c)
	}
}

// TestCensusFollowsAWriteTheBoardWasNotToldOf proves a read of the census
// finds a Task that settled without Follow being called: the census pulls
// the record, not the notification.
func TestCensusFollowsAWriteTheBoardWasNotToldOf(t *testing.T) {
	r := newTestRun()
	r.declareGroup("g_1", "items", "")
	first := r.declareTask("first", "g_1")
	r.declareTask("second", "g_1")
	r.board.Frame(24, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))

	r.tasks[first].Settle(record.Succeeded)

	g := r.board.nodes["g_1"]
	got := *g.currentCensus()
	var want census
	walkCensus(g, &want)
	want.earliestSeen = got.earliestSeen
	if got != want {
		t.Errorf("census = %+v, a walk counts %+v", got, want)
	}
	if got.done != 1 {
		t.Errorf("census counts %d Done, want the settled Task counted", got.done)
	}
}
