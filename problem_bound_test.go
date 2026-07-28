package evo_test

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestHumanProblemList_IsBounded(t *testing.T) {
	var buf bytes.Buffer
	out := evo.NewWithOptions(
		evo.Title("tool"),
		evo.To(&buf),
		evo.Plain(),
		evo.NoColor(),
	)
	t.Cleanup(func() { _ = out.Close() })

	problems := make([]evo.Problem, 0, 8)
	for i := 0; i < 8; i++ {
		problems = append(problems, evo.Problem{
			Subject: fmt.Sprintf("path-%d", i),
			Summary: "failed",
		})
	}
	out.Item("placement").FailedBy(problems...)
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "and 3 more failures") {
		t.Fatalf("want omission line:\n%s", got)
	}
	// Snapshot retains all problems.
	snap := out.Snapshot()
	if len(snap.Items) != 1 || len(snap.Items[0].Problems) != 8 {
		t.Fatalf("snapshot must retain all problems, got %#v", snap.Items)
	}
}
