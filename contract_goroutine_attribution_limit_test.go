package evo_test

import (
	"os"
	"strings"
	"testing"
)

// A Get or Wait through a goroutine the runtime cannot trace to the callback
// that started it is not caught: no call names the creator of an exited
// goroutine, and the lookups that name the rest are bounded. CONTRACT.md must
// state both limits beside the rule they limit, so "in every mode" is never
// read as covering those shapes.
func TestContractStatesWhereGoroutineAttributionEnds(t *testing.T) {
	body, err := os.ReadFile("CONTRACT.md")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(strings.Fields(string(body)), " ")
	limits := []string{
		"Attribution ends at a goroutine that exited before anything asked about it",
		"and at one first seen after the run's bounded allowance of lookups (eight for each callback running, shared by the callbacks running together)",
	}
	for _, limit := range limits {
		if !strings.Contains(text, limit) {
			t.Errorf("CONTRACT.md does not state the limit of goroutine attribution: %q", limit)
		}
	}
}
