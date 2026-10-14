package review_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// runTableFixture is a program with 14 RunFunc literals in one table. Their
// run bodies reuse Task names ("c", "t", "e", "v") across bodies, and two of
// them (the "duplicate sibling" and "refuse plus misuse" scenarios) declare
// r.Task twice within one body on purpose.
const runTableFixture = "sibling_names/run_table/main.go"

// duplicateLines are the lines of runTableFixture holding the second,
// genuinely duplicate r.Task declaration of its own run body.
var duplicateLines = []int{25, 95}

// TestDuplicateSiblingScopedToOneRunBody proves API-048 compares Task names
// inside one function literal: a name reused by another run body is a
// different parent, while a repeat inside one body still fires.
func TestDuplicateSiblingScopedToOneRunBody(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", filepath.FromSlash(runTableFixture)))
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)

	var lines []int
	for _, f := range review.GoSource("main.go", src).Findings {
		if f.RuleID == "API-048" {
			lines = append(lines, f.Line)
		}
	}
	slices.Sort(lines)
	if !slices.Equal(lines, duplicateLines) {
		t.Errorf("API-048 lines = %v, want exactly the in-body duplicates %v", lines, duplicateLines)
	}

	// Without the two in-body repeats the table reuses names only across
	// bodies, and API-048 must stay silent.
	withoutRepeats := removeLines(src, duplicateLines...)
	for _, f := range review.GoSource("main.go", withoutRepeats).Findings {
		if f.RuleID == "API-048" {
			t.Errorf("API-048 fired across separate run bodies at line %d: %s", f.Line, f.Message)
		}
	}
}

// removeLines drops the given 1-based lines from src.
func removeLines(src string, lines ...int) string {
	var kept []string
	for i, line := range strings.Split(src, "\n") {
		if !slices.Contains(lines, i+1) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// TestDuplicateSiblingScopedToOneReceiver proves two Groups may each hold a
// Task of the same name: the parent differs, so the names are not siblings.
func TestDuplicateSiblingScopedToOneReceiver(t *testing.T) {
	const src = `package p

import (
	"context"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func run(ctx context.Context, r *evo.Run) error {
	build := r.Group("build")
	test := r.Group("test")
	build.Task("lint").Define(func(context.Context) error { return nil })
	test.Task("lint").Define(func(context.Context) error { return nil })
	return nil
}
`
	for _, f := range review.GoSource("main.go", src).Findings {
		if f.RuleID == "API-048" {
			t.Errorf("API-048 fired across two receivers at line %d: %s", f.Line, f.Message)
		}
	}
}
