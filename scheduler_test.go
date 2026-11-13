package evo_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

const schedulerTestCeiling = 3

func isolatedScheduler(t *testing.T, maxConcurrency int, stdout io.Writer, dryRun bool) *evo.Output {
	t.Helper()
	if stdout == nil {
		stdout = io.Discard
	}
	out := evo.Init(evo.Config{
		Isolated:       true,
		Plain:          true,
		Color:          evo.ColorNever,
		Clock:          testkit.NewClock(),
		MaxConcurrency: maxConcurrency,
		Stdout:         stdout,
		Stderr:         io.Discard,
		DryRun:         dryRun,
	})
	t.Cleanup(func() { _ = out.Close() })
	return out
}

func TestScheduler_GroupOverlapRespectsCeiling(t *testing.T) {
	t.Parallel()
	out := isolatedScheduler(t, schedulerTestCeiling, nil, false)
	g := out.Group("checks")

	var mu sync.Mutex
	inflight, maxObserved := 0, 0
	started := make(chan struct{}, schedulerTestCeiling)
	release := make(chan struct{})

	for _, name := range []string{"a", "b", "c"} {
		task := g.Task(name)
		task.Define(func(ctx context.Context) error {
			mu.Lock()
			inflight++
			if inflight > maxObserved {
				maxObserved = inflight
			}
			mu.Unlock()
			started <- struct{}{}
			<-release
			mu.Lock()
			inflight--
			mu.Unlock()
			return nil
		})
	}

	<-started
	<-started
	close(release)
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	if maxObserved <= 1 {
		t.Fatalf("Group max observed concurrency = %d, want > 1", maxObserved)
	}
	if maxObserved > schedulerTestCeiling {
		t.Fatalf("Group max observed concurrency = %d, want <= %d", maxObserved, schedulerTestCeiling)
	}
	if got := out.SchedulerMaxObserved(); got > schedulerTestCeiling {
		t.Fatalf("scheduler max observed = %d, want <= %d", got, schedulerTestCeiling)
	}
}

// TestScheduler_GroupSiblingsReportTruthfulIndependentOutcomes proves a
// Group is not a Sequence: one child failing does not cascade its siblings
// to NotStarted (§4's "independent child work" contract) — each sibling
// settles into its own true, observed outcome (Done or Failed) instead.
func TestScheduler_GroupSiblingsReportTruthfulIndependentOutcomes(t *testing.T) {
	t.Parallel()
	out := isolatedScheduler(t, schedulerTestCeiling, nil, false)
	g := out.Group("checks")

	g.Task("ok-a").Define(func(ctx context.Context) error { return nil })
	g.Task("broken").Define(func(ctx context.Context) error { return errors.New("boom") })
	g.Task("ok-b").Define(func(ctx context.Context) error { return nil })
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	snap := g.Snapshot()
	states := make(map[string]evo.EntityState, len(snap.Tasks))
	for _, child := range snap.Tasks {
		states[child.Name] = child.State
	}
	if states["ok-a"] != evo.Done {
		t.Fatalf("ok-a state = %s, want Done", states["ok-a"])
	}
	if states["broken"] != evo.Failed {
		t.Fatalf("broken state = %s, want Failed", states["broken"])
	}
	if states["ok-b"] != evo.Done {
		t.Fatalf("ok-b state = %s, want Done — a Group sibling's failure must not cascade to NotStarted (that is Sequence's contract, not Group's)", states["ok-b"])
	}
}

func TestScheduler_SequenceDeclarationOrderMaxOne(t *testing.T) {
	t.Parallel()
	out := isolatedScheduler(t, schedulerTestCeiling, nil, false)
	seq := out.Sequence("setup")

	var mu sync.Mutex
	var startOrder []string
	inflight, maxObserved := 0, 0

	aRelease := make(chan struct{})
	bRelease := make(chan struct{})
	aStarted := make(chan struct{})
	bStarted := make(chan struct{})
	cStarted := make(chan struct{})

	seq.Task("a").Define(func(ctx context.Context) error {
		mu.Lock()
		inflight++
		if inflight > maxObserved {
			maxObserved = inflight
		}
		startOrder = append(startOrder, "a")
		mu.Unlock()
		close(aStarted)
		<-aRelease
		mu.Lock()
		inflight--
		mu.Unlock()
		return nil
	})
	seq.Task("b").Define(func(ctx context.Context) error {
		mu.Lock()
		inflight++
		if inflight > maxObserved {
			maxObserved = inflight
		}
		startOrder = append(startOrder, "b")
		mu.Unlock()
		close(bStarted)
		<-bRelease
		mu.Lock()
		inflight--
		mu.Unlock()
		return nil
	})
	seq.Task("c").Define(func(ctx context.Context) error {
		mu.Lock()
		inflight++
		if inflight > maxObserved {
			maxObserved = inflight
		}
		startOrder = append(startOrder, "c")
		mu.Unlock()
		close(cStarted)
		mu.Lock()
		inflight--
		mu.Unlock()
		return nil
	})

	<-aStarted
	select {
	case <-bStarted:
		t.Fatal("sequence child b started while a was still running")
	default:
	}
	close(aRelease)
	<-bStarted
	select {
	case <-cStarted:
		t.Fatal("sequence child c started while b was still running")
	default:
	}
	close(bRelease)
	<-cStarted
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	if maxObserved != 1 {
		t.Fatalf("Sequence max observed concurrency = %d, want 1", maxObserved)
	}
	want := []string{"a", "b", "c"}
	if got := out.SchedulerStartOrder(); !equalStrings(got, want) {
		t.Fatalf("scheduler start order = %v, want %v", got, want)
	}
	if !equalStrings(startOrder, want) {
		t.Fatalf("callback start order = %v, want %v", startOrder, want)
	}
}

