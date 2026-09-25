package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// API-090 (E-119): TaskHandle.Step was removed in 1.1 with no alias. The
// current item of a count is Progress(completed, total).Doing(item), so the
// MCP rewrites each remaining Step call to exactly that chain.

const stepCallSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run(task *evo.TaskHandle, names []string) {
  for i, name := range names {
    task.Step(i, len(names), name)
  }
}
`

func TestAPI090_StepCall_SuggestsProgressDoing(t *testing.T) {
	f := findingByID(t, review.GoSource("sync.go", stepCallSrc), "API-090")
	want := "task.Progress(i, len(names)).Doing(name)"
	if !strings.Contains(f.Suggestion, want) {
		t.Fatalf("API-090 suggestion = %q, want it to spell %q", f.Suggestion, want)
	}
	if f.Line != 5 {
		t.Fatalf("API-090 line = %d, want 5", f.Line)
	}
	if f.RequiredVersion != "1.1.0" {
		t.Fatalf("API-090 required_version = %q, want 1.1.0", f.RequiredVersion)
	}
}

// A Step chained straight off Task(...) is still an evo Task receiver.
const stepOnTaskCallSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func run() {
  evo.Task("sync").Step(1, 2, "a")
}
`

func TestAPI090_StepOnTaskCall_Fires(t *testing.T) {
	findingByID(t, review.GoSource("sync.go", stepOnTaskCallSrc), "API-090")
}

// A same-named method on a non-evo value (a state machine's Step) is not
// the removed evo verb, and neither is the canonical replacement.
const stepNotEvoSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
type machine struct{}
func (machine) Step(a, b int, s string) {}
func run(m machine, task *evo.TaskHandle) {
  m.Step(1, 2, "x")
  task.Progress(1, 2).Doing("x")
}
`

func TestAPI090_NonEvoStepAndCanonicalForm_Silent(t *testing.T) {
	assertNoFinding(t, review.GoSource("machine.go", stepNotEvoSrc), "API-090")
}
