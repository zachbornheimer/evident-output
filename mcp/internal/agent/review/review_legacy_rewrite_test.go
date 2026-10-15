package review_test

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// The v1.0.0 README quickstart and the round-3 legacy program are the two
// canonical 1.x programs. Review's suggestions must be precise enough to
// apply: each is "replace OLD with NEW", "delete OLD; ...", or a documented
// manual step this test performs once (add an import, move statements into
// run). Applying them all must give a program that type-checks against the
// real 2.0 package and that review finds clean.

// quickstartCompanions are the application symbols the README quickstart
// calls but never declares.
const quickstartCompanions = `package main

var packages []string

func install(string) error
func removeStaleBranches() error
`

func TestLegacyQuickstartRewritesTypeCheck(t *testing.T) {
	rewritten := applyReviewRewrites(t, "legacy_v1_quickstart/main.go.txt", []string{
		"evo.Init", "evo.Main(run)", "evo.Println", "evo.Printf", "evo.Group was removed",
		"func run(ctx) error", "Task.Done", "Task.Block", "Task.Delete",
	}, nil)
	requireTypeChecksAgainstTwoZero(t, rewritten, quickstartCompanions)
}

func TestLegacyOneXProgramRewritesTypeCheck(t *testing.T) {
	rewritten := applyReviewRewrites(t, "legacy_1x/main.go", []string{
		"evo.New", "Output.Task", "Output.Finish", "Task.Block", "Task.Skipped", "evo.NextCommand",
	}, moveStatementsIntoRun)
	requireTypeChecksAgainstTwoZero(t, rewritten, "")
}

// applyReviewRewrites reviews the fixture, applies every suggestion
// mechanically (outermost first, so a call is rewritten whole before its
// arguments), performs the documented manual steps once, and returns the
// rewritten source after proving review is clean on it. wantMessages are
// substrings the first review's messages must cover.
func applyReviewRewrites(t *testing.T, fixture string, wantMessages []string, moveIntoRun func(*testing.T, string) string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", filepath.FromSlash(fixture)))
	if err != nil {
		t.Fatal(err)
	}
	src := string(raw)
	findings := review.GoSource("main.go", src).Findings

	var messages []string
	for _, f := range findings {
		messages = append(messages, f.Message)
	}
	all := strings.Join(messages, "\n")
	for _, want := range wantMessages {
		if !strings.Contains(all, want) {
			t.Errorf("review reports nothing about %q; messages:\n%s", want, all)
		}
	}

	src = applyAllSuggestions(t, src, findings, moveIntoRun)

	if again := review.GoSource("main.go", src); len(again.Findings) != 0 || again.RecheckRequired {
		t.Fatalf("review is not clean after applying every suggestion:\n%s\nrewritten source:\n%s", describeFindings(again.Findings), src)
	}
	return src
}

// applyAllSuggestions applies every mechanically applicable suggestion in
// findings to src, outermost first, then performs the documented manual
// steps once (add the named imports, move statements into run) and returns
// the rewritten source. A suggestion that is guidance only changes nothing.
func applyAllSuggestions(t *testing.T, src string, findings []review.Finding, moveIntoRun func(*testing.T, string) string) string {
	t.Helper()
	findings = slices.Clone(findings)
	sort.SliceStable(findings, func(i, j int) bool { return len(findings[i].Suggestion) > len(findings[j].Suggestion) })
	var imports []string
	var moved bool
	for _, f := range findings {
		switch {
		case strings.HasPrefix(f.Suggestion, "add the imports "):
			for _, quoted := range regexp.MustCompile(`"(\w+)"`).FindAllStringSubmatch(f.Suggestion, -1) {
				imports = append(imports, quoted[1])
			}
			continue
		case strings.Contains(f.Suggestion, "then move the statements"):
			moved = true
		}
		if next, ok := applySuggestion(src, f.Suggestion); ok {
			src = next
		}
	}
	if moved {
		if moveIntoRun == nil {
			t.Fatal("review asks to move statements into run, but the fixture has no manual step for it")
		}
		src = moveIntoRun(t, src)
	}
	return addImports(src, imports)
}

