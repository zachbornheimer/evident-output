package evo

import "testing"

func requireConclusion(t *testing.T, out *Run, want conclusionState) {
	t.Helper()
	got := out.conclusion()
	if got.state != want {
		t.Fatalf("conclusion state = %q, want %q", got.state, want)
	}
}

func requireClean(t *testing.T, out *Run) {
	t.Helper()
	if err := out.err(); err != nil {
		t.Fatalf("output misuse error: %v", err)
	}
}
