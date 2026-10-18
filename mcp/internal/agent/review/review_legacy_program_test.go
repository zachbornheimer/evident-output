package review_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// legacyProgramFixture is a 1.x program (the module path without /v2, the
// evo.New options, Task.Block, evo.NextCommand and Task.Skipped) that
// docs/migration/2.0.md maps name by name.
const legacyProgramFixture = "legacy_1x/main.go"

// removedInTwoZero are 1.x names no 2.0 suggestion may recommend. A
// suggestion may still quote them inside the "replace X with Y" call site
// it rewrites.
var removedInTwoZero = regexp.MustCompile(`\b(Block|Blockf|NextCommand|Skipped|Reason|Title|Width|Finish|Init)\b`)

// TestReviewFlagsEveryRemovedCallInLegacyProgram proves review reports each
// call 2.0 removed in a 1.x program exactly once and names the 2.0 spelling
// the migration guide gives for it.
func TestReviewFlagsEveryRemovedCallInLegacyProgram(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", filepath.FromSlash(legacyProgramFixture)))
	if err != nil {
		t.Fatal(err)
	}
	res := review.GoSource("main.go", string(src))

	for _, want := range []struct {
		name, line, message, suggests string
	}{
		{"import path", `evo "github.com/zachbornheimer/evident-output"`, "module path", `github.com/zachbornheimer/evident-output/v2`},
		{"Task.Block", `t.Block(`, "Task.Block", `evo.Refuse("no remote", evo.Next(evo.Action{Command: []string{"git", "remote", "add"}}))`},
		{"evo.NextCommand", `evo.NextCommand("git"`, "evo.NextCommand", `evo.Next(evo.Action{Command: []string{"git", "remote", "add"}})`},
		{"Task.Skipped", `t.Skipped(`, "Task.Skipped", `evo.Exclude("protected")`},
		{"evo.Title", `evo.Title("tool")`, "evo.Title", `Name: "tool"`},
		{"evo.Width", `evo.Width(100)`, "evo.Width", `no 2.0 equivalent`},
	} {
		line := lineOf(t, string(src), want.line)
		var hits []review.Finding
		for _, f := range res.Findings {
			if f.RuleID == "API-032" && f.Line == line && strings.Contains(f.Message, want.message) &&
				strings.Contains(f.Suggestion, want.suggests) {
				hits = append(hits, f)
			}
		}
		if len(hits) != 1 {
			t.Errorf("%s (line %d): want one API-032 finding about %q suggesting %q, got %d\nall findings:\n%s",
				want.name, line, want.message, want.suggests, len(hits), describeFindings(res.Findings))
		}
	}

	for _, f := range res.Findings {
		if name := removedNameSuggested(f.Suggestion); name != "" {
			t.Errorf("%s at line %d suggests the removed name %s:\n%s", f.RuleID, f.Line, name, f.Suggestion)
		}
	}
}

// TestReviewLegacyProgramAtOneOneSeesNoTwoZeroRemoval proves a project
// pinned to 1.1 is not told about removals 2.0 made: Task.Block,
// Task.Skipped, evo.NextCommand and the module path are live there.
func TestReviewLegacyProgramAtOneOneSeesNoTwoZeroRemoval(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("testdata", filepath.FromSlash(legacyProgramFixture)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range goSourceOneOne("main.go", string(src)).Findings {
		if strings.Contains(f.Message, "removed in 2.0") || strings.Contains(f.Message, "module path") {
			t.Errorf("a 1.1 pin got a 2.0 finding: %s L%d: %s", f.RuleID, f.Line, f.Message)
		}
	}
}

// TestReviewTwoZeroProgramIsCleanOfRemovedCalls proves the 2.0 spellings the
// legacy findings suggest raise no finding themselves.
func TestReviewTwoZeroProgramIsCleanOfRemovedCalls(t *testing.T) {
	const src = `package main

import (
	"context"
	"os"

	evo "github.com/zachbornheimer/evident-output/v2"
)

func main() { os.Exit(evo.Main(evo.Config{Name: "tool", Output: evo.Plain}, run)) }

func run(ctx context.Context, r *evo.Run) error {
	r.Task("build").Define(func(context.Context) error {
		return evo.Refuse("no remote", evo.Next(evo.Action{Command: []string{"git", "remote", "add"}}))
	})
	r.Task("publish").Define(func(context.Context) error { return evo.Exclude("protected") })
	return nil
}
`
	if res := review.GoSource("main.go", src); len(res.Findings) != 0 {
		t.Fatalf("2.0 program has findings:\n%s", describeFindings(res.Findings))
	}
}

// lineOf is the 1-based line of src that holds marker.
func lineOf(t *testing.T, src, marker string) int {
	t.Helper()
	at := strings.Index(src, marker)
	if at < 0 {
		t.Fatalf("fixture has no %q", marker)
	}
	return 1 + strings.Count(src[:at], "\n")
}

// removedNameSuggested is the first removed name a suggestion recommends,
// ignoring the call site a "replace X with Y" suggestion quotes as X.
func removedNameSuggested(suggestion string) string {
	if rest, ok := strings.CutPrefix(suggestion, "replace "); ok {
		if _, after, found := strings.Cut(rest, " with "); found {
			suggestion = after
		}
	}
	if rest, ok := strings.CutPrefix(suggestion, "delete "); ok {
		_, suggestion, _ = strings.Cut(rest, "; ")
	}
	return removedInTwoZero.FindString(suggestion)
}

func describeFindings(findings []review.Finding) string {
	var b strings.Builder
	for _, f := range findings {
		fmt.Fprintf(&b, "  %s L%d: %s\n", f.RuleID, f.Line, f.Suggestion)
	}
	return b.String()
}

// goSourceOneOne reviews src as a project pinned to the 1.1 release, where
// Task.Block, Task.Skipped and evo.NextCommand are live API. The tests of the
// 1.1 migration rewrites state their dialect with it.
func goSourceOneOne(filename, src string) review.Result {
	return review.GoSourceAt(filename, src, "v1.1.0")
}
