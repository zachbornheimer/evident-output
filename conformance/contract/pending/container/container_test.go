//go:build ignore

// C31-009 and C31-010: only Output, GroupHandle, and SequenceHandle satisfy
// evo.Container; a Task never declares children.
package container_test

import (
	"context"
	"io"
	"reflect"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestC31_009_OnlyOutputGroupAndSequenceSatisfyContainer(t *testing.T) {
	container := reflect.TypeFor[evo.Container]()
	for _, typ := range []reflect.Type{
		reflect.TypeFor[*evo.Run](),
		reflect.TypeFor[*evo.Group](),
		reflect.TypeFor[*evo.Sequence](),
	} {
		if !typ.Implements(container) {
			t.Fatalf("%s does not satisfy evo.Container", typ)
		}
	}
}

func TestC31_009_TaskHandleDoesNotSatisfyContainer(t *testing.T) {
	if reflect.TypeFor[*evo.Task]().Implements(reflect.TypeFor[evo.Container]()) {
		t.Fatal("*evo.Task satisfies evo.Container; a Task never declares children")
	}
}

// mountChecks is a topology builder that knows its shape and accepts any
// Container, the way the contract says shared builders are written.
func mountChecks(c evo.Container) {
	c.Task("check file integrity")
	c.Group("validate plan").Task("check schema")
	c.Sequence("consolidate packages").Task("detect package managers")
}

func TestC31_010_TopologyBuilderMountsOntoAnyContainer(t *testing.T) {
	out := (*evo.Run)(nil)
	_ = evo.Execute(context.Background(), evo.Config{Output: evo.Plain, Facades: evo.Facades{Stdout: io.Discard}}, func(ctx context.Context, r *evo.Run) error { return nil })
	t.Cleanup(func() { _ = error(nil) })
	mountChecks(out)
	mountChecks(out.Group("g"))
	mountChecks(out.Sequence("s"))
	if out.Err() != nil {
		t.Fatalf("mounting onto a Container is misuse: %v", out.Err())
	}
}
