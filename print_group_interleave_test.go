package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestPrintln_StreamsAheadOfStillRunningGroup is the RED-then-GREEN
// regression for the ordering defect closed in this slice:
// hasPendingCollectionRowsLocked (internal/engine/progressive.go) used to
// hold back every plain-mode Println/Printf call once a run had declared
// any Group/Sequence at all, even one wholly unrelated to the call and
// still Running. That inverted the P2 "interleave by call time" contract
// (residualPlainLocked's doc comment): a Println made chronologically
// before a collection settles must stream immediately, exactly as a
// standalone Task's progressive row does, not wait behind that collection's
// eventual Finish-time render.
//
// Probe: Group g, Task a (standalone) done, Println("msg1"), Task b
// (standalone) done, g.Task(x) done, Finish. At the regressed HEAD, msg1 was
// not in the buffer right after the Println call (it only appeared at
// Finish, after g's row), and the final transcript read a, b, x, msg1
// instead of a, msg1, b, x. This test pins the correct call-time order.
func TestPrintln_StreamsAheadOfStillRunningGroup(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Color: evo.ColorNever, Plain: true, Title: "probe",
		Stdout: &buf,
	})
	t.Cleanup(func() { _ = out.Close() })

	g := out.Group("g")
	x := g.Task("x")

	succeed(out.Task("a"))
	out.Println("msg1")

	afterMsg := buf.String()
	if !bytes.Contains([]byte(afterMsg), []byte("msg1")) {
		t.Fatalf("msg1 did not stream immediately at call time; buffer so far:\n%s", afterMsg)
	}

	succeed(out.Task("b"))
	succeed(x)

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	msgIdx := strings.Index(got, "msg1")
	bIdx := strings.Index(got, "✓ b")
	xIdx := strings.Index(got, "✓ x")
	if msgIdx < 0 || bIdx < 0 || xIdx < 0 {
		t.Fatalf("expected a, msg1, b, x all present; got:\n%s", got)
	}
	if msgIdx >= bIdx || bIdx >= xIdx {
		t.Fatalf("expected call-time order msg1 < b < x; got:\n%s", got)
	}
}

// TestPrintln_HeldBehindSettledGroupStaysAheadOfLaterTask is the
// RED-then-GREEN regression for the companion defect the review round
// found in this slice's first attempt: once a Println is actually held
// back (hasPendingCollectionRowsLocked is true and the call chronologically
// follows a collection that has already settled), a standalone Task
// resolved after that Println must not stream immediately and print ahead
// of it — that inverts the same P2 "interleave by call time" contract this
// package's other test protects, just on the other side of the settle
// point. A standalone Task by itself still jumps ahead of a settled
// collection's own Finish-only row (TestV8_Stress pins that), but once a
// message is waiting on that same collection, a later Task must wait with
// it (commitResolvedTaskLocked's doc comment).
//
// Probe: Group g, g.Task(x) done (settles g), Println("msg-after-x"),
// standalone Task b done, Println("msg-after-b"), Finish. At the regression
// this test guards against, "b" streamed the instant it resolved and
// printed ahead of "msg-after-x" even though it resolved after that call.
func TestPrintln_HeldBehindSettledGroupStaysAheadOfLaterTask(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Color: evo.ColorNever, Plain: true, Title: "probe",
		Stdout: &buf,
	})
	t.Cleanup(func() { _ = out.Close() })

	g := out.Group("g")
	x := g.Task("x")
	succeed(x)
	out.Println("msg-after-x")
	succeed(out.Task("b"))
	out.Println("msg-after-b")

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	got := buf.String()
	xIdx := strings.Index(got, "✓ x")
	msg1Idx := strings.Index(got, "msg-after-x")
	bIdx := strings.Index(got, "✓ b")
	msg2Idx := strings.Index(got, "msg-after-b")
	if xIdx < 0 || msg1Idx < 0 || bIdx < 0 || msg2Idx < 0 {
		t.Fatalf("expected x, msg-after-x, b, msg-after-b all present; got:\n%s", got)
	}
	if xIdx >= msg1Idx || msg1Idx >= bIdx || bIdx >= msg2Idx {
		t.Fatalf("expected call-time order x < msg-after-x < b < msg-after-b; got:\n%s", got)
	}
}
