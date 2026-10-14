package plain

import (
	"strings"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// blockLens is the row model's view of a run with nothing live.
type blockLens struct{ rows *project.Rows }

func (blockLens) FirstSeen(record.TaskID) time.Time           { return time.Time{} }
func (blockLens) Tail(record.TaskID) record.LiveTail          { return record.LiveTail{} }
func (blockLens) Tracked(record.TaskID) record.TaskOperations { return record.TaskOperations{} }

func (l blockLens) States(group string) project.StateCounts {
	var c project.StateCounts
	for _, t := range l.rows.Group(group).Tasks {
		switch t.Rec.State() {
		case record.Running:
			c.Running++
		case record.Failed:
			c.Failed++
		case record.Succeeded, record.Satisfied:
		default:
			c.Unresolved++
		}
	}
	return c
}

// blockScene is a run that never ends, on a terminal of the default width.
type blockScene struct{}

func (blockScene) Ended() bool { return false }
func (blockScene) Width() int  { return project.DefaultWidth }

// blockNode is a declaration whose membership closes when its flag is set.
type blockNode struct {
	key    string
	closed *bool
}

func (n blockNode) Key() string            { return n.key }
func (blockNode) HasBuilder() bool         { return false }
func (blockNode) BuilderFailed() bool      { return false }
func (blockNode) BuilderNotStarted() bool  { return false }
func (n blockNode) MembershipClosed() bool { return *n.closed }

// blockRun is a Stream over the real row model, so the block it writes is
// the block a run would.
type blockRun struct {
	t      *testing.T
	run    *record.Run
	rows   *project.Rows
	stream *Stream
	sinks  *bufferSinks
	closed map[string]*bool
}

func newBlockRun(t *testing.T) *blockRun {
	t.Helper()
	lens := &blockLens{}
	b := &blockRun{t: t, run: record.NewRun(), rows: project.NewRows(lens), sinks: &bufferSinks{}, closed: map[string]*bool{}}
	lens.rows = b.rows
	b.stream = NewStream(Config{Run: b.run, Region: &quietRegion{}, Source: b.rows, Sinks: b.sinks, Scene: blockScene{}})
	return b
}

// sequence declares a Sequence id at the root; setting *closed ends its
// membership, as sealing it or draining the run does.
func (b *blockRun) sequence(id string) (closed *bool) { return b.collection(id, true) }

func (b *blockRun) collection(id string, sequential bool) (closed *bool) {
	return b.collectionIn(id, "", sequential)
}

// collectionIn declares the collection id inside parent, or at the root when "".
func (b *blockRun) collectionIn(id, parent string, sequential bool) (closed *bool) {
	closed = new(bool)
	b.closed[id] = closed
	b.rows.DeclareGroup(project.DeclaredGroup{
		ID: id, Name: id, Sequential: sequential, Rec: b.run.NewContainer(), Node: blockNode{key: "group:" + id, closed: closed},
	}, parent)
	b.stream.DeclareGroup(DeclaredGroup{ID: id, Name: id, Parent: parent, Sequential: sequential})
	return closed
}

// task declares Task id running, in collection in or at the root when "". It
// has run its Define, so a Succeeded row of it says something.
func (b *blockRun) task(id, in string) { b.declare(id, in, true) }

// noopTask declares a Task that never ran a Define: settled Satisfied, its row
// is one human output may hide.
func (b *blockRun) noopTask(id, in string) { b.declare(id, in, false) }

func (b *blockRun) declare(id, in string, defineRan bool) {
	rec := b.run.NewTask(record.TaskInit{ID: record.TaskID(id), State: record.Running, DefineRan: defineRan})
	row := b.rows.DeclareTask(project.DeclaredTask{ID: record.TaskID(id), Node: blockNode{key: "task:" + id}, Name: id, Rec: rec}, in)
	declared := Declared{ID: row.ID, Name: id, Rec: rec}
	if row.Group != nil {
		declared.Group, declared.GroupID, declared.InGroup = row.Group.Name, row.Group.ID, true
	}
	b.stream.Declare(declared)
}

// settle resolves Task id in state and commits it, as the run's host does.
func (b *blockRun) settle(id string, state record.State) {
	b.rows.Task(record.TaskID(id)).Rec.Settle(state)
	b.stream.Commit(record.TaskID(id))
}

func (b *blockRun) written() string { return b.sinks.human.String() }

// finish is what Finish still owes the human stream.
func (b *blockRun) finish() string {
	plain, _ := b.stream.Residual(b.rows.Into(record.Snapshot{}))
	return plain
}

// inOrder reports that every needle appears in got, in order.
func inOrder(got string, needles ...string) bool {
	at := 0
	for _, n := range needles {
		i := strings.Index(got[at:], n)
		if i < 0 {
			return false
		}
		at += i + len(n)
	}
	return true
}

// TestPlainCommitsContainerRowsWhenSettled pins the order of a plain log: a
// collection writes as one block, header and members, and its rows never
// wait for Finish or land under a row of a Task that ran After it. Members
// write as they settle (TestPlainStreamsSettledMembersOfAnOpenGroup), so a
// check that the block is "not yet" written asserts what is still unsettled.
func TestPlainCommitsContainerRowsWhenSettled(t *testing.T) {
	t.Run("the block is written before a dependent and before Finish", func(t *testing.T) {
		b := newBlockRun(t)
		*b.sequence("steps") = true
		b.task("one", "steps")
		b.task("two", "steps")
		b.task("after", "")

		b.settle("one", record.Succeeded)
		if got := b.written(); !inOrder(got, "steps", "one") || strings.Contains(got, "two") {
			t.Fatalf("want the header and the settled member only, got %q", got)
		}
		b.settle("two", record.Succeeded)
		if !inOrder(b.written(), "steps", "one", "two") {
			t.Fatalf("the settled block is not written whole, header first:\n%s", b.written())
		}
		b.settle("after", record.Succeeded)

		if got := b.written(); !inOrder(got, "two", "after") {
			t.Fatalf("the dependent's row was written above the collection it ran after:\n%s", got)
		}
		if rest := b.finish(); strings.Contains(rest, "one") || strings.Contains(rest, "steps") {
			t.Fatalf("Finish repeats a block that already streamed:\n%s", rest)
		}
	})

	t.Run("rows are written while another root Task is still running", func(t *testing.T) {
		b := newBlockRun(t)
		b.task("slow", "")
		*b.sequence("steps") = true
		b.task("one", "steps")
		b.settle("one", record.Succeeded)

		if got := b.written(); !strings.Contains(got, "one") || strings.Contains(got, "slow") {
			t.Fatalf("want the settled block and nothing for the Running Task, got:\n%s", got)
		}
	})

	t.Run("a block waits for its membership to close, then writes ahead of the next row", func(t *testing.T) {
		b := newBlockRun(t)
		closed := b.sequence("steps")
		b.task("one", "steps")
		b.task("after", "")

		b.settle("one", record.Succeeded)
		if got := b.written(); strings.Count(got, "steps") != 1 {
			t.Fatalf("a collection still open to new members closed: %q", got)
		}
		*closed = true
		b.settle("after", record.Succeeded)

		if got := b.written(); !inOrder(got, "steps", "one", "after") {
			t.Fatalf("want the block ahead of the row written next, got:\n%s", got)
		}
	})

	t.Run("a Task starting writes the collection it followed", func(t *testing.T) {
		b := newBlockRun(t)
		closed := b.sequence("steps")
		b.task("one", "steps")
		b.settle("one", record.Succeeded)
		*closed = true

		b.stream.Started("after", time.Time{})

		if got := b.written(); !strings.Contains(got, "one") {
			t.Fatalf("a closed, settled collection stayed silent when the next Task started:\n%s", got)
		}
	})
}

// TestPlainLeavesPossiblyHiddenContainerRowsToFinish: whether a no-op row is
// hidden turns on the rest of the run, so its block is not written early.
func TestPlainLeavesPossiblyHiddenContainerRowsToFinish(t *testing.T) {
	b := newBlockRun(t)
	*b.collection("pool", false) = true
	b.noopTask("noop", "pool")
	b.settle("noop", record.Satisfied)

	if got := b.written(); got != "" {
		t.Fatalf("a block holding a row Finish may hide was written early:\n%s", got)
	}
}

// TestPlainWritesTheBlockAgainWhenAMemberIsOverturned: a member that settles
// again in another state changes what the block says, so scrollback tells
// both, as it does for a root Task.
func TestPlainWritesTheBlockAgainWhenAMemberIsOverturned(t *testing.T) {
	b := newBlockRun(t)
	*b.sequence("steps") = true
	b.task("one", "steps")
	b.settle("one", record.Succeeded)
	first := b.written()

	b.settle("one", record.Failed)

	if got := strings.TrimPrefix(b.written(), first); !strings.Contains(got, "one") {
		t.Fatalf("an overturned member was not written again:\n%s", b.written())
	}
	if rest := b.finish(); strings.Contains(rest, "one") {
		t.Fatalf("Finish repeats the overturned block:\n%s", rest)
	}
}
