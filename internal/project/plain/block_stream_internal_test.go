package plain

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// TestPlainStreamsSettledMembersOfAnOpenGroup pins the durable stream's
// promise that a verdict reaches the log when it exists: a Group's header
// opens when its first member settles, each member follows as it settles,
// and the block closes with the container's own verdict. A kill -9 between
// two members loses only the members still running.
func TestPlainStreamsSettledMembersOfAnOpenGroup(t *testing.T) {
	t.Run("members stream under a header opened by the first", func(t *testing.T) {
		b := newBlockRun(t)
		b.collection("sg", false)
		b.task("a", "sg")
		b.task("b", "sg")
		b.task("c", "sg")

		b.settle("a", record.Succeeded)
		if got := b.written(); !inOrder(got, "sg", "a") || strings.Contains(got, "b") {
			t.Fatalf("the first settled member did not open the block alone:\n%s", got)
		}
		b.settle("b", record.Succeeded)
		if got := b.written(); strings.Count(got, "sg") != 1 || !inOrder(got, "sg", "a", "b") || strings.Contains(got, "c") {
			t.Fatalf("the second member did not stream under the same header:\n%s", got)
		}
	})

	t.Run("the block closes with the container's verdict once it settles", func(t *testing.T) {
		b := newBlockRun(t)
		closed := b.collection("sg", false)
		b.task("a", "sg")
		b.task("b", "sg")
		b.settle("a", record.Succeeded)
		b.settle("b", record.Failed)
		if got := b.written(); strings.Count(got, "sg") != 1 {
			t.Fatalf("a block with an unclosed membership already closed:\n%s", got)
		}
		*closed = true
		b.stream.CommitSettledBlocks()

		got := b.written()
		if strings.Count(got, "sg") != 2 || !inOrder(got, "sg", "a", "b", "sg") {
			t.Fatalf("want header, members, then the container's own verdict row:\n%s", got)
		}
		if rest := b.finish(); strings.Contains(rest, "sg") || strings.Contains(rest, "a ") {
			t.Fatalf("Finish repeats a block that streamed whole:\n%s", rest)
		}
	})

	t.Run("a root row waits for the open block so cause precedes effect", func(t *testing.T) {
		b := newBlockRun(t)
		closed := b.collection("sg", false)
		b.task("a", "sg")
		b.task("c", "sg")
		b.task("after", "")
		b.after("after", nil, "sg")
		b.settle("a", record.Succeeded)
		b.settle("after", record.Succeeded)
		if got := b.written(); strings.Contains(got, "after") {
			t.Fatalf("a row settled inside an open block and landed in it:\n%s", got)
		}

		*closed = true
		b.settle("c", record.Succeeded)

		if got := b.written(); !inOrder(got, "sg", "a", "c", "after") {
			t.Fatalf("the held row did not follow the block it waited for:\n%s", got)
		}
	})

	t.Run("a nested group streams inside its parent's block", func(t *testing.T) {
		b := newBlockRun(t)
		b.collection("outer", false)
		b.collectionIn("inner", "outer", false)
		b.task("slow", "outer")
		b.task("i1", "inner")

		b.settle("i1", record.Succeeded)

		got := b.written()
		if !inOrder(got, "outer\n", "   ✓ inner\n", "      ✓ i1\n") || strings.Contains(got, "slow") {
			t.Fatalf("the nested member did not stream under both headers:\n%s", got)
		}
	})
}

// graceClock is a clock whose timers fire when the test says.
type graceClock struct{ timers []func() }

func (*graceClock) Now() time.Time { return time.Time{} }
func (c *graceClock) AfterFunc(_ time.Duration, fn func()) func() {
	c.timers = append(c.timers, fn)
	return func() {}
}

// TestPlainWritesAGroupThatSettlesInsideTheGraceWhole: a Group whose members
// settle together is one block as it ends, not a header, a trickle and a
// close; only one still open when the grace passes streams.
func TestPlainWritesAGroupThatSettlesInsideTheGraceWhole(t *testing.T) {
	newRun := func() (*blockRun, *graceClock, *bool) {
		b, clk := newBlockRun(t), &graceClock{}
		b.stream.cfg.Clock, b.stream.cfg.Lock = clk, &sync.Mutex{}
		return b, clk, b.collection("sg", false)
	}

	t.Run("settling inside the grace writes the block whole", func(t *testing.T) {
		b, clk, closed := newRun()
		b.task("a", "sg")
		b.task("b", "sg")
		b.settle("a", record.Succeeded)
		if got := b.written(); got != "" {
			t.Fatalf("a member inside the grace wrote %q", got)
		}
		*closed = true
		b.settle("b", record.Succeeded)

		if got := b.written(); got != "✓ sg\n   ✓ a\n   ✓ b\n" {
			t.Fatalf("want the block as it ends, got:\n%s", got)
		}
		for _, fire := range clk.timers {
			fire()
		}
		if got := b.written(); strings.Count(got, "sg") != 1 {
			t.Fatalf("the grace timer wrote a block that had already written:\n%s", got)
		}
	})

	t.Run("a block still open when the grace passes streams what settled", func(t *testing.T) {
		b, clk, _ := newRun()
		b.task("a", "sg")
		b.task("b", "sg")
		b.settle("a", record.Succeeded)
		clk.timers[0]()

		if got := b.written(); !inOrder(got, "sg", "a") || strings.Contains(got, "b") {
			t.Fatalf("want the header and the member that settled, got:\n%s", got)
		}
	})
}

