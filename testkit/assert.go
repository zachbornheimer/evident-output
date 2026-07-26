package testkit

import (
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// RequireConclusion asserts the output conclusion state.
func RequireConclusion(t *testing.T, out *evo.Output, want evo.ConclusionState) {
	t.Helper()
	got := out.Conclusion()
	if got.State != want {
		t.Fatalf("conclusion state = %q, want %q", got.State, want)
	}
}

// RequireClean asserts no misuse error is recorded.
func RequireClean(t *testing.T, out *evo.Output) {
	t.Helper()
	if err := out.Err(); err != nil {
		t.Fatalf("output misuse error: %v", err)
	}
}
