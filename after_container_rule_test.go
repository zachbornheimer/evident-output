package evo

import (
	"context"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	afterContainerRuleSentence = "`After(group)` and `After(sequence)` wait for the members the container held when `After` was called and never consult the container's own later outcome, so a member declared afterward that fails leaves the container Failed and the dependent still runs; a container whose `Build` was refused counts as a Failed predecessor."
	afterContainerLateFailExit = 2
	afterContainerLateDelay    = 100 * time.Millisecond
)

// TestAfterContainerRuleMatchesContract pins the one CONTRACT.md sentence on After(group) and
// After(sequence) to the runtime: the edge is a snapshot of the members declared before the After call.
func TestAfterContainerRuleMatchesContract(t *testing.T) {
	body, err := os.ReadFile("CONTRACT.md")
	if err != nil {
		t.Fatal(err)
	}
	flat := strings.Join(strings.Fields(string(body)), " ")
	if n := strings.Count(flat, afterContainerRuleSentence); n != 1 {
		t.Fatalf("CONTRACT.md states the After(container) rule %d times, want once: %q", n, afterContainerRuleSentence)
	}

	var dependentRan atomic.Bool
	result := Execute(context.Background(), Config{Output: Plain, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard}},
		func(ctx context.Context, r *Run) error {
			g := r.Group("g")
			g.Task("a").Define(nopWork)
			r.Task("dependent").After(g).Define(func(context.Context) error {
				dependentRan.Store(true)
				return nil
			})
			g.Task("b").Define(func(context.Context) error {
				time.Sleep(afterContainerLateDelay)
				return Fail("b broke")
			})
			return nil
		})
	if !dependentRan.Load() {
		t.Error("dependent never ran: a member declared after After(g) gated the edge")
	}
	if got := result.ExitCode(); got != afterContainerLateFailExit {
		t.Errorf("exit code = %d, want %d (the late member failed)", got, afterContainerLateFailExit)
	}
}
