package evo_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func newQuietOutput(t *testing.T, strict bool) *evo.Output {
	t.Helper()
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Plain: true, Strict: strict})
	t.Cleanup(func() { _ = out.Close() })
	return out
}

func TestCompute_ValueFlowsToAfterConsumer(t *testing.T) {
	out := newQuietOutput(t, false)
	repo := out.Group("repository")
	branches := evo.Compute(repo.Task("prune landed branches"), func(context.Context) ([]string, error) {
		return []string{"a", "b"}, nil
	})
	var got []string
	consumer := repo.Task("prune deleted remote branches").After(branches).Define(func(context.Context) error {
		got = branches.Get()
		return nil
	})
	if err := consumer.Wait(); err != nil {
		t.Fatalf("consumer: %v", err)
	}
	if len(got) != 2 || got[0] != "a" {
		t.Fatalf("Get() = %v", got)
	}
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
}

func TestCompute_GetBeforeSettleIsMisuse(t *testing.T) {
	out := newQuietOutput(t, false)
	release := make(chan struct{})
	c := evo.Compute(out.Task("slow"), func(context.Context) (int, error) {
		<-release
		return 7, nil
	})
	if v := c.Get(); v != 0 {
		t.Fatalf("early Get() = %d, want zero value", v)
	}
	close(release)
	_ = out.Finish()
	if !errors.Is(out.Err(), evo.ErrComputedUnsettled) {
		t.Fatalf("Err() = %v, want ErrComputedUnsettled", out.Err())
	}
}

func TestCompute_GetBeforeSettlePanicsUnderStrict(t *testing.T) {
	out := newQuietOutput(t, true)
	release := make(chan struct{})
	c := evo.Compute(out.Task("slow"), func(context.Context) (int, error) {
		<-release
		return 7, nil
	})
	defer func() {
		close(release)
		_ = out.Finish()
		if recover() == nil {
			t.Fatal("Get before settle did not panic under Config.Strict")
		}
	}()
	c.Get()
}

func TestCompute_FailedProducerNeverStartsConsumerOrBuilder(t *testing.T) {
	out := newQuietOutput(t, false)
	g := out.Group("g")
	bad := evo.Compute(g.Task("produce"), func(context.Context) (int, error) { return 0, errors.New("boom") })
	var ran atomic.Bool
	consumer := g.Task("consume").After(bad).Define(func(context.Context) error { ran.Store(true); return nil })
	built := g.Group("built").After(bad)
	built.Define(func(*evo.GroupHandle) { ran.Store(true) })

	_ = out.Finish()
	if ran.Load() {
		t.Fatal("consumer or builder ran after its producer failed")
	}
	if got := consumer.Snapshot().State; got != evo.NotStarted {
		t.Fatalf("consumer state = %v, want NotStarted", got)
	}
	if got := built.Snapshot().State; got != evo.NotStarted {
		t.Fatalf("builder container state = %v, want NotStarted", got)
	}
}

func TestCompute_CrossContainerGroupProducerSequenceConsumer(t *testing.T) {
	out := newQuietOutput(t, false)
	repo := out.Group("repository")
	worktrees := evo.Compute(repo.Task("prune worktrees"), func(context.Context) (string, error) { return "kept", nil })
	seq := out.Sequence("packages")
	var got string
	seq.Task("detect").After(worktrees).Define(func(context.Context) error { got = worktrees.Get(); return nil })
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got != "kept" {
		t.Fatalf("got %q", got)
	}
}

func TestContainerDefine_BuilderRunsOnceAfterPredecessor(t *testing.T) {
	out := newQuietOutput(t, false)
	pre := evo.Compute(out.Task("pre"), func(context.Context) (int, error) { return 3, nil })
	var builds atomic.Int32
	var preSettledAtBuild atomic.Bool
	g := out.Group("fan").After(pre)
	g.Define(func(g *evo.GroupHandle) {
		builds.Add(1)
		preSettledAtBuild.Store(pre.Get() == 3)
		for i := range pre.Get() {
			g.Task(fmt.Sprint("t", i)).Define(func(context.Context) error { return nil })
		}
	})
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if builds.Load() != 1 || !preSettledAtBuild.Load() {
		t.Fatalf("builds=%d preSettled=%v", builds.Load(), preSettledAtBuild.Load())
	}
	if n := len(g.Snapshot().Tasks); n != 3 {
		t.Fatalf("children = %d, want 3", n)
	}
}

func TestContainerDefine_GroupChildrenRunConcurrently(t *testing.T) {
	out := newQuietOutput(t, false)
	const n = 4
	var ready sync.WaitGroup
	ready.Add(n)
	g := out.Group("g")
	g.Define(func(g *evo.GroupHandle) {
		for i := range n {
			g.Task(fmt.Sprint("t", i)).Define(func(context.Context) error {
				ready.Done()
				ready.Wait() // deadlocks unless all n run at once
				return nil
			})
		}
	})
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
}

func TestContainerDefine_SequenceChildrenRunInOrder(t *testing.T) {
	out := newQuietOutput(t, false)
	var mu sync.Mutex
	var order []int
	s := out.Sequence("s")
	s.Define(func(s *evo.SequenceHandle) {
		for i := range 5 {
			s.Task(fmt.Sprint("t", i)).Define(func(context.Context) error {
				mu.Lock()
				defer mu.Unlock()
				order = append(order, i)
				return nil
			})
		}
	})
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	for i, v := range order {
		if v != i {
			t.Fatalf("order = %v", order)
		}
	}
	if len(order) != 5 {
		t.Fatalf("order = %v", order)
	}
}

