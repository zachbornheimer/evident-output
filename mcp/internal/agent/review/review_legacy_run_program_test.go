package review_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/rules"
)

// legacyRunProgramFixture is the second canonical 1.x program (release gate 8):
// the package-level entry points legacy_1x/main.go does not use. Its good_
// twin is the same program with every rewrite applied.
const (
	legacyRunProgramFixture    = "dialect_2_0/detectRemovedTwoZero/legacy_run_program.go"
	convergedRunProgramFixture = "dialect_2_0/detectRemovedTwoZero/good_converged_legacy_run_program.go"
	migrationGuideFile         = "../../../../docs/migration/2.0.md"
)

// legacyRunProgramEntryPoints are the removed names the program uses, each
// with the substring of the finding that must carry an applicable rewrite.
// Package Warn stays advice: a run-level warning has no Task to land on.
var legacyRunProgramEntryPoints = []string{
	"evo.Run", "evo.Fact", "evo.Verbose", "ExecSpec.Basis", "evo.Exec",
	"evo.Confirm", "evo.PolicyFlag", "Task.Key", "Task.Bytes",
}

// warnByHand is the one step the review names without being able to apply:
// the warning belongs to the upload Task.
const (
	warnCall    = `evo.Warn("cache headers not set")`
	warnOnTask  = `upload.Problem("cache headers not set")`
	warnMessage = "evo.Warn was removed"
)

// TestLegacyEightConvergesAndRetiredSpecFieldsMatchGuide proves a v1.0.0
// program that uses evo.Run, ExecSpec.Basis, package Confirm and PolicyFlag,
// Fact, Verbose, Key and Bytes migrates through review alone: every entry
// point carries a rewrite, the applied program type-checks against the
// 2.0 package and re-reviews clean, and the Spec.Field names the rewrites and
// the retired table give are the ones docs/migration/2.0.md gives.
func TestLegacyEightConvergesAndRetiredSpecFieldsMatchGuide(t *testing.T) {
	path := filepath.Join("testdata", filepath.FromSlash(legacyRunProgramFixture))
	src := readConvergenceFixture(t, path)
	findings := review.GoSource("main.go", src).Findings

	for _, entry := range legacyRunProgramEntryPoints {
		var found bool
		for _, f := range findings {
			if strings.Contains(f.Message, entry) {
				found = true
				if !isApplicable(src, f) {
					t.Errorf("%s has no applicable rewrite: %s", entry, f.Suggestion)
				}
			}
		}
		if !found {
			t.Errorf("review reports nothing about %s:\n%s", entry, describeFindings(findings))
		}
	}

	rewritten := applyAllSuggestions(t, src, findings, nil)
	if !strings.Contains(rewritten, warnCall) {
		t.Fatalf("the rewritten program lost %s, which only the author can place:\n%s", warnCall, rewritten)
	}
	rewritten = strings.Replace(rewritten, warnCall, warnOnTask, 1)
	requireTypeChecksAgainstTwoZero(t, rewritten, "")
	if again := review.GoSource("main.go", rewritten); len(again.Findings) != 0 || again.RecheckRequired {
		t.Fatalf("review is not clean after applying every suggestion:\n%s\nrewritten source:\n%s", describeFindings(again.Findings), rewritten)
	}

	requireTypeChecksAgainstTwoZero(t, readConvergenceFixture(t, filepath.Join("testdata", filepath.FromSlash(convergedRunProgramFixture))), "")

	cell := migrationGuideCell(t, "Basis")
	for _, name := range []string{"Fingerprint", "FSPath", "Value", "App"} {
		for _, sym := range rules.RetiredSymbols() {
			if sym.Contract == name {
				requireSpecFieldsIn(t, "retired table row "+name, sym.Replacement, cell)
			}
		}
	}
	for _, f := range findings {
		if strings.Contains(f.Message, "ExecSpec.Basis") {
			requireSpecFieldsIn(t, "review suggestion for ExecSpec.Basis", f.Suggestion, cell)
		}
	}
	if warn := strings.Join(messages(findings), "\n"); !strings.Contains(warn, warnMessage) {
		t.Errorf("the program's package Warn draws no finding:\n%s", warn)
	}
}

func messages(findings []review.Finding) []string {
	out := make([]string, len(findings))
	for i, f := range findings {
		out[i] = f.Message
	}
	return out
}

var specField = regexp.MustCompile(`\b([A-Z]\w*)\.([A-Z]\w*)\b`)

// requireSpecFieldsIn fails when text names a Type.Field that cell, the 2.0
// column of the guide row, does not.
func requireSpecFieldsIn(t *testing.T, source, text, cell string) {
	t.Helper()
	if rest, ok := strings.CutPrefix(text, "replace "); ok {
		if _, after, found := strings.Cut(rest, " with "); found {
			text = after
		}
	}
	for _, m := range specField.FindAllString(text, -1) {
		if !strings.Contains(cell, m) {
			t.Errorf("%s names %s, but docs/migration/2.0.md maps it to %q", source, m, cell)
		}
	}
}

// migrationGuideCell is the 2.0 cell of the guide row whose 1.x cell lists name.
func migrationGuideCell(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.FromSlash(migrationGuideFile))
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(string(body), "\n") {
		cells := strings.Split(line, "|")
		if len(cells) >= 4 && strings.Contains(cells[1], "`"+name+"`") {
			return strings.TrimSpace(cells[2])
		}
	}
	t.Fatalf("docs/migration/2.0.md has no row for %s", name)
	return ""
}
