package evo

import (
	"os"
	"strings"
	"testing"
)

// A Get or Wait through a goroutine the runtime cannot trace to the callback
// that started it is not caught: no call names the creator of an exited
// goroutine, the lookups that name the rest are bounded, the callback that
// started a goroutine may have returned, and a timer's goroutine has no creator.
// CONTRACT.md must state each limit beside the rule it limits, so "in every
// mode" is never read as covering those shapes.
func TestContractStatesWhereGoroutineAttributionEnds(t *testing.T) {
	body, err := os.ReadFile("CONTRACT.md")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(strings.Fields(string(body)), " ")
	limits := []string{
		"Attribution ends at a goroutine that exited before anything asked about it",
		"at one first seen after the run's bounded allowance of lookups (eight for each callback or container builder running, shared by those running together)",
		"and at the callback that started it once that callback has returned, so a `Get` through such a goroutine is not caught.",
		"A goroutine `time.AfterFunc` starts carries no creator, so a `Get` through it is not caught either.",
	}
	for _, limit := range limits {
		if !strings.Contains(text, limit) {
			t.Errorf("CONTRACT.md does not state the limit of goroutine attribution: %q", limit)
		}
	}
}
