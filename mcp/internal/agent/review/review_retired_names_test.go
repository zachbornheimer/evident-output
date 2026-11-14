package review_test

import (
	"go/ast"
	"go/parser"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/review"
)

// The 1.x names the migration guide maps live in two lists beside the guide's
// own test, and CONTRACT.md is what 2.0 declares.
const (
	repoRoot          = "../../../.."
	contractPath      = repoRoot + "/CONTRACT.md"
	oneXExportsPath   = repoRoot + "/testdata/v1.2.0-alpha.1_exports.txt"
	oneXConfigPath    = repoRoot + "/testdata/v1.0.0_config_fields.txt"
	freshOneXProgram  = "testdata/legacy_fresh1x/main.go"
	contractRowFormat = "^\\|\\s*`([A-Za-z_][A-Za-z0-9_]*)(?:\\.([A-Za-z_][A-Za-z0-9_]*))?`\\s*\\|"
)

var contractRow = regexp.MustCompile(contractRowFormat)

// nameList is the identifiers of a testdata list, one per line.
func nameList(t *testing.T, path string) []string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for line := range strings.SplitSeq(string(body), "\n") {
		if name := strings.TrimSpace(line); name != "" && !strings.HasPrefix(name, "#") {
			names = append(names, name)
		}
	}
	return names
}

// contractNames is the top-level names CONTRACT.md declares and, per type,
// the members it declares (Config.Name gives {"Config": {"Name"}}).
func contractNames(t *testing.T) (top map[string]bool, members map[string]bool) {
	t.Helper()
	body, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	top, members = map[string]bool{}, map[string]bool{}
	for line := range strings.SplitSeq(string(body), "\n") {
		if m := contractRow.FindStringSubmatch(line); m != nil {
			top[m[1]] = true
			if m[2] != "" {
				members[m[1]+"."+m[2]] = true
			}
		}
	}
	return top, members
}

// retiredTopLevelNames are the exports of 1.2.0-alpha.1 that 2.0 does not
// declare at the top level.
func retiredTopLevelNames(t *testing.T) []string {
	t.Helper()
	declared, _ := contractNames(t)
	var retired []string
	for _, name := range nameList(t, oneXExportsPath) {
		if !declared[name] {
			retired = append(retired, name)
		}
	}
	return retired
}

// retiredConfigFields are the v1.0.0 Config fields CONTRACT.md does not
// declare as Config.<Field>.
func retiredConfigFields(t *testing.T) []string {
	t.Helper()
	_, members := contractNames(t)
	var retired []string
	for _, field := range nameList(t, oneXConfigPath) {
		if !members["Config."+field] {
			retired = append(retired, field)
		}
	}
	return retired
}

// recommended is what a suggestion tells the author to write: the quoted
// call site of "replace X with Y" and "delete X; Y" is dropped.
func recommended(suggestion string) string {
	if rest, ok := strings.CutPrefix(suggestion, "replace "); ok {
		if _, after, found := strings.Cut(rest, " with "); found {
			suggestion = after
		}
	}
	if rest, ok := strings.CutPrefix(suggestion, "delete "); ok {
		_, suggestion, _ = strings.Cut(rest, "; ")
	}
	return suggestion
}

// configLiteralKeys are the keys of every evo.Config{...} literal in text.
func configLiteralKeys(text string) []string {
	var keys []string
	for rest := text; ; {
		_, after, found := strings.Cut(rest, "evo.Config{")
		if !found {
			return keys
		}
		depth, end := 1, 0
		for end = 0; end < len(after) && depth > 0; end++ {
			switch after[end] {
			case '{':
				depth++
			case '}':
				depth--
			}
		}
		if expr, err := parser.ParseExpr("Config{" + after[:end]); err == nil {
			for _, elt := range expr.(*ast.CompositeLit).Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					if id, ok := kv.Key.(*ast.Ident); ok {
						keys = append(keys, id.Name)
					}
				}
			}
		}
		rest = after[end:]
	}
}

// retiredNamesIn is every retired top-level name qualified with evo. and
// every retired Config field keyed in an evo.Config literal in text.
func retiredNamesIn(text string, topLevel, fields []string) []string {
	var found []string
	for _, name := range topLevel {
		if regexp.MustCompile(`\bevo\.` + name + `\b`).MatchString(text) {
			found = append(found, "evo."+name)
		}
	}
	for _, key := range configLiteralKeys(text) {
		for _, field := range fields {
			if key == field {
				found = append(found, "Config."+field)
			}
		}
	}
	return found
}

// legacyCorpus is every 1.x program the review tests migrate.
func legacyCorpus(t *testing.T) map[string]string {
	t.Helper()
	paths, err := filepath.Glob("testdata/legacy_programs/*.go")
	if err != nil {
		t.Fatal(err)
	}
	paths = append(paths, "testdata/legacy_1x/main.go", freshOneXProgram)
	corpus := map[string]string{}
	for _, path := range paths {
		corpus[path] = string(readConvergenceFixture(t, path))
	}
	return corpus
}

// TestSuggestionsNeverContainRetiredNames proves no suggestion review gives
// for a 1.x program recommends a name 2.0 removed: not a top-level name of
// testdata/v1.2.0-alpha.1_exports.txt, and not a Config field of
// testdata/v1.0.0_config_fields.txt, that CONTRACT.md does not declare.
func TestSuggestionsNeverContainRetiredNames(t *testing.T) {
	topLevel, fields := retiredTopLevelNames(t), retiredConfigFields(t)
	for path, src := range legacyCorpus(t) {
		for _, f := range review.GoSource("main.go", src).Findings {
			if names := retiredNamesIn(recommended(f.Suggestion), topLevel, fields); len(names) > 0 {
				t.Errorf("%s: %s L%d suggests retired %v:\n  %s", path, f.RuleID, f.Line, names, f.Suggestion)
			}
		}
	}
}

// TestReviewRewritesEveryMappedOneXName proves a fresh 1.x program, the
// shape the quickstart taught, gets a concrete 2.0 rewrite for each Config
// field and top-level name docs/migration/2.0.md maps, so applying the first
// review's suggestions leaves nothing for a second pass to report as
// "does not compile".
func TestReviewRewritesEveryMappedOneXName(t *testing.T) {
	src := readConvergenceFixture(t, freshOneXProgram)
	findings := review.GoSource("main.go", src).Findings
	for _, want := range []struct{ name, message, suggests string }{
		{"Config.Verbosity", "evo.Main(run) was removed", "Verbose: true"},
		{"Config.Format", "evo.Main(run) was removed", "Output: evo.JSON"},
		{"Config.Title", "evo.Main(run) was removed", `Name: "prune"`},
		{"evo.Skipped(evo.Reason)", "evo.Skipped", `evo.Exclude("protected")`},
		{"evo.Fail printf", "evo.Fail", `evo.Fail(fmt.Sprintf("lint failed: %v", err))`},
		{"evo.EffectDelete", "evo.EffectDelete", "evo.Delete"},
	} {
		var found bool
		for _, f := range findings {
			found = found || strings.Contains(f.Message, want.message) && strings.Contains(f.Suggestion, want.suggests)
		}
		if !found {
			t.Errorf("%s: no finding about %q suggesting %q:\n%s", want.name, want.message, want.suggests, describeFindings(findings))
		}
	}
	rewritten := applyAllSuggestions(t, src, findings, moveStatementsIntoRun)
	if names := retiredNamesIn(rewritten, retiredTopLevelNames(t), retiredConfigFields(t)); len(names) > 0 {
		t.Errorf("applying every suggestion leaves retired %v:\n%s", names, rewritten)
	}
}
