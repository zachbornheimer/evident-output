package evo

import (
	"testing"
)

// A builder ordered After a Task can only run once that Task settled, so a
// Wait on it is no wait at all and is answered.
func TestSlice36_WaitInABuilderOnATaskItIsOrderedAfterIsAnswered(t *testing.T) {
	out := quietOutput(Config{MaxConcurrency: 4})
	t.Cleanup(func() { _ = out.close() })
	before := out.Task("before").Define(noop)
	var waitErr error
	out.Group("builder").After(before).define(func(g *Group) {
		waitErr = before.wait()
		g.Task("child").Define(noop)
	})

	_ = waitWithin(t, "Finish", out.finish)

	if waitErr != nil {
		t.Errorf("Wait inside the builder on its own predecessor = %v, want nil", waitErr)
	}
	if misuse := out.firstMisuse(); misuse != nil {
		t.Errorf("recorded misuse = %v, want none", misuse)
	}
}