func TestScheduler_AfterWaitsForPredecessors(t *testing.T) {
	t.Parallel()
	out := isolatedScheduler(t, schedulerTestCeiling, nil, false)

	aRelease := make(chan struct{})
	aStarted := make(chan struct{})
	bStarted := make(chan struct{})

	a := out.Task("scan")
	b := out.Task("fetch")
	a.Define(func(ctx context.Context) error {
		close(aStarted)
		<-aRelease
		return nil
	})
	b.After(a).Define(func(ctx context.Context) error {
		close(bStarted)
		return nil
	})

	<-aStarted
	select {
	case <-bStarted:
		t.Fatal("After successor started while predecessor was still running")
	default:
	}
	close(aRelease)
	<-bStarted
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
}

// TestScheduler_GroupChildrenSettleSubmittedWork proves multiple Group
// children declared and Defined in a loop (the plain-Group-children shape
// §3.1's identity reversal and Each's removal both point callers to) settle
// Done once Finish drains the scheduler — Each's own "range-end waits only
// for Defined children" framing no longer applies (there is no range), but
// the underlying settle-on-Finish guarantee is the same one Define always
// gave.
func TestScheduler_GroupChildrenSettleSubmittedWork(t *testing.T) {
	t.Parallel()
	out := isolatedScheduler(t, schedulerTestCeiling, nil, false)
	g := out.Group("worktrees")
	paths := []string{"alpha", "beta"}

	for _, path := range paths {
		g.Task(path).Define(func(ctx context.Context) error { return nil })
	}
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	snap := g.Snapshot()
	if len(snap.Tasks) != len(paths) {
		t.Fatalf("children = %d, want %d", len(snap.Tasks), len(paths))
	}
	for _, child := range snap.Tasks {
		if child.State != evo.Done {
			t.Fatalf("child %q state = %s, want Done", child.Name, child.State)
		}
	}
}

// TestScheduler_UndefinedGroupChildDoesNotBlockItsSibling proves declaring
// one Group child that never receives Define does not block a sibling that
// does: waiting on the defined child alone (not draining the whole run via
// Finish, which would report the never-defined sibling as an unresolved
// task) settles it Done while the undefined one stays Pending.
func TestScheduler_UndefinedGroupChildDoesNotBlockItsSibling(t *testing.T) {
	t.Parallel()
	out := isolatedScheduler(t, schedulerTestCeiling, nil, false)
	g := out.Group("worktrees")

	defined := g.Task("defined")
	defined.Define(func(ctx context.Context) error { return nil })
	g.Task("undefined")
	if err := defined.Wait(); err != nil {
		t.Fatalf("Wait: %v", err)
	}

	snap := g.Snapshot()
	var definedSnap, undefinedSnap evo.TaskSnapshot
	for _, child := range snap.Tasks {
		switch child.Name {
		case "defined":
			definedSnap = child
		case "undefined":
			undefinedSnap = child
		}
	}
	if definedSnap.State != evo.Done {
		t.Fatalf("defined child state = %s, want Done", definedSnap.State)
	}
	if undefinedSnap.State != evo.Pending {
		t.Fatalf("undefined child state = %s, want Pending (its sibling's Define must not resolve it)", undefinedSnap.State)
	}
}

func TestScheduler_DryRunMutationNeverCallsCallback(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	out := isolatedScheduler(t, schedulerTestCeiling, &buf, true)
	called := false
	out.Task("delete branch").Delete("local tip", func() error {
		called = true
		return nil
	})
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if called {
		t.Fatal("dry-run mutation callback was invoked")
	}
	got := buf.String()
	if strings.Contains(got, "[changed]") {
		t.Fatalf("dry-run rendered [changed]:\n%s", got)
	}
	collapsed := strings.Join(strings.Fields(got), " ")
	if !strings.Contains(collapsed, "delete") {
		t.Fatalf("want planned imperative tense, got:\n%s", got)
	}
	if strings.Contains(collapsed, "deleted") {
		t.Fatalf("dry-run used committed tense:\n%s", got)
	}
}

// equalStrings reports whether a and b hold the same strings in the same
// order — used to pin the scheduler's observed task-start order exactly.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
