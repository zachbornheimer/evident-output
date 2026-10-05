package review_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// entityBeforeInit declares a Task inside a closure that textually precedes
// the function's evo.Init call — the conformance/future/behavior/file shape
// that sliced body[init:entity] with entity < init and panicked.
const entityBeforeInit = `package app
import evo "github.com/zachbornheimer/evident-output"
func run(roots []string) {
  step := func(out *evo.Output) { out.Task("opaque") }
  out := evo.Init(evo.Config{Isolated: true})
  step(out)
}
`

func TestGoSource_EntityBeforeInitDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("GoSource panicked on a Task declared before evo.Init: %v", r)
		}
	}()
	review.GoSource("entity_before_init.go", entityBeforeInit)
}

// The first-paint window runs from evo.Init to the first entity declared
// after it. A Task that precedes Init must not close the window early.
func TestFirstPaint_WindowStartsAtInitNotAtEarlierEntity(t *testing.T) {
	src := `package app
import (
  "os"
  evo "github.com/zachbornheimer/evident-output"
)
func run(path string) {
  step := func(out *evo.Output) { out.Task("early") }
  out := evo.Init(evo.Config{Title: "x"})
  data, _ := os.ReadFile(path)
  _ = data
  out.Task("late")
  step(out)
}
`
	res := review.GoSource("window.go", src)
	for _, f := range res.Findings {
		if f.RuleID == "FP-002" {
			return
		}
	}
	t.Fatalf("expected FP-002 for os.ReadFile between Init and the first later Task: %+v", res.Findings)
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
	root := filepath.Join(filepath.Dir(self), "..", "..", "..")
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("GoDirectory(%s) panicked: %v", root, r)
		}
	}()
	if _, err := review.GoDirectory(root); err != nil {
		t.Fatalf("GoDirectory(%s): %v", root, err)
	}
}
