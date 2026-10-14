package evo

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/misuse"
)

func nopWork(context.Context) error { return nil }

func newGraphTestOutput(t *testing.T) *Run {
	t.Helper()
	return initRun(Config{stateDir: t.TempDir(), stdout: io.Discard, stderr: io.Discard})
}

// closeWithin runs Finish and Close and fails the test, instead of hanging
// it, when they do not return in time. It returns Finish's error.
func closeWithin(t *testing.T, out *Run) error {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		err := out.finish()
		_ = out.close()
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(waitOutcomeTimeout):
		t.Fatal("Finish hung: the run never settled its queued work")
		return nil
	}
}

// TestAfterCycle_SettlesBlockedInsteadOfHanging pins the After-cycle hang:
// each Task waits for something that waits for it, so none can ever start,
// and Finish used to wait on them forever without printing a line.
func TestAfterCycle_SettlesBlockedInsteadOfHanging(t *testing.T) {
	cases := map[string]struct {
		build func(o *Run) []*Task
		path  string
	}{
		"self": {
			build: func(o *Run) []*Task {
				x := o.Task("x")
				x.After(x).Define(nopWork)
				return []*Task{x}
			},
			path: "x → x",
		},
		"pair": {
			build: func(o *Run) []*Task {
				a, b := o.Task("a"), o.Task("b")
				a.After(b)
				b.After(a)
				a.Define(nopWork)
				b.Define(nopWork)
				return []*Task{a, b}
			},
			path: "a → b → a",
		},
		"own group": {
			build: func(o *Run) []*Task {
				g := o.Group("g")
				x := g.Task("x")
				x.After(g).Define(nopWork)
				return []*Task{x}
			},
			path: "x → g → x",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			out := newGraphTestOutput(t)
			tasks := tc.build(out)
			err := closeWithin(t, out)
			if !errors.Is(err, misuse.ErrDependencyCycle) {
				t.Fatalf("Finish = %v, want misuse.ErrDependencyCycle", err)
			}
			for _, task := range tasks {
				snap := task.snapshot()
				if snap.State != refused || !strings.Contains(snap.Summary, tc.path) {
					t.Errorf("%s = %s %q, want blocked naming %q", snap.Name, snap.State, snap.Summary, tc.path)
				}
			}
		})
	}
}

// TestAfterCycle_DependentsDoNotStart proves the cycle's rows are the only
// Blocked ones: a Task after the cycle never started, and says so.
func TestAfterCycle_DependentsDoNotStart(t *testing.T) {
	out := newGraphTestOutput(t)
	a, b := out.Task("a"), out.Task("b")
	a.After(b)
	b.After(a)
	a.Define(nopWork)
	b.Define(nopWork)
	after := out.Task("after").After(a)
	after.Define(nopWork)
	_ = closeWithin(t, out)
	if got := after.snapshot().State; got != notStarted {
		t.Fatalf("dependent of a cycle = %s, want not_started", got)
	}
}

// TestAfterGroupDeclaredLater_WaitsForItsChildren pins the ordering bug: a
// Task wired After a Group before the Group had children saw an empty
// Group, counted it as succeeded, and ran before children declared later.
func TestAfterGroupDeclaredLater_WaitsForItsChildren(t *testing.T) {
	out := newGraphTestOutput(t)
	var mu sync.Mutex
	var order []string
	record := func(name string) func(context.Context) error {
		return func(context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			order = append(order, name)
			return nil
		}
	}
	g := out.Group("items")
	out.Task("fetch").After(g).Define(record("fetch"))
	// Give a wrongly eligible fetch every chance to start first.
	time.Sleep(20 * time.Millisecond)
	// Declare both children before either runs: a finishing before b
	// exists would make the Group momentarily all-succeeded, and fetch
	// starting then is correct, not the bug.
	a, b := g.Task("a"), g.Task("b")
	a.Define(record("a"))
	b.Define(record("b"))
	if err := closeWithin(t, out); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if len(order) != 3 || order[2] != "fetch" {
		t.Fatalf("start order = %v, want fetch after a and b", order)
	}
}

// TestAfterEmptyGroup_WaitRunsItNow proves an empty Group is no hang: the
// Wait seals it, so the Task after it runs at once.
func TestAfterEmptyGroup_WaitRunsItNow(t *testing.T) {
	out := newGraphTestOutput(t)
	g := out.Group("nothing to do")
	out.Task("later") // declared, still the caller's to Define
	fetch := out.Task("fetch").After(g).Define(nopWork)
	done := make(chan error, 1)
	go func() { done <- fetch.wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Wait = %v, want nil", err)
		}
	case <-time.After(waitOutcomeTimeout):
		t.Fatal("Wait hung on an empty Group")
	}
	_ = closeWithin(t, out)
}

// TestAfterEmptyGroup_DrainRunsIt proves the drain treats a Group nobody
// populated as having nothing to wait for.
func TestAfterEmptyGroup_DrainRunsIt(t *testing.T) {
	out := newGraphTestOutput(t)
	g := out.Group("nothing to do")
	fetch := out.Task("fetch").After(g).Define(nopWork)
	if err := closeWithin(t, out); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got := fetch.snapshot().State; got != succeeded {
		t.Fatalf("fetch = %s, want done", got)
	}
}

// TestAfterGroupWithUndefinedChild_DrainSettlesNotStarted pins a second
// hang: a Group child nobody Defined never resolves, so the Task after the
// Group stayed queued and Finish waited on it forever.
func TestAfterGroupWithUndefinedChild_DrainSettlesNotStarted(t *testing.T) {
	out := newGraphTestOutput(t)
	g := out.Group("g")
	g.Task("never defined")
	fetch := out.Task("fetch").After(g).Define(nopWork)
	_ = closeWithin(t, out)
	if got := fetch.snapshot().State; got != notStarted {
		t.Fatalf("fetch = %s, want not_started", got)
	}
}

// TestAfterForeignHandle_NeverStarts proves a predecessor from another run
// is not mistaken for this run's Task with the same id.
func TestAfterForeignHandle_NeverStarts(t *testing.T) {
	other := newGraphTestOutput(t)
	foreign := other.Task("elsewhere")
	out := newGraphTestOutput(t)
	out.Task("same id in this run").Define(nopWork)
	dependent := out.Task("dependent").After(foreign).Define(nopWork)
	_ = closeWithin(t, out)
	_ = closeWithin(t, other)
	if got := dependent.snapshot().State; got != notStarted {
		t.Fatalf("dependent = %s, want not_started", got)
	}
}
