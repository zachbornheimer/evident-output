package compose_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func mustMethod(t *testing.T, typ reflect.Type, name string) reflect.Method {
	t.Helper()
	method, ok := typ.MethodByName(name)
	if !ok {
		t.Fatalf("%s has no method %s", typ, name)
	}
	return method
}

func TestC31_011_BuilderCallbacksTakeOnlyTheirContainer(t *testing.T) {
	for name, method := range map[string]reflect.Method{
		"GroupHandle.Define":    mustMethod(t, reflect.TypeFor[*evo.GroupHandle](), "Define"),
		"SequenceHandle.Define": mustMethod(t, reflect.TypeFor[*evo.SequenceHandle](), "Define"),
	} {
		build := method.Type.In(1)
		if build.NumIn() != 1 || build.NumOut() != 0 {
			t.Fatalf("%s builder is %s, want func(container) with no ctx and no error", name, build)
		}
	}
}

func TestC31_012_BuilderMayReadProvenComputedValues(t *testing.T) {
	out := newQuietOutput(t, true)
	pre := evo.Compute(out.Task("discover installed packages"), func(context.Context) (int, error) { return 2, nil })
	fan := out.Group("centralize packages").After(pre)
	fan.Define(func(g *evo.GroupHandle) {
		for i := range pre.Get() {
			g.Task(fmt.Sprint("centralize ", i)).Define(func(context.Context) error { return nil })
		}
	})
	steps := out.Sequence("steps")
	first := evo.Compute(steps.Task("detect"), func(context.Context) (int, error) { return 1, nil })
	ordered := steps.Group("ordered")
	ordered.Define(func(g *evo.GroupHandle) {
		for i := range first.Get() {
			g.Task(fmt.Sprint("ordered ", i)).Define(func(context.Context) error { return nil })
		}
	})
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if out.Err() != nil || len(fan.Snapshot().Tasks) != 2 || len(ordered.Snapshot().Tasks) != 1 {
		t.Fatalf("Err() = %v, fan=%d ordered=%d", out.Err(), len(fan.Snapshot().Tasks), len(ordered.Snapshot().Tasks))
	}
}

func TestC31_014_BuilderRunsOnceWhenContainerBecomesEligible(t *testing.T) {
	out := newQuietOutput(t, false)
	var builds atomic.Int32
	var predecessorSettled atomic.Bool
	pre := evo.Compute(out.Task("pre"), func(context.Context) (int, error) { return 3, nil })
	fan := out.Group("fan").After(pre)
	fan.Define(func(g *evo.GroupHandle) {
		builds.Add(1)
		predecessorSettled.Store(pre.Get() == 3)
		g.Task("child").Define(func(context.Context) error { return nil })
	})
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if builds.Load() != 1 || !predecessorSettled.Load() {
		t.Fatalf("builds=%d predecessorSettled=%v", builds.Load(), predecessorSettled.Load())
	}
	if got := fan.Snapshot().Tasks[0].State; got != evo.Done {
		t.Fatalf("child state = %v, want Done (container owns its lifecycle)", got)
	}
}

func TestC31_015_FailedPredecessorLeavesBuilderContainerNotStarted(t *testing.T) {
	out := newQuietOutput(t, false)
	bad := evo.Compute(out.Task("produce"), func(context.Context) (int, error) { return 0, errors.New("boom") })
	var built atomic.Bool
	fan := out.Group("fan").After(bad)
	fan.Define(func(*evo.GroupHandle) { built.Store(true) })
	_ = out.Finish()
	if built.Load() {
		t.Fatal("builder ran after its predecessor failed")
	}
	if got := fan.Snapshot().State; got != evo.NotStarted {
		t.Fatalf("container state = %v, want NotStarted", got)
	}
}

func TestC31_016_DeclaringTopologyInsideTaskDefineIsMisuse(t *testing.T) {
	declarations := map[string]func(g *evo.GroupHandle){
		"Task":     func(g *evo.GroupHandle) { g.Task("inner").Define(func(context.Context) error { return nil }) },
		"Group":    func(g *evo.GroupHandle) { g.Group("inner") },
		"Sequence": func(g *evo.GroupHandle) { g.Sequence("inner") },
	}
	for name, declare := range declarations {
		t.Run(name, func(t *testing.T) {
			out := newQuietOutput(t, false)
			g := out.Group("g")
			g.Task("outer").Define(func(context.Context) error { declare(g); return nil })
			_ = out.Finish()
			if !errors.Is(out.Err(), evo.ErrDeclaredInCallback) {
				t.Fatalf("Err() = %v, want ErrDeclaredInCallback", out.Err())
			}
		})
	}
}

func TestC31_017_BuilderContainerRejectsStaticChildrenAndSecondDefine(t *testing.T) {
	setups := map[string]func(g *evo.GroupHandle){
		"static child first": func(g *evo.GroupHandle) {
			g.Task("static").Define(func(context.Context) error { return nil })
			g.Define(func(*evo.GroupHandle) {})
		},
		"second Define": func(g *evo.GroupHandle) {
			g.Define(func(*evo.GroupHandle) {})
			g.Define(func(*evo.GroupHandle) {})
		},
	}
	for name, setup := range setups {
		t.Run(name, func(t *testing.T) {
			out := newQuietOutput(t, false)
			setup(out.Group("g"))
			_ = out.Finish()
			if !errors.Is(out.Err(), evo.ErrInvalidConfig) {
				t.Fatalf("Err() = %v, want ErrInvalidConfig", out.Err())
			}
		})
	}
}

func TestC31_017_PriorAfterDoesNotInvalidateDefine(t *testing.T) {
	out := newQuietOutput(t, false)
	pre := evo.Compute(out.Task("pre"), func(context.Context) (int, error) { return 1, nil })
	out.Group("g").After(pre).Define(func(g *evo.GroupHandle) {
		g.Task("child").Define(func(context.Context) error { return nil })
	})
	if err := out.Finish(); err != nil || out.Err() != nil {
		t.Fatalf("Finish = %v, Err() = %v", err, out.Err())
	}
}
