package evo_test

import (
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestProblemLocation_SetsSourceLocation is C5: the At(path, line, column)
// ProblemOption is renamed Location(...) — it no longer shares a name with
// Output.At(visibility), which confused autocomplete and readers alike.
func TestProblemLocation_SetsSourceLocation(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(nil)}})
	task := out.Task("lint")
	task.Fail("syntax error", evo.Location("main.go", 12, 4))

	problems := out.Snapshot().Tasks[0].Problems
	if len(problems) == 0 || problems[0].Location == nil {
		t.Fatalf("problems = %+v, want a Location", problems)
	}
	loc := problems[0].Location
	if loc.Path != "main.go" || loc.Line != 12 || loc.Column != 4 {
		t.Fatalf("location = %+v, want main.go:12:4", loc)
	}
}
