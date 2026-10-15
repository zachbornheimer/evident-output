package review_test

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// legacyProgramDir holds one complete v1.0.0 program per file. Adding a
// program there is the whole change a new case needs: every file is migrated
// by TestEveryLegacyProgramConverges.
const legacyProgramDir = "testdata/legacy_programs"

// authorStep is one edit only the program's author can make, written in the
// fixture as "// author-step: OLD => NEW". OLD must still stand in the
// program after every suggested rewrite is applied: a step whose OLD review
// already rewrote is stale.
var authorStep = regexp.MustCompile(`(?m)^// author-step: (.*?) =>(?: (.*))?$`)

// TestEveryLegacyProgramConverges proves each v1.0.0 program in
// testdata/legacy_programs migrates through review alone: every suggestion
// applied, then only the author steps the fixture names, gives a program that
// type-checks against the 2.0 package and re-reviews clean.
func TestEveryLegacyProgramConverges(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(legacyProgramDir, "*.go"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no legacy programs under %s: %v", legacyProgramDir, err)
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) { requireLegacyProgramConverges(t, path) })
	}
}

// readLegacyProgram is a fixture's program without its author-step comments
// (they quote the code they edit), and the steps themselves.
func readLegacyProgram(t *testing.T, path string) (src string, steps [][]string) {
	t.Helper()
	raw := readConvergenceFixture(t, path)
	return authorStep.ReplaceAllString(raw, ""), authorStep.FindAllStringSubmatch(raw, -1)
}

// requireLegacyProgramConverges is the whole loop on one program and returns
// the converged source.
func requireLegacyProgramConverges(t *testing.T, path string) string {
	t.Helper()
	src, steps := readLegacyProgram(t, path)
	rewritten := applyAllSuggestions(t, src, review.GoSource("main.go", src).Findings, moveStatementsIntoRun)
	for _, m := range steps {
		old, replacement := m[1], m[2]
		if strings.Count(rewritten, old) != 1 {
			t.Fatalf("%s: author step %q needs its OLD exactly once in the rewritten program, found %d:\n%s", path, old, strings.Count(rewritten, old), rewritten)
		}
		rewritten = strings.Replace(rewritten, old, replacement, 1)
	}
	requireTypeChecksAgainstTwoZero(t, rewritten, "")
	if again := review.GoSource("main.go", rewritten); len(again.Findings) != 0 || again.RecheckRequired {
		t.Fatalf("%s: review is not clean after the rewrites and author steps:\n%s\nrewritten source:\n%s", path, describeFindings(again.Findings), rewritten)
	}
	return rewritten
}

// legacyNineEntries are the legacy9 constructs review round 9 left without a
// rewrite, each with the finding that must carry the 2.0 spelling the guide
// table gives (docs/migration/2.0.md).
var legacyNineEntries = []struct{ name, message, spelling string }{
	{"AssumeYes", "Output.Confirm was removed", "(len(os.Args) > 1 || r.Confirm("},
	{"ConfirmDetail", "Output.Confirm was removed", `evo.Detail("2 tags")`},
	{"Count", "evo.Count was removed", `evo.Detail("2 tag")`},
	{"Location", "evo.Location was removed", `evo.On("NOTES.md:1:1")`},
	{"Command", "Task.Next was removed", `evo.Action{Command: []string{"git", "push", "--tags"}}`},
	{"Output.ResultWriter", "Output.ResultWriter was removed", "r.ResultWriter()"},
	{"Output.Run", "Output.Run(ctx, run) was removed", "func(ctx context.Context, r *evo.Run) error"},
	{"Config.Subject", "Config.Subject was removed", `r.Fact("subject", "~/repo")`},
	{"Config.Stdout", "Output.Run(ctx, run) was removed", "Facades: evo.Facades{Stdout: os.Stderr}"},
}

// TestLegacyNineConverges holds legacy9 to what the guide table
// docs/migration/2.0.md maps: each construct round 9 left unmigrated carries
// an applicable rewrite, the rewrites name no removed API, and the program
// converges.
func TestLegacyNineConverges(t *testing.T) {
	path := filepath.Join(legacyProgramDir, "legacy9.go")
	src, _ := readLegacyProgram(t, path)
	findings := review.GoSource("main.go", src).Findings
	for _, entry := range legacyNineEntries {
		var found bool
		for _, f := range findings {
			if strings.Contains(f.Message, entry.message) && strings.Contains(f.Suggestion, entry.spelling) {
				found = true
			}
		}
		if !found {
			t.Errorf("review gives no %s the spelling %q:\n%s", entry.name, entry.spelling, describeFindings(findings))
		}
	}

	rewritten := requireLegacyProgramConverges(t, path)
	for _, removed := range []string{"evo.Location", "evo.Count", "evo.Command", "evo.AssumeYes", "evo.ConfirmDetail", "Label: evo."} {
		if strings.Contains(rewritten, removed) {
			t.Errorf("the converged program still uses %s:\n%s", removed, rewritten)
		}
	}
}
