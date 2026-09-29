package engine

import (
	"io"
	"strings"
	"testing"
)

// TestAfterCycle_PlainRowsStateTheCycleOnce pins E-089: each Task in an
// After cycle stated the cycle on its row and again on a child line under
// its own name, like a nested Task. Like an ordinary Block, the reason
// belongs on the row once, plus the single run-level misuse line.
func TestAfterCycle_PlainRowsStateTheCycleOnce(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{
		Isolated: true, Plain: true, Color: ColorNever, Title: "job",
		Stdout: &buf, Stderr: io.Discard,
	})
	s := out.Sequence("s")
	a, b := s.Task("a"), s.Task("b")
	a.After(b).Define(nopWork)
	b.Define(nopWork)
	_ = closeWithin(t, out)
	got := buf.String()
	const reason = "dependency cycle: a → b → a"
	for _, row := range []string{"⊘ a  " + reason, "⊘ b  " + reason} {
		if !strings.Contains(got, row) {
			t.Errorf("output lacks row %q:\n%s", row, got)
		}
	}
	if n := strings.Count(got, reason); n != 2 {
		t.Errorf("%q appears %d times; want once per cycle row (2):\n%s", reason, n, got)
	}
	if n := strings.Count(got, "a → b → a"); n != 3 {
		t.Errorf("cycle path appears %d times; want two rows plus one misuse line:\n%s", n, got)
	}
}
