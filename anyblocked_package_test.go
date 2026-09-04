package evo

import (
	"bytes"
	"testing"
)

// TestAnyBlockedSoFar_BeforeMutate is a white-box carryover of the deleted
// public AnyBlockedSoFar/AnyFailed surface (P6 deletion census): the
// mid-run "is anything blocked/failed yet" question concludeRun needs is
// still exercised, just as an internal helper rather than exported API —
// nothing outside the package needs it, since Output.Run/Conclusion already
// answer the same question once a run has finished.
func TestAnyBlockedSoFar_BeforeMutate(t *testing.T) {
	var buf bytes.Buffer
	out := Init(Config{Isolated: true, Options: []Option{Title("gates"), To(&buf), Plain(), NoColor()}})
	out.Task("a").Done()
	out.Task("b").Block("policy")
	if !out.anyBlockedSoFar() {
		t.Fatal("expected anyBlockedSoFar")
	}
	if out.anyFailed() {
		t.Fatal("no failures")
	}
	_ = out.Finish()
	if !out.Conclusion().AnyBlocked() {
		t.Fatal("conclusion AnyBlocked")
	}
}
