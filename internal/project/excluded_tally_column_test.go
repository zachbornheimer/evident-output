package project_test

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// TestExcludedTallyColumnIsStable pins that a Group's folded Excluded tally
// sits in the Group's child column whether the Group has only Excluded items,
// builds them, or also keeps a work child that survives the fold.
func TestExcludedTallyColumnIsStable(t *testing.T) {
	var stderr bytes.Buffer
	exclude := func(context.Context) error { return evo.Exclude("protected") }
	evo.Execute(context.Background(), evo.Config{
		Name: "tally", Output: evo.Plain,
		Facades: evo.Facades{Stdout: &bytes.Buffer{}, Stderr: &stderr},
	}, func(_ context.Context, r *evo.Run) error {
		only := r.Group("only items")
		for i := range 20 {
			only.Task(fmt.Sprintf("p%d", i)).Define(exclude)
		}
		r.Group("built items").Build(func(g *evo.Group) {
			for i := range 20 {
				g.Task(fmt.Sprintf("b%d", i)).Define(exclude)
			}
		})
		beside := r.Group("beside work")
		for i := range 20 {
			beside.Task(fmt.Sprintf("m%d", i)).Define(exclude)
		}
		beside.Task("ok").Define(func(context.Context) error { return nil })
		return nil
	})

	const wantPrefix = "   ○ excluded 20"
	tallies := 0
	for line := range strings.SplitSeq(stderr.String(), "\n") {
		if !strings.Contains(line, "○ excluded") {
			continue
		}
		tallies++
		if !strings.HasPrefix(line, wantPrefix) {
			t.Errorf("tally %q is not in the Group child column (%q)", line, wantPrefix)
		}
	}
	if tallies != 3 {
		t.Fatalf("want one tally per Group (3), got %d in:\n%s", tallies, stderr.String())
	}
}
