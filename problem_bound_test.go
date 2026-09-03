package evo_test

import (
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestHumanProblemList_IsBounded pins the plain-projection omission rule
// (maxVisibleProblems) directly against a hand-built Snapshot — a single
// Task verb call now produces exactly one Problem, so a multi-problem
// Snapshot is constructed via the public Snapshot/TaskSnapshot fields
// (§25 model) rather than a deprecated bulk-attach verb.
func TestHumanProblemList_IsBounded(t *testing.T) {
	problems := make([]evo.Problem, 0, 8)
	for i := 0; i < 8; i++ {
		problems = append(problems, evo.Problem{
			Subject: fmt.Sprintf("path-%d", i),
			Summary: "failed",
		})
	}
	snap := evo.Snapshot{
		Subject: "tool",
		Tasks: []evo.TaskSnapshot{
			{Name: "placement", State: evo.Failed, Problems: problems},
		},
	}

	out, err := evo.RenderPlain(snap, evo.PlainOptions{NoColor: true})
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if !strings.Contains(got, "and 3 more failures") {
		t.Fatalf("want omission line:\n%s", got)
	}
	// The Snapshot itself retains all problems; only plain projection bounds display.
	if len(snap.Tasks[0].Problems) != 8 {
		t.Fatalf("snapshot must retain all problems, got %#v", snap.Tasks[0].Problems)
	}
}
