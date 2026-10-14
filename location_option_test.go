package evo

import (
	"testing"
)

// TestProblemLocation_SetsSourceLocation is C5: the At(path, line, column)
// ProblemOption is renamed Location(...) — it no longer shares a name with
// Output.At(visibility), which confused autocomplete and readers alike.
func TestProblemLocation_SetsSourceLocation(t *testing.T) {
	out := initRun(Config{Facades: Facades{Stderr: nil}})
	task := out.Task("lint")
	task.fail("syntax error", location("main.go", 12, 4))

	problems := out.snapshot().Tasks[0].Problems
	if len(problems) == 0 || problems[0].Location == nil {
		t.Fatalf("problems = %+v, want a Location", problems)
	}
	loc := problems[0].Location
	if loc.Path != "main.go" || loc.Line != 12 || loc.Column != 4 {
		t.Fatalf("location = %+v, want main.go:12:4", loc)
	}
}