// applySuggestion applies one "replace OLD with NEW" or "delete OLD; why"
// suggestion to src. " with " and "; " may also occur inside the code, so
// OLD is the first split that is lexically complete and occurs in src.
func applySuggestion(src, suggestion string) (string, bool) {
	if next, ok := tryApplyReplace(src, suggestion); ok {
		return next, true
	}
	rest, ok := strings.CutPrefix(suggestion, "delete ")
	if !ok {
		return "", false
	}
	for offset := 0; ; {
		i := strings.Index(rest[offset:], "; ")
		if i < 0 {
			return "", false
		}
		cut := offset + i
		offset = cut + 1
		pattern, closed := leftPattern(rest[:cut])
		if !closed {
			continue
		}
		if loc := regexp.MustCompile(pattern).FindStringIndex(src); loc != nil {
			return src[:loc[0]] + src[loc[1]:], true
		}
	}
}

// addImports adds each missing standard-library import to the file's first
// import block.
func addImports(src string, paths []string) string {
	for _, path := range paths {
		quoted := fmt.Sprintf("%q", path)
		if !strings.Contains(src, quoted) {
			src = strings.Replace(src, "import (\n", "import (\n\t"+quoted+"\n", 1)
		}
	}
	return src
}

// moveStatementsIntoRun is the manual step "move the statements that
// followed evo.New into func run(ctx, r) error ending in return nil": every
// statement of main but the last becomes the body of run.
func moveStatementsIntoRun(t *testing.T, src string) string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "main.go", src, 0)
	if err != nil {
		t.Fatalf("parse rewritten source: %v\n%s", err, src)
	}
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "main" || len(fd.Body.List) < 2 {
			continue
		}
		text := func(a, b token.Pos) string { return src[fset.Position(a).Offset:fset.Position(b).Offset] }
		last := fd.Body.List[len(fd.Body.List)-1]
		body := text(fd.Body.List[0].Pos(), last.Pos())
		main := "func main() {\n\t" + text(last.Pos(), last.End()) + "\n}\n\nfunc run(ctx context.Context, r *evo.Run) error {\n\t" +
			strings.TrimSpace(body) + "\n\treturn nil\n}"
		return text(file.Pos(), fd.Pos()) + main + src[fset.Position(fd.End()).Offset:]
	}
	t.Fatalf("no main to move statements out of:\n%s", src)
	return ""
}

// requireTypeChecksAgainstTwoZero type-checks src (plus the companion
// declarations of symbols it leaves to other files) against the real 2.0
// package, with every error fatal: unused names and imports included.
func requireTypeChecksAgainstTwoZero(t *testing.T, src, companions string) {
	t.Helper()
	if problems := typeCheckProblemsAgainstTwoZero(t, src, companions); len(problems) > 0 {
		t.Fatalf("the rewritten program does not compile against 2.0:\n  %s\nsource:\n%s", strings.Join(problems, "\n  "), src)
	}
}

// typeCheckProblemsAgainstTwoZero returns every type error src has against
// the real 2.0 package.
func typeCheckProblemsAgainstTwoZero(t *testing.T, src, companions string) []string {
	t.Helper()
	fset := token.NewFileSet()
	var files []*ast.File
	for name, text := range map[string]string{"main.go": src, "companions.go": companions} {
		if text == "" {
			continue
		}
		file, err := parser.ParseFile(fset, name, text, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v\n%s", name, err, text)
		}
		files = append(files, file)
	}
	var problems []string
	conf := types.Config{Importer: importer.ForCompiler(fset, "source", nil), Error: func(err error) { problems = append(problems, err.Error()) }}
	_, _ = conf.Check("main", fset, files, nil)
	return problems
}
