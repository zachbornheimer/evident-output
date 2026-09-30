package scheduler_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/conformance/contract/harness"
)

const rendezvousTimeout = 2 * time.Second

func TestC03_001_GroupSiblingsRunConcurrently(t *testing.T) {
	out, _ := harness.New(t)
	g := out.Group("g")
	aUp, bUp := make(chan struct{}), make(chan struct{})
	meet := func(mine, theirs chan struct{}) func(context.Context) error {
		return func(context.Context) error {
			close(mine)
			select {
			case <-theirs:
				return nil
			case <-time.After(rendezvousTimeout):
				return errors.New("sibling never started: declaration order serialized the Group")
			}
		}
	}
	g.Task("a").Define(meet(aUp, bUp))
	g.Task("b").Define(meet(bUp, aUp))
	if err := g.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestC03_002_SequenceRunsChildrenInOrderOneAtATime(t *testing.T) {
	out, _ := harness.New(t)
	var running, peak atomic.Int32
	var mu sync.Mutex
	var order []string
	step := func(name string) func(context.Context) error {
		return func(context.Context) error {
			now := running.Add(1)
			if now > peak.Load() {
				peak.Store(now)
			}
			time.Sleep(5 * time.Millisecond)
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			running.Add(-1)
			return nil
		}
	}
	s := out.Sequence("s")
	for _, name := range []string{"one", "two", "three"} {
		s.Task(name).Define(step(name))
	}
	if err := s.Wait(); err != nil {
		t.Fatal(err)
	}
	if peak.Load() != 1 || len(order) != 3 || order[0] != "one" || order[1] != "two" || order[2] != "three" {
		t.Fatalf("peak=%d order=%v", peak.Load(), order)
	}
}

func TestC03_003_AfterDelaysATaskUntilItsPredecessorSettles(t *testing.T) {
	out, _ := harness.New(t)
	var mu sync.Mutex
	var order []string
	record := func(name string) func(context.Context) error {
		return func(context.Context) error {
			time.Sleep(5 * time.Millisecond)
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			return nil
		}
	}
	first := out.Task("first")
	second := out.Task("second").After(first)
	second.Define(record("second"))
	first.Define(record("first"))
	if err := second.Wait(); err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 || order[0] != "first" {
		t.Fatalf("order = %v", order)
	}
}

func TestC03_004_SchedulerBoundsConcurrencyToConfig(t *testing.T) {
	const limit, tasks = 2, 8
	out, _ := harness.New(t, func(c *evo.Config) { c.MaxConcurrency = limit })
	var running, peak atomic.Int32
	g := out.Group("g")
	for i := range tasks {
		g.Task(string(rune('a' + i))).Define(func(context.Context) error {
			now := running.Add(1)
			for {
				old := peak.Load()
				if now <= old || peak.CompareAndSwap(old, now) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			running.Add(-1)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		t.Fatal(err)
	}
	if peak.Load() > limit || peak.Load() < 1 {
		t.Fatalf("peak concurrency %d, limit %d", peak.Load(), limit)
	}
}
