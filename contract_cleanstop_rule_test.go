package evo

import (
	"os"
	"strings"
	"testing"
)

const cleanStopRuleSentence = "Main subscribes each signal named in `CleanStop` for exactly the time a callback naming it runs, and that signal, arriving then, cancels only that Task's context and ends it Succeeded without cancelling the run, so a run with nothing else left exits 0; `CleanStop` under `Execute` is refused at `Define` with `ErrInvalidConfig`, because the host owns signals there."

// TestContractCleanStopRuleIsStatedOnce pins the one CONTRACT.md sentence about CleanStop signals.
func TestContractCleanStopRuleIsStatedOnce(t *testing.T) {
	body, err := os.ReadFile("CONTRACT.md")
	if err != nil {
		t.Fatal(err)
	}
	flat := strings.Join(strings.Fields(string(body)), " ")
	if n := strings.Count(flat, cleanStopRuleSentence); n != 1 {
		t.Fatalf("CONTRACT.md states the CleanStop rule %d times, want once: %q", n, cleanStopRuleSentence)
	}
	if n := strings.Count(flat, "arriving then, cancels only that Task's context"); n != 1 {
		t.Fatalf("CONTRACT.md repeats the CleanStop cancel clause %d times, want once", n)
	}
}
