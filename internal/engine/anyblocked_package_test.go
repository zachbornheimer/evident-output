package engine

import (
	"testing"
)

// TestAnyBlockedSoFar_BeforeMutate is a white-box carryover of the deleted
// public AnyBlockedSoFar/AnyFailed surface (P6 deletion census): the
// mid-run "is anything blocked/failed yet" question concludeRun needs is
// still exercised, just as an internal helper rather than exported aPI —
// nothing outside the package needs it, since Output.Run/Conclusion already
// answer the same question once a run has finished.
func TestAnyBlockedSoFar_BeforeMutate(t *testing.T) {
	out := Init(Config{Isolated: true})
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
