package evo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/record"
)

func newQuietOutput(t *testing.T, strict bool) *Run {
	t.Helper()
	out := initRun(Config{isolated: true, stdout: io.Discard, plain: true, strict: strict})
	t.Cleanup(func() { _ = out.close() })
	return out
}

func TestCompute_ValueFlowsToAfterConsumer(t *testing.T) {
	out := newQuietOutput(t, false)
	repo := out.Group("repository")
	branches := Compute(repo.Task("prune landed branches"), func(context.Context) ([]string, error) {
		return []string{"a", "b"}, nil
	})
	var got []string
	consumer := repo.Task("prune deleted remote branches").After(branches).Define(func(context.Context) error {
		got = branches.Get()
		return nil
	})
	if err := consumer.wait(); err != nil {
		t.Fatalf("consumer: %v", err)
	}
	if len(got) != 2 || got[0] != "a" {
		t.Fatalf("Get() = %v", got)
	}
	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
}

func TestCompute_GetBeforeSettleIsMisuse(t *testing.T) {
	out := newQuietOutput(t, false)
	release := make(chan struct{})
	c := Compute(out.Task("slow"), func(context.Context) (int, error) {
		<-release
		return 7, nil
	})
	if v := c.Get(); v != 0 {
		t.Fatalf("early Get() = %d, want zero value", v)
	}
	close(release)
	_ = out.finish()
	if !errors.Is(out.err(), errComputedUnsettled) {
		t.Fatalf("Err() = %v, want ErrComputedUnsettled", out.err())
	}
}

func TestCompute_GetBeforeSettlePanicsUnderStrict(t *testing.T) {
	out := newQuietOutput(t, true)
	release := make(chan struct{})
	c := Compute(out.Task("slow"), func(context.Context) (int, error) {
		<-release
		return 7, nil
	})
	defer func() {
		close(release)
		_ = out.finish()
		if recover() == nil {
			t.Fatal("Get before settle did not panic under Config.Strict")
		}
	}()
	c.Get()
}

func TestCompute_FailedProducerNeverStartsConsumerOrBuilder(t *testing.T) {
	out := newQuietOutput(t, false)
	g := out.Group("g")
	bad := Compute(g.Task("produce"), func(context.Context) (int, error) { return 0, errors.New("boom") })
	var ran atomic.Bool
	consumer := g.Task("consume").After(bad).Define(func(context.Context) error { ran.Store(true); return nil })
	built := g.Group("built").After(bad)
	built.define(func(*Group) { ran.Store(true) })

	_ = out.finish()
	if ran.Load() {
		t.Fatal("consumer or builder ran after its producer failed")
	}
	if got := consumer.snapshot().State; got != notStarted {
		t.Fatalf("consumer state = %v, want NotStarted", got)
	}
	if got := built.snapshot().State; got != notStarted {
		t.Fatalf("builder container state = %v, want NotStarted", got)
	}
}

func TestCompute_CrossContainerGroupProducerSequenceConsumer(t *testing.T) {
	out := newQuietOutput(t, false)
	repo := out.Group("repository")
	worktrees := Compute(repo.Task("prune worktrees"), func(context.Context) (string, error) { return "kept", nil })
	seq := out.Sequence("packages")
	var got string
	seq.Task("detect").After(worktrees).Define(func(context.Context) error { got = worktrees.Get(); return nil })
	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got != "kept" {
		t.Fatalf("got %q", got)
	}
}

func TestContainerDefine_BuilderRunsOnceAfterPredecessor(t *testing.T) {
	out := newQuietOutput(t, false)
	pre := Compute(out.Task("pre"), func(context.Context) (int, error) { return 3, nil })
	var builds atomic.Int32
	var preSettledAtBuild atomic.Bool
	g := out.Group("fan").After(pre)
	g.define(func(g *Group) {
		builds.Add(1)
		preSettledAtBuild.Store(pre.Get() == 3)
		for i := range pre.Get() {
			g.Task(fmt.Sprint("t", i)).Define(func(context.Context) error { return nil })
		}
	})
	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if builds.Load() != 1 || !preSettledAtBuild.Load() {
		t.Fatalf("builds=%d preSettled=%v", builds.Load(), preSettledAtBuild.Load())
	}
	if n := len(g.snapshot().Tasks); n != 3 {
		t.Fatalf("children = %d, want 3", n)
	}
}

func TestContainerDefine_GroupChildrenRunConcurrently(t *testing.T) {
	out := newQuietOutput(t, false)
	const n = 4
	var ready sync.WaitGroup
	ready.Add(n)
	g := out.Group("g")
	g.define(func(g *Group) {
		for i := range n {
			g.Task(fmt.Sprint("t", i)).Define(func(context.Context) error {
				ready.Done()
				ready.Wait() // deadlocks unless all n run at once
				return nil
			})
		}
	})
	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
}

