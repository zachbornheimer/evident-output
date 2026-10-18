package review_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// entityBeforeRun declares a Task inside a closure that textually precedes
// the run body's own first entity.
const entityBeforeRun = `package app
import (
  "context"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, r *evo.Run) error {
  step := func(g *evo.Group) { g.Task("opaque") }
  step(r.Group("steps"))
  return nil
}
`

func TestGoSource_EntityBeforeRunBodyDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("GoSource panicked on a Task declared in a closure: %v", r)
		}
	}()
	review.GoSource("entity_before_run.go", entityBeforeRun)
}

// The first-paint window runs from the top of the run body to its first
// entity. A Task declared inside an earlier closure must not close the
// window early, because the closure does not run where it is written.
func TestFirstPaint_ClosureDeclaredTaskDoesNotCloseTheWindow(t *testing.T) {
	src := `package app
import (
  "context"
  "os"
  evo "github.com/zachbornheimer/evident-output/v2"
)
func run(ctx context.Context, r *evo.Run, path string) error {
  step := func(g *evo.Group) { g.Task("early") }
  data, _ := os.ReadFile(path)
  _ = data
  r.Task("late")
  step(r.Group("steps"))
  return nil
}
`
	res := review.GoSource("window.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "FP-002" {
			return
		}
	}
	t.Fatalf("expected FP-002 for os.ReadFile before the run body's first entity: %+v", res.Findings)
}

// TestGoDirectory_ReviewsThisRepoWithoutPanic is the guard: directory review
// of the whole module must complete, since agents run it on worktrees.
func TestGoDirectory_ReviewsThisRepoWithoutPanic(t *testing.T) {
	if testing.Short() {
		t.Skip("walks the whole module")
	}
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller: no file for this test")
	}
	root := filepath.Join(filepath.Dir(self), "..", "..", "..", "..")
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("GoDirectory(%s) panicked: %v", root, r)
		}
	}()
	if _, err := review.GoDirectory(root); err != nil {
		t.Fatalf("GoDirectory(%s): %v", root, err)
	}
}
