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