func TestContainerDefine_SequenceChildrenRunInOrder(t *testing.T) {
	out := newQuietOutput(t, false)
	var mu sync.Mutex
	var order []int
	s := out.Sequence("s")
	s.define(func(s *Sequence) {
		for i := range 5 {
			s.Task(fmt.Sprint("t", i)).Define(func(context.Context) error {
				mu.Lock()
				defer mu.Unlock()
				order = append(order, i)
				return nil
			})
		}
	})
	if err := out.finish(); err != nil {
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
	_ = out.finish()
	if !errors.Is(out.err(), errDeclaredInCallback) {
		t.Fatalf("Err() = %v, want ErrDeclaredInCallback", out.err())
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
	branches := Compute(repository.Task("prune landed branches"), func(context.Context) ([]string, error) {
		note("branches")
		return []string{"old"}, nil
	})
	worktrees := Compute(repository.Task("prune unused worktrees"), func(context.Context) ([]string, error) {
		note("worktrees")
		return []string{"/kept"}, nil
	})
	repository.Task("prune stale remote-tracking refs").Define(func(context.Context) error { return nil })
	repository.Task("prune deleted remote branches").After(branches).Define(func(context.Context) error {
		note("remote:" + branches.Get()[0])
		return nil
	})

	packages := out.Sequence("consolidate packages")
	managers := Compute(packages.Task("detect package managers").After(worktrees), func(context.Context) ([]string, error) {
		note("managers:" + worktrees.Get()[0])
		return []string{"brew", "npm"}, nil
	})
	inventory := Compute(packages.Task("discover installed packages"), func(context.Context) ([]managerPackages, error) {
		note("inventory")
		var inv []managerPackages
		for _, m := range managers.Get() {
			inv = append(inv, managerPackages{Name: m, Packages: []string{m + "-a", m + "-modified"}})
		}
		return inv, nil
	})
	centralize := packages.Group("centralize packages")
	centralize.define(func(g *Group) {
		for _, m := range inventory.Get() {
			mg := g.Group(m.Name)
			for _, pkg := range m.Packages {
				task := mg.Task("centralize " + pkg)
				if pkg == m.Name+"-modified" {
					task.skipped(reason("modified"))
					continue
				}
				task.Define(func(context.Context) error { note("centralize:" + pkg); return nil })
			}
		}
	})

	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if err := out.err(); err != nil {
		t.Fatalf("Err: %v", err)
	}

	index := map[string]int{}
	for i, s := range log {
		index[s] = i
	}
	for _, pair := range [][2]string{
		{"worktrees", "managers:/kept"},
		{"managers:/kept", "inventory"},
		{"inventory", "centralize:brew-a"},
		{"inventory", "centralize:npm-a"},
		{"branches", "remote:old"},
	} {
		a, aok := index[pair[0]]
		b, bok := index[pair[1]]
		if !aok || !bok || a > b {
			t.Fatalf("want %q before %q in %v", pair[0], pair[1], log)
		}
	}

	snap := centralize.snapshot()
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
			case record.Succeeded:
				done++
			case record.Excluded:
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
	first := Compute(seq.Task("first"), func(context.Context) (int, error) { return 41, nil })
	var got int
	seq.Task("second").Define(func(context.Context) error { got = first.Get() + 1; return nil })
	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got != 42 || out.err() != nil {
		t.Fatalf("got %d, Err() = %v", got, out.err())
	}
}

func TestCompute_CrossContainerGetWithoutAfterIsMisuse(t *testing.T) {
	out := newQuietOutput(t, false)
	makeTask := out.Group("producers").Task("make")
	producer := Compute(makeTask, func(context.Context) (int, error) { return 1, nil })
	seq := out.Sequence("consumers")
	var got int
	seq.Task("read").Define(func(context.Context) error {
		_ = makeTask.wait()
		got = producer.Get()
		return nil
	})
	_ = out.finish()
	if !errors.Is(out.err(), ErrComputedUnordered) {
		t.Fatalf("Err() = %v, want ErrComputedUnordered", out.err())
	}
	if got != 0 {
		t.Fatalf("unordered Get() = %d, want zero value", got)
	}
}

func TestCompute_UnorderedGetFailsRunInDefaultMode(t *testing.T) {
	out := newQuietOutput(t, false)
	makeTask := out.Group("producers").Task("make")
	producer := Compute(makeTask, func(context.Context) (int, error) { return 1, nil })
	seq := out.Sequence("consumers")
	var continued bool
	read := seq.Task("read")
	read.Define(func(context.Context) error {
		_ = makeTask.wait()
		_ = producer.Get()
		continued = true
		return nil
	})
	_ = out.finish()
	if !errors.Is(out.err(), ErrComputedUnordered) {
		t.Fatalf("Err() = %v, want ErrComputedUnordered", out.err())
	}
	if continued {
		t.Fatal("callback continued past an unordered Get")
	}
	if err := read.wait(); err == nil || !strings.Contains(err.Error(), ErrComputedUnordered.Error()) {
		t.Fatalf("consumer Wait() = %v, want a failure naming ErrComputedUnordered", err)
	}
}

func TestCompute_AfterOnBuilderContainerBeforeEligibilityIsValid(t *testing.T) {
	out := newQuietOutput(t, true)
	inv := Compute(out.Group("inventory").Task("scan"), func(context.Context) ([]string, error) {
		return []string{"a", "b"}, nil
	})
	var seen []string
	group := out.Group("centralize").After(inv)
	group.define(func(g *Group) {
		for _, p := range inv.Get() {
			seen = append(seen, p)
			g.Task(p).Define(func(context.Context) error { return nil })
		}
	})
	if err := out.finish(); err != nil || out.err() != nil {
		t.Fatalf("Finish: %v, Err() = %v", err, out.err())
	}
	if len(seen) != 2 {
		t.Fatalf("seen = %v", seen)
	}
}

func TestCompute_AfterOnBuilderContainerAfterDefineIsInvalidConfig(t *testing.T) {
	out := newQuietOutput(t, false)
	pre := out.Group("pre").Task("p")
	pre.Define(func(context.Context) error { return nil })
	g := out.Group("later")
	g.define(func(g *Group) {})
	g.After(pre)
	_ = out.finish()
	if !errors.Is(out.err(), errInvalidConfig) {
		t.Fatalf("Err() = %v, want ErrInvalidConfig", out.err())
	}
}
