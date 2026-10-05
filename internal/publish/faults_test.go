package publish

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
)

// commitSteps commits a tree over an existing one under faults that
// record every step reached for dest.
func commitSteps(t *testing.T, noExchange bool) []Step {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "pkg")
	plantTree(t, dest, originalFiles)
	var steps []Step
	restore := InjectFaults(Faults{NoExchange: noExchange, At: func(step Step, at string) {
		if at == dest {
			steps = append(steps, step)
		}
	}})
	defer restore()
	if err := stageTree(t, dest, replacementFiles).Commit(context.Background(), Guard{}); err != nil {
		t.Fatal(err)
	}
	requireOnly(t, filepath.Dir(dest), "pkg")
	return steps
}

func TestFaultsSeeEveryStepOfATreeCommit(t *testing.T) {
	if got, want := commitSteps(t, false), []Step{StepStaged, StepLocked, StepSwapped, StepReleasing}; !slices.Equal(got, want) {
		t.Fatalf("steps = %v, want %v", got, want)
	}
}

func TestFaultsCanForceTheMoveAsidePath(t *testing.T) {
	if got, want := commitSteps(t, true), []Step{StepStaged, StepLocked, StepAside, StepSwapped, StepReleasing}; !slices.Equal(got, want) {
		t.Fatalf("steps = %v, want %v", got, want)
	}
}

func TestRestoredFaultsInjectNothing(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "pkg")
	called := false
	InjectFaults(Faults{At: func(Step, string) { called = true }})()
	if err := stageTree(t, dest, replacementFiles).Commit(context.Background(), Guard{}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("a restored fault still ran")
	}
}
