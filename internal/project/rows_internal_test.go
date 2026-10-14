package project

import (
	"sync"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/record"
)

// countingLens counts each collection's direct Tasks from their records, the
// slow way a live board's index must agree with.
type countingLens struct{ rows *Rows }

func (l countingLens) FirstSeen(record.TaskID) time.Time           { return time.Time{} }
func (l countingLens) Tail(record.TaskID) record.LiveTail          { return record.LiveTail{} }
func (l countingLens) Tracked(record.TaskID) record.TaskOperations { return record.TaskOperations{} }

func (l countingLens) States(group string) StateCounts {
	var c StateCounts
	for _, t := range l.rows.Group(group).Tasks {
		switch t.Rec.State() {
		case record.Running:
			c.Running++
		case record.Failed:
			c.Failed++
		case record.NotStarted:
			c.NotStarted++
		case record.Succeeded:
		default:
			c.Unresolved++
		}
		if t.Rec.WarningCount() > 0 {
			c.Warned++
		}
	}
	return c
}

// node is a declaration with a fixed key and a builder that ran.
type node string

func (n node) Key() string           { return string(n) }
func (node) BuilderFailed() bool     { return false }
func (node) BuilderNotStarted() bool { return false }

func newTestRows() (*Rows, *record.Run) {
	run := record.NewRun()
	lens := &countingLens{}
	rows := NewRows(lens)
	lens.rows = rows
	return rows, run
}

func declareTask(rows *Rows, run *record.Run, id, in string, state record.State) *DeclaredTask {
	rec := run.NewTask(record.TaskInit{ID: record.TaskID(id), State: state})
	return rows.DeclareTask(DeclaredTask{ID: record.TaskID(id), Node: node("task:" + id), Name: id, Rec: rec}, in)
}

// TestNestedFailureSurfacesAtTheRootHeader proves a collection's verdict
// folds its nested collections as if each were one more Task.
func TestNestedFailureSurfacesAtTheRootHeader(t *testing.T) {
	rows, run := newTestRows()
	for _, g := range []struct{ id, parent string }{{"outer", ""}, {"inner", "outer"}} {
		rows.DeclareGroup(DeclaredGroup{ID: g.id, Name: g.id, Rec: run.NewContainer(), Node: node(g.id)}, g.parent)
	}
	declareTask(rows, run, "ok", "outer", record.Succeeded)
	declareTask(rows, run, "broke", "inner", record.Failed)

	if got := rows.Header("outer").State; got != record.Failed {
		t.Fatalf("outer header = %s, want the nested failure", got)
	}
	snap := rows.Into(record.Snapshot{})
	if len(snap.Collections) != 1 || len(snap.Collections[0].Collections) != 1 {
		t.Fatalf("snapshot collections = %+v, want outer holding inner", snap.Collections)
	}
}

// TestSuccessSummaryHidesUnderAWarning proves a collection's own summary
// does not paper over a warning beneath it.
func TestSuccessSummaryHidesUnderAWarning(t *testing.T) {
	rows, run := newTestRows()
	group := run.NewContainer()
	group.SetSummary("all clean")
	rows.DeclareGroup(DeclaredGroup{ID: "g", Name: "g", Rec: group, Node: node("g")}, "")
	task := declareTask(rows, run, "careful", "g", record.Succeeded)

	if got := rows.Header("g").Summary; got != "all clean" {
		t.Fatalf("summary = %q before any warning, want it shown", got)
	}
	task.Rec.AppendWarning(record.Problem{Summary: "heads up"})
	if got := rows.Header("g").Summary; got != "" {
		t.Fatalf("summary = %q under a warning, want it hidden", got)
	}
}

// answerLog records the settles a Follower answered; it owes each Task
// one answer.
type answerLog struct {
	answered map[record.TaskID]int
	owed     map[record.TaskID]bool
}

func (a *answerLog) Changed(id record.TaskID) (record.State, bool) {
	if a.owed[id] {
		return "", false
	}
	a.owed[id] = true
	return record.Succeeded, true
}

func (a *answerLog) Settled(id record.TaskID, _ record.State) { a.answered[id]++ }

// TestFollowerAnswersEachSettleOnce proves a settle the follower already
// answered under the lock costs the listener nothing, and is never answered
// twice.
func TestFollowerAnswersEachSettleOnce(t *testing.T) {
	run := record.NewRun()
	log := &answerLog{answered: map[record.TaskID]int{}, owed: map[record.TaskID]bool{}}
	var mu sync.Mutex
	follower := NewFollower(run, &mu, log)
	run.Listen(follower)
	task := run.NewTask(record.TaskInit{ID: "t", State: record.Running})

	task.Settle(record.Succeeded)
	mu.Lock()
	follower.Follow()
	mu.Unlock()

	if got := log.answered["t"]; got != 1 {
		t.Fatalf("settle answered %d times, want once", got)
	}
}
