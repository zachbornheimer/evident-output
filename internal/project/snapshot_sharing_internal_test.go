package project

import (
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

func taskNamed(name string) record.TaskSnapshot { return record.TaskSnapshot{Name: name} }

func TestTasksAtVerbosityHidesFactsWithoutModifyingTheInput(t *testing.T) {
	facts := []record.Fact{{Name: "k", Value: "v"}}
	in := []record.TaskSnapshot{taskNamed("a"), {Name: "b", Facts: facts}}

	out := tasksAtVerbosity(in, false)

	if out[1].Facts != nil {
		t.Fatalf("Facts = %v, want hidden", out[1].Facts)
	}
	if in[1].Facts == nil {
		t.Fatal("tasksAtVerbosity modified its input")
	}
}

func TestTasksAtVerbosityCopiesNothingWhenNoTaskHasFacts(t *testing.T) {
	in := make([]record.TaskSnapshot, 1000)
	allocs := testing.AllocsPerRun(10, func() { _ = tasksAtVerbosity(in, false) })
	if allocs != 0 {
		t.Fatalf("allocs = %v, want 0 for 1,000 Tasks without Facts", allocs)
	}
}
