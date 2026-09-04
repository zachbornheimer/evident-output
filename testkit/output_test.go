package testkit_test

import (
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestNew_IsolatedPlainNonInteractive pins L4: testkit.New(t) hands back a
// working, isolated *Output with no live region and no package-global side
// effects, closed automatically at test end.
func TestNew_IsolatedPlainNonInteractive(t *testing.T) {
	out := testkit.New(t)

	out.Task("build").Done()
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if got := out.Conclusion().State; got == evo.StateFailed || got == evo.StateBlocked {
		t.Fatalf("conclusion = %q, want a non-failure state", got)
	}

	// Isolated: never installed as the package-level default.
	if evo.Default() == out {
		t.Fatal("testkit.New must not install the default instance")
	}
}