// TestPlainWritesNestedGroupBeforeItsDependent: a Task wired After a Group
// nested in another prints below the block that explains it, once that block
// closes. A Task wired to nothing in a block never waits for one.
func TestPlainWritesNestedGroupBeforeItsDependent(t *testing.T) {
	setup := func(t *testing.T) (*blockRun, *bool) {
		b := newBlockRun(t)
		closed := b.collection("outer", false)
		*b.collectionIn("inner", "outer", false) = true
		b.task("slow", "outer")
		b.task("i1", "inner")
		b.task("dep", "")
		b.task("alone", "")
		b.after("dep", nil, "inner")
		return b, closed
	}

	t.Run("the dependent waits for the block holding its predecessor", func(t *testing.T) {
		b, _ := setup(t)
		b.settle("dep", record.Succeeded)
		if got := b.written(); strings.Contains(got, "dep") {
			t.Fatalf("the dependent printed above the block that explains it:\n%s", got)
		}

		b.settle("i1", record.Succeeded)

		if got := b.written(); !inOrder(got, "inner", "i1", "dep") {
			t.Fatalf("want the block, then its dependent, got:\n%s", got)
		}
	})

	t.Run("an unrelated row does not wait", func(t *testing.T) {
		b, _ := setup(t)
		b.settle("i1", record.Succeeded)
		b.settle("alone", record.Succeeded)

		if got := b.written(); !strings.Contains(got, "alone") {
			t.Fatalf("a row with no predecessor in an open block waited:\n%s", got)
		}
	})

	t.Run("a row After a held row waits for it too", func(t *testing.T) {
		b, _ := setup(t)
		b.after("alone", []record.TaskID{"dep"})
		b.settle("dep", record.Succeeded)
		b.settle("alone", record.Succeeded)
		if got := b.written(); strings.Contains(got, "alone") || strings.Contains(got, "dep") {
			t.Fatalf("a dependent printed before the block that explains it:\n%s", got)
		}
		b.settle("i1", record.Succeeded)

		if got := b.written(); !inOrder(got, "i1", "dep", "alone") {
			t.Fatalf("want the dependent's own dependent after it, got:\n%s", got)
		}
	})
}

// TestNotStartedDependentWritesWhenItsCauseIsDurable: a Task that never ran
// because a member of a Group was Excluded settles the moment that member
// does, with no verb of its own to commit it. Its row follows the block that
// holds its cause as soon as the block closes, not at Finish.
func TestNotStartedDependentWritesWhenItsCauseIsDurable(t *testing.T) {
	b := newBlockRun(t)
	closed := b.collection("eg", false)
	b.task("ex", "eg")
	b.task("slow", "eg")
	b.task("depEx", "")
	b.after("depEx", []record.TaskID{"ex"})

	b.settle("ex", record.Excluded)
	// The scheduler's cascade settles the dependent; only the follower's
	// Settled answer reaches the stream.
	b.rows.Task("depEx").Rec.Settle(record.NotStarted)
	b.stream.Settled("depEx")
	if got := b.written(); strings.Contains(got, "depEx") {
		t.Fatalf("the dependent printed above the block that holds its cause:\n%s", got)
	}

	*closed = true
	b.settle("slow", record.Succeeded)

	if got := b.written(); !inOrder(got, "eg", "ex", "slow", "depEx") {
		t.Fatalf("want the block, then its NotStarted dependent, as the block closes; got:\n%s", got)
	}
	if rest := b.finish(); strings.Contains(rest, "depEx") {
		t.Fatalf("Finish still owed the dependent's row:\n%s", rest)
	}
}

