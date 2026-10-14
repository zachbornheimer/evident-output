package evo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
)

func newQuietOutput(t *testing.T, strict bool) *Run {
	t.Helper()
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: io.Discard}}, tuning{strict: strict})
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
	if !errors.Is(out.err(), ErrInvalidConfig) {
		t.Fatalf("Err() = %v, want ErrInvalidConfig", out.err())
	}
}