func TestContainerDefine_DeclaringFromTaskCallbackIsMisuse(t *testing.T) {
	out := newQuietOutput(t, false)
	g := out.Group("g")
	g.Task("outer").Define(func(context.Context) error {
		g.Task("inner").Define(func(context.Context) error { return nil })
		return nil
	})
	_ = out.Finish()
	if !errors.Is(out.Err(), evo.ErrDeclaredInCallback) {
		t.Fatalf("Err() = %v, want ErrDeclaredInCallback", out.Err())
	}
}

// TestCompute_FullRepositoryAndPackagesScenario runs Zach's sketch with
// in-memory work and asserts the tree, the order, and that a per-package
// Skipped("modified") folds without failing the run.
func TestCompute_FullRepositoryAndPackagesScenario(t *testing.T) {
	out := newQuietOutput(t, false)
	var mu sync.Mutex
	var log []string
	note := func(s string) { mu.Lock(); log = append(log, s); mu.Unlock() }

	type managerPackages struct {
		Name     string
		Packages []string
	}
	repository := out.Group("repository")
	branches := evo.Compute(repository.Task("prune landed branches"), func(context.Context) ([]string, error) {
		note("branches")
		return []string{"old"}, nil
	})
	worktrees := evo.Compute(repository.Task("prune unused worktrees"), func(context.Context) ([]string, error) {
		note("worktrees")
		return []string{"/kept"}, nil
	})
	repository.Task("prune stale remote-tracking refs").Define(func(context.Context) error { return nil })
	repository.Task("prune deleted remote branches").After(branches).Define(func(context.Context) error {
		note("remote:" + branches.Get()[0])
		return nil
	})

	packages := out.Sequence("consolidate packages")
	managers := evo.Compute(packages.Task("detect package managers").After(worktrees), func(context.Context) ([]string, error) {
		note("managers:" + worktrees.Get()[0])
		return []string{"brew", "npm"}, nil
	})
	inventory := evo.Compute(packages.Task("discover installed packages"), func(context.Context) ([]managerPackages, error) {
		note("inventory")
		var inv []managerPackages
		for _, m := range managers.Get() {
			inv = append(inv, managerPackages{Name: m, Packages: []string{m + "-a", m + "-modified"}})
		}
		return inv, nil
	})
	centralize := packages.Group("centralize packages")
	centralize.Define(func(g *evo.GroupHandle) {
		for _, m := range inventory.Get() {
			mg := g.Group(m.Name)
			for _, pkg := range m.Packages {
				task := mg.Task("centralize " + pkg)
				if pkg == m.Name+"-modified" {
					task.Skipped(evo.Reason("modified"))
					continue
				}
				task.Define(func(context.Context) error { note("centralize:" + pkg); return nil })
			}
		}
	})

	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if err := out.Err(); err != nil {
		t.Fatalf("Err: %v", err)
	}

	index := map[string]int{}
	for i, s := range log {
		index[s] = i
	}
	for _, pair := range [][2]string{
		{"worktrees", "managers:/kept"}, {"managers:/kept", "inventory"},
		{"inventory", "centralize:brew-a"}, {"inventory", "centralize:npm-a"},
		{"branches", "remote:old"},
	} {
		a, aok := index[pair[0]]
		b, bok := index[pair[1]]
		if !aok || !bok || a > b {
			t.Fatalf("want %q before %q in %v", pair[0], pair[1], log)
		}
	}

	snap := centralize.Snapshot()
	if len(snap.Collections) != 2 {
		t.Fatalf("manager groups = %d, want 2", len(snap.Collections))
	}
	for _, mg := range snap.Collections {
		if len(mg.Tasks) != 2 {
			t.Fatalf("%s tasks = %d, want 2", mg.Name, len(mg.Tasks))
		}
		var done, skipped int
		for _, task := range mg.Tasks {
			switch task.State {
			case evo.Done:
				done++
			case evo.Skipped:
				skipped++
			}
		}
		if done != 1 || skipped != 1 {
			t.Fatalf("%s: done=%d skipped=%d, want 1 and 1", mg.Name, done, skipped)
		}
	}
}

func TestCompute_SameSequenceLaterSiblingGetsWithoutAfter(t *testing.T) {
	out := newQuietOutput(t, true)
	seq := out.Sequence("steps")
	first := evo.Compute(seq.Task("first"), func(context.Context) (int, error) { return 41, nil })
	var got int
	seq.Task("second").Define(func(context.Context) error { got = first.Get() + 1; return nil })
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got != 42 || out.Err() != nil {
		t.Fatalf("got %d, Err() = %v", got, out.Err())
	}
}

func TestCompute_CrossContainerGetWithoutAfterIsMisuse(t *testing.T) {
	out := newQuietOutput(t, false)
	makeTask := out.Group("producers").Task("make")
	producer := evo.Compute(makeTask, func(context.Context) (int, error) { return 1, nil })
	seq := out.Sequence("consumers")
	var got int
	seq.Task("read").Define(func(context.Context) error {
		_ = makeTask.Wait()
		got = producer.Get()
		return nil
	})
	_ = out.Finish()
	if !errors.Is(out.Err(), evo.ErrComputedUnordered) {
		t.Fatalf("Err() = %v, want ErrComputedUnordered", out.Err())
	}
	if got != 0 {
		t.Fatalf("unordered Get() = %d, want zero value", got)
	}
}
