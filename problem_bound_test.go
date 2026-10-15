package evo

import (
	"fmt"
	"strings"
	"testing"
)

// TestHumanProblemList_IsBounded pins the plain-projection omission rule
// (maxVisibleProblems) directly against a hand-built Snapshot — a single
// Task verb call now produces exactly one Problem, so a multi-problem
// Snapshot is constructed via the public Snapshot/TaskSnapshot fields
// (§25 model) rather than a deprecated bulk-attach verb.
func TestHumanProblemList_IsBounded(t *testing.T) {
	problems := make([]problem, 0, 8)
	for i := range 8 {
		problems = append(problems, problem{
			Subject: fmt.Sprintf("path-%d", i),
			Summary: "failed",
		})
	}
	snap := snapshot{
		Subject: "tool",
		Tasks: []taskSnapshot{
			{Name: "placement", State: failed, Problems: problems},
		},
	}

	out, err := renderPlain_1(snap, plainOptions{noColor: true})
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
