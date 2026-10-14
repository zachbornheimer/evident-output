package evo_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

const (
	excludedTallyItems  = 20
	excludedTallyReason = "protected"
)

// excludedTallyProgram declares excludedTallyItems per-item Tasks that each
// Exclude with excludedTallyReason, beside one Task that ran.
func excludedTallyProgram(_ context.Context, r *evo.Run) error {
	refs := r.Group("refs")
	for i := range excludedTallyItems {
		refs.Task(fmt.Sprintf("item-%02d", i)).Define(func(context.Context) error {
			return evo.Exclude(excludedTallyReason)
		})
	}
	refs.Task("fetch").Define(func(context.Context) error { return nil })
	return nil
}

// TestExcludedItemsFoldIntoOneTally pins CONTRACT.md §8: Excluded Tasks of one
// Group that share an Exclude reason print one tally line carrying the count
// and the reason, Verbose names the items, and JSON keeps every item.
func TestExcludedItemsFoldIntoOneTally(t *testing.T) {
	t.Setenv("EVO_VERBOSE", "")
	for _, mode := range []evo.Output{evo.Human, evo.Plain} {
		t.Run(outputName[mode], func(t *testing.T) {
			quiet := executeContract(context.Background(), t, evo.Config{Output: mode}, excludedTallyProgram)
			lines := linesMentioning(quiet.Text(), excludedTallyReason)
			if len(lines) != 1 {
				t.Fatalf("%d lines name the reason %q, want one tally line:\n%s", len(lines), excludedTallyReason, quiet.Text())
			}
			if !strings.Contains(lines[0], fmt.Sprint(excludedTallyItems)) {
				t.Errorf("tally line %q does not carry the count %d", lines[0], excludedTallyItems)
			}
			if strings.Contains(quiet.Text(), "item-00") {
				t.Errorf("an Excluded item prints its own row at normal verbosity:\n%s", quiet.Text())
			}

			verbose := executeContract(context.Background(), t, evo.Config{Output: mode, Verbose: true}, excludedTallyProgram)
			for i := range excludedTallyItems {
				if name := fmt.Sprintf("item-%02d", i); !strings.Contains(verbose.Text(), name) {
					t.Fatalf("Verbose does not name %s:\n%s", name, verbose.Text())
				}
			}
		})
	}

	t.Run("json keeps every item", func(t *testing.T) {
		run := executeContract(context.Background(), t, evo.Config{Output: evo.JSON}, excludedTallyProgram)
		excluded := 0
		for _, task := range documentTasks(t, runDocument(t, run.Stdout)) {
			if task["outcome"] == string(evo.Excluded) {
				excluded++
			}
		}
		if excluded != excludedTallyItems {
			t.Errorf("evo.run holds %d Excluded Tasks, want %d", excluded, excludedTallyItems)
		}
	})
}

// linesMentioning returns the lines of text that contain word.
func linesMentioning(text, word string) []string {
	var out []string
	for line := range strings.SplitSeq(text, "\n") {
		if strings.Contains(line, word) {
			out = append(out, line)
		}
	}
	return out
}
