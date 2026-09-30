package compose_test

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestC31_001_ComputeDefinesExactlyOneTask(t *testing.T) {
	out := newQuietOutput(t, false)
	group := out.Group("packages")
	inventory := evo.Compute(group.Task("discover installed packages"), func(context.Context) (int, error) { return 7, nil })
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	snap := group.Snapshot()
	if len(snap.Tasks) != 1 || snap.Tasks[0].State != evo.Done {
		t.Fatalf("Compute declared %+v, want exactly one Done Task", snap.Tasks)
	}
	if got := inventory.Get(); got != 7 {
		t.Fatalf("Get() after settle = %d, want 7", got)
	}
}

func TestC31_004_GetBeforeProducerSettlesIsMisuse(t *testing.T) {
	out := newQuietOutput(t, false)
	release := make(chan struct{})
	value := evo.Compute(out.Task("slow"), func(context.Context) (int, error) {
		<-release
		return 7, nil
	})
	_ = value.Get()
	close(release)
	_ = out.Finish()
	if !errors.Is(out.Err(), evo.ErrComputedUnsettled) {
		t.Fatalf("Err() = %v, want ErrComputedUnsettled", out.Err())
	}
}

func TestC31_004_GetBeforeProducerSettlesPanicsUnderStrict(t *testing.T) {
	out := newQuietOutput(t, true)
	release := make(chan struct{})
	value := evo.Compute(out.Task("slow"), func(context.Context) (int, error) {
		<-release
		return 7, nil
	})
	defer func() {
		close(release)
		_ = out.Finish()
		if recover() == nil {
			t.Fatal("early Get did not fail loudly under Config.Strict")
		}
	}()
	value.Get()
}

func TestC31_005_AfterComputedOrdersConsumerAcrossContainers(t *testing.T) {
	out := newQuietOutput(t, true)
	worktrees := evo.Compute(out.Group("repository").Task("prune unused worktrees"), func(context.Context) (string, error) {
		return "kept", nil
	})
	var got string
	out.Sequence("packages").Task("detect package managers").After(worktrees).Define(func(context.Context) error {
		got = worktrees.Get()
		return nil
	})
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got != "kept" || out.Err() != nil {
		t.Fatalf("got %q, Err() = %v", got, out.Err())
	}
}

func TestC31_006_SequenceOrderSufficesForLaterStep(t *testing.T) {
	out := newQuietOutput(t, true)
	steps := out.Sequence("steps")
	first := evo.Compute(steps.Task("detect package managers"), func(context.Context) (int, error) { return 41, nil })
	var got int
	steps.Task("discover installed packages").Define(func(context.Context) error {
		got = first.Get() + 1
		return nil
	})
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got != 42 || out.Err() != nil {
		t.Fatalf("got %d, Err() = %v", got, out.Err())
	}
}

func TestC31_007_FailedProducerLeavesDependentsNotStarted(t *testing.T) {
	out := newQuietOutput(t, false)
	group := out.Group("g")
	bad := evo.Compute(group.Task("produce"), func(context.Context) (int, error) { return 0, errors.New("boom") })
	var ran atomic.Bool
	consumer := group.Task("consume").After(bad).Define(func(context.Context) error { ran.Store(true); return nil })
	_ = out.Finish()
	if ran.Load() {
		t.Fatal("consumer ran after its producer failed")
	}
	if got := consumer.Snapshot().State; got != evo.NotStarted {
		t.Fatalf("consumer state = %v, want NotStarted", got)
	}
}

func TestC31_008_TaskHandleDeclaresNoChildren(t *testing.T) {
	taskType := reflect.TypeFor[*evo.TaskHandle]()
	for _, name := range []string{"Task", "Group", "Sequence"} {
		if _, ok := taskType.MethodByName(name); ok {
			t.Fatalf("*TaskHandle has method %s: a Task must never declare children", name)
		}
	}
}

func TestC31_012_UnorderedGetIsMisuse(t *testing.T) {
	out := newQuietOutput(t, false)
	makeTask := out.Group("producers").Task("make")
	producer := evo.Compute(makeTask, func(context.Context) (int, error) { return 1, nil })
	out.Sequence("consumers").Task("read").Define(func(context.Context) error {
		_ = makeTask.Wait()
		_ = producer.Get()
		return nil
	})
	_ = out.Finish()
	if !errors.Is(out.Err(), evo.ErrComputedUnordered) {
		t.Fatalf("Err() = %v, want ErrComputedUnordered", out.Err())
	}
}

func TestC31_012_UnorderedGetPanicsUnderStrict(t *testing.T) {
	out := newQuietOutput(t, true)
	makeTask := out.Group("producers").Task("make")
	producer := evo.Compute(makeTask, func(context.Context) (int, error) { return 1, nil })
	var recovered atomic.Bool
	out.Sequence("consumers").Task("read").Define(func(context.Context) error {
		_ = makeTask.Wait()
		defer func() { recovered.Store(recover() != nil) }()
		_ = producer.Get()
		return nil
	})
	_ = out.Finish()
	if !recovered.Load() {
		t.Fatal("unordered Get did not fail loudly under Config.Strict")
	}
}
