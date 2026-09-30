//go:build evopending

// Pending: ZYS-1203 exports evo.Container. Red today because the package
// does not compile: evo.Container is undefined.
package container_test

import (
	"io"
	"reflect"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestC31_009_OnlyOutputGroupAndSequenceSatisfyContainer(t *testing.T) {
	container := reflect.TypeFor[evo.Container]()
	for _, typ := range []reflect.Type{
		reflect.TypeFor[*evo.Output](),
		reflect.TypeFor[*evo.GroupHandle](),
		reflect.TypeFor[*evo.SequenceHandle](),
	} {
		if !typ.Implements(container) {
			t.Fatalf("%s does not satisfy evo.Container", typ)
		}
	}
}

func TestC31_009_TaskHandleDoesNotSatisfyContainer(t *testing.T) {
	if reflect.TypeFor[*evo.TaskHandle]().Implements(reflect.TypeFor[evo.Container]()) {
		t.Fatal("*evo.TaskHandle satisfies evo.Container; a Task never declares children")
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
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	mountChecks(out)
	mountChecks(out.Group("g"))
	mountChecks(out.Sequence("s"))
	if out.Err() != nil {
		t.Fatalf("mounting onto a Container is misuse: %v", out.Err())
	}
}