// TestOwnTaskFailureStreamsImmediately: a Group's own Task that fails is a
// verdict a kill -9 must not lose, so it writes as a member row the moment
// it settles, and the closing row keeps the Group's verdict. An own Task that
// succeeds says nothing a close would lose and still folds into that row.
func TestOwnTaskFailureStreamsImmediately(t *testing.T) {
	t.Run("a failed own Task writes as a member when it settles", func(t *testing.T) {
		b := newBlockRun(t)
		closed := b.collection("pkg", false)
		b.task("pkg", "pkg")
		b.task("slow", "pkg")
		b.rows.Task("pkg").Rec.SetSummary("own broke")

		b.settle("pkg", record.Failed)

		if got := b.written(); !inOrder(got, "pkg\n", "   ✗ pkg  own broke\n") || strings.Contains(got, "slow") {
			t.Fatalf("the own Task's failure did not stream under the header:\n%s", got)
		}
		*closed = true
		b.settle("slow", record.Succeeded)

		got := b.written()
		if !inOrder(got, "   ✗ pkg  own broke\n", "   ✓ slow\n", "✗ pkg\n") || strings.Count(got, "own broke") != 1 {
			t.Fatalf("want the member row, its sibling, then the Group's verdict once:\n%s", got)
		}
		if rest := b.finish(); strings.Contains(rest, "own broke") {
			t.Fatalf("Finish repeats the own Task's failure:\n%s", rest)
		}
	})

	t.Run("a succeeded own Task still folds into the closing row", func(t *testing.T) {
		b := newBlockRun(t)
		closed := b.collection("pkg", false)
		b.task("pkg", "pkg")
		b.task("slow", "pkg")
		b.settle("pkg", record.Succeeded)
		if got := b.written(); got != "" {
			t.Fatalf("a succeeded own Task wrote before the block did:\n%s", got)
		}
		b.settle("slow", record.Succeeded)
		*closed = true
		b.stream.CommitSettledBlocks()

		if got := b.written(); got != "✓ pkg\n   ✓ slow\n" {
			t.Fatalf("want the own Task folded into the Group's row, got:\n%s", got)
		}
	})
}

// TestHeldRowFollowsItsNestedBlockNotTheRoot: a root row that ran After a
// block nested in another is written when that nested block closes, not when
// the root block does, so a kill -9 while a slow sibling runs loses no verdict
// that already existed. It still never prints above the block that explains it.
func TestHeldRowFollowsItsNestedBlockNotTheRoot(t *testing.T) {
	setup := func(t *testing.T) (b *blockRun, innerClosed, outerClosed *bool) {
		b = newBlockRun(t)
		outerClosed = b.collection("outer", false)
		innerClosed = b.collectionIn("inner", "outer", false)
		b.task("i1", "inner")
		b.task("i2", "inner")
		b.task("slow", "outer")
		b.task("dep", "")
		b.after("dep", nil, "inner")
		b.task("taskDep", "")
		b.after("taskDep", []record.TaskID{"i1"})
		return b, innerClosed, outerClosed
	}

	t.Run("the nested block closing releases the row while the root stays open", func(t *testing.T) {
		b, innerClosed, outerClosed := setup(t)
		b.settle("dep", record.Succeeded)
		b.settle("i1", record.Succeeded)
		if got := b.written(); strings.Contains(got, "dep") {
			t.Fatalf("the row printed before its nested block closed:\n%s", got)
		}

		*innerClosed = true
		b.settle("i2", record.Succeeded)

		got := b.written()
		if !inOrder(got, "outer", "inner", "i1", "i2", "dep") || strings.Contains(got, "slow") {
			t.Fatalf("want the nested block, then the row, with the root still open; got:\n%s", got)
		}

		*outerClosed = true
		b.settle("slow", record.Succeeded)
		if got := b.written(); strings.Count(got, "dep") != 1 || !inOrder(got, "dep", "slow") {
			t.Fatalf("the root closing wrote the row again or out of order:\n%s", got)
		}
	})

	t.Run("a row After a Task waits for the block holding that Task", func(t *testing.T) {
		b, innerClosed, _ := setup(t)
		b.settle("taskDep", record.Succeeded)
		b.settle("i1", record.Succeeded)
		if got := b.written(); strings.Contains(got, "taskDep") {
			t.Fatalf("the row printed while its Task's block was open:\n%s", got)
		}

		*innerClosed = true
		b.settle("i2", record.Succeeded)

		if got := b.written(); !inOrder(got, "i2", "taskDep") || strings.Contains(got, "slow") {
			t.Fatalf("want the row once the Task's block closed; got:\n%s", got)
		}
	})

	t.Run("a root that writes whole still holds the row until it is written", func(t *testing.T) {
		b := newBlockRun(t)
		clk := &graceClock{}
		b.stream.cfg.Clock, b.stream.cfg.Lock = clk, &sync.Mutex{}
		outerClosed := b.collection("outer", false)
		innerClosed := b.collectionIn("inner", "outer", false)
		b.task("i1", "inner")
		b.task("slow", "outer")
		b.task("dep", "")
		b.after("dep", nil, "inner")

		*innerClosed = true
		b.settle("i1", record.Succeeded)
		b.settle("dep", record.Succeeded)
		if got := b.written(); got != "" {
			t.Fatalf("a block inside its grace wrote %q", got)
		}

		*outerClosed = true
		b.settle("slow", record.Succeeded)

		if got := b.written(); !inOrder(got, "outer", "slow", "inner", "i1", "dep") {
			t.Fatalf("want the block whole, then the row; got:\n%s", got)
		}
	})
}
