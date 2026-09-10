package evo_test

import (
	"bytes"
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
		task.Define(func() error {
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

	seq.Task("a").Define(func() error {
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
	seq.Task("b").Define(func() error {
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
	seq.Task("c").Define(func() error {
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
	a.Define(func() error {
		close(aStarted)
		<-aRelease
		return nil
	})
	b.After(a).Define(func() error {
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

func TestScheduler_EachRangeSettlesSubmittedWork(t *testing.T) {
	t.Parallel()
	out := isolatedScheduler(t, schedulerTestCeiling, nil, false)
	g := out.Group("worktrees")
	paths := []string{"alpha", "beta"}

	for path, task := range g.Each(paths) {
		task.Define(func() error { return nil })
		_ = path
	}

	snap := g.Snapshot()
	if len(snap.Tasks) != len(paths) {
		t.Fatalf("Each children = %d, want %d", len(snap.Tasks), len(paths))
	}
	for _, child := range snap.Tasks {
		if child.State != evo.Done {
			t.Fatalf("child %q state = %s, want Done after Each range", child.Name, child.State)
		}
	}
}

func TestScheduler_EachRangeDoesNotHangOnUndefinedChildren(t *testing.T) {
	t.Parallel()
	out := isolatedScheduler(t, schedulerTestCeiling, nil, false)
	g := out.Group("worktrees")
	paths := []string{"defined", "undefined"}

	for path, task := range g.Each(paths) {
		if path == "defined" {
			task.Define(func() error { return nil })
		}
	}

	snap := g.Snapshot()
	var defined, undefined evo.TaskSnapshot
	for _, child := range snap.Tasks {
		switch child.Name {
		case "defined":
			defined = child
		case "undefined":
			undefined = child
		}
	}
	if defined.State != evo.Done {
		t.Fatalf("defined child state = %s, want Done", defined.State)
	}
	if undefined.State != evo.Pending {
		t.Fatalf("undefined child state = %s, want Pending (range must not wait)", undefined.State)
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
