// Package mcp_test binds contract §21 MCP guidance rules to the taught
// material the server serves.
package mcp_test

import (
	"os"
	"strings"
	"testing"
)

const teachingLadderPath = "../../../mcp/docs/teaching-ladder.md"

// contractTeachingOrder is contract §21's teaching order, each step named
// by the phrase the ladder uses for it.
var contractTeachingOrder = []string{
	"1. Task + Define",
	"2. Group / Sequence",
	"3. evo.File",
	"4. evo.Exec",
	"5. FileSpec.Basis",
	"6. After for exceptional",
	"7. Facts / warnings / Effects",
	"8. Task.Verify",
}

func TestC21_002_TeachingLadderFollowsTheContractOrder(t *testing.T) {
	raw, err := os.ReadFile(teachingLadderPath)
	if err != nil {
		t.Fatal(err)
	}
	ladder := string(raw)
	previous, previousStep := -1, ""
	for _, step := range contractTeachingOrder {
		at := strings.Index(ladder, step)
		if at < 0 {
			t.Fatalf("teaching ladder never mentions %q", step)
		}
		if at < previous {
			t.Errorf("teaching ladder teaches %q before %q, contract §21 orders them the other way", step, previousStep)
		}
		previous, previousStep = at, step
	}
}
