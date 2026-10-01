package review_test

import (
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
	"github.com/zachbornheimer/evident-output/internal/agent/rules"
	"github.com/zachbornheimer/evident-output/internal/agent/vocabulary"
)

func vocabularyPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "testdata", "api_vocabulary.txt")
}

func TestMigration1_1EveryRemovedNameHasDirtyRewriteCleanFixture(t *testing.T) {
	entries, err := vocabulary.LoadVocabulary(vocabularyPath(t))
	if err != nil {
		t.Fatal(err)
	}
	fixtures := migration1_1Fixtures()
	var missing []string
	var nRemoved int
	for _, e := range entries {
		if e.Class != vocabulary.ClassRemoved {
			continue
		}
		nRemoved++
		fx, ok := fixtures[e.Name]
		if !ok {
			missing = append(missing, e.Name)
			t.Errorf("missing fixture for removed name %s", e.Name)
			continue
		}
		checkRemovedNameFixture(t, e.Name, fx)
	}
	if len(missing) > 0 {
		t.Fatalf("removed names without fixtures (%d of %d): %s", len(missing), nRemoved, strings.Join(missing, ", "))
	}
	if nRemoved == 0 {
		t.Fatal("vocabulary loaded no class=removed names")
	}
	t.Logf("removed names=%d fixtures=%d", nRemoved, len(fixtures))
}

func checkRemovedNameFixture(t *testing.T, name string, fx migrationFixture) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		res := review.GoSource(name+".go", fx.dirty)
		found := migrationFindings(res)
		if len(found) == 0 {
			t.Fatalf("dirty %s: want 1.1 migration finding, got %+v", name, res.Findings)
		}
		hit, ok := findingAbout(found, name)
		if !ok {
			t.Fatalf("dirty %s: no finding about %q; got %q", name, name, joinSuggestions(found))
		}
		if _, ok := rules.Explain(hit.RuleID); !ok {
			t.Fatalf("rules.Explain(%q) failed", hit.RuleID)
		}
		canonical := appliedSuggestion(t, name, fx, hit)
		if hasRetiredSpelling(fx.clean, name) {
			t.Fatalf("canonical rewrite for %s still names the retired spelling:\n%s", name, fx.clean)
		}
		clean := review.GoSource(name+".go", fx.clean)
		if n := migrationFindings(clean); len(n) != 0 || clean.RecheckRequired {
			t.Fatalf("canonical rewrite is dirty: recheck=%v findings=%+v\n%s",
				clean.RecheckRequired, clean.Findings, fx.clean)
		}
		if leftover := review.GoSource(name+".go", canonical); len(migrationFindings(leftover)) != 0 || leftover.RecheckRequired {
			t.Fatalf("rewritten source still dirty: recheck=%v findings=%+v\n%s",
				leftover.RecheckRequired, leftover.Findings, canonical)
		}
		if len(clean.Findings) != 0 {
			t.Fatalf("canonical rewrite has non-migration findings: %+v\n%s", clean.Findings, fx.clean)
		}
	})
}

// appliedSuggestion applies the finding's own suggestion to the dirty source
// and requires it to yield exactly the fixture's clean code. It never falls
// back to the hand-written clean. A suggestion that introduces the remedy
// placeholder must keep the review open until the fixture's fill replaces it.
func appliedSuggestion(t *testing.T, name string, fx migrationFixture, hit review.Finding) string {
	t.Helper()
	if _, fragment := fragmentSuggestions[name]; fragment {
		return fx.clean
	}
	if _, guidance := guidanceOnlySuggestions[name]; guidance {
		if strings.HasPrefix(hit.Suggestion, "replace ") || hit.Suggestion == "" {
			t.Fatalf("%s: want guidance, not an applicable replace that could ship broken code: %q", name, hit.Suggestion)
		}
		return fx.clean
	}
	applied, ok := tryApplyReplace(fx.dirty, hit.Suggestion)
	if !ok {
		t.Fatalf("%s: suggestion is not an applicable single replace: %q", name, hit.Suggestion)
	}
	if fx.fill != "" {
		if !strings.Contains(applied, remedyPlaceholder) {
			t.Fatalf("%s: fixture expects the placeholder fallback; applied:\n%s", name, applied)
		}
		if !review.GoSource(name+".go", applied).RecheckRequired {
			t.Fatalf("%s: placeholder summary shipped with recheck closed:\n%s", name, applied)
		}
		applied = strings.Replace(applied, remedyPlaceholder, fx.fill, 1)
	}
	if applied != fx.clean {
		t.Fatalf("%s: applying the suggestion %q does not yield the expected clean code\n got:\n%s\nwant:\n%s",
			name, hit.Suggestion, applied, fx.clean)
	}
	return applied
}

func migrationFindings(res review.Result) []review.Finding {
	var found []review.Finding
	for _, f := range res.Findings {
		if f.RuleID == "API-032" || strings.Contains(f.Message, "removed in 1.1") {
			found = append(found, f)
		}
	}
	return found
}

func findingAbout(fs []review.Finding, name string) (review.Finding, bool) {
	for _, f := range fs {
		if strings.Contains(f.Message, name) || strings.Contains(f.Suggestion, name) {
			return f, true
		}
	}
	return review.Finding{}, false
}

func tryApplyReplace(src, suggestion string) (string, bool) {
	rest, ok := strings.CutPrefix(suggestion, "replace ")
	if !ok || strings.Contains(suggestion, "\n") {
		return "", false // a Suggestion is one line (review.Finding)
	}
	// " with " may also occur inside a quoted summary: the left side is the
	// split whose text occurs in src. A space in it matches any whitespace run,
	// which is how a one-line suggestion names a multi-line span.
	for offset := 0; ; {
		i := strings.Index(rest[offset:], " with ")
		if i < 0 {
			return "", false
		}
		cut := offset + i
		old, repl := rest[:cut], rest[cut+len(" with "):]
		left := regexp.MustCompile(strings.ReplaceAll(regexp.QuoteMeta(old), " ", `\s+`))
		if loc := left.FindStringIndex(src); loc != nil {
			return src[:loc[0]] + repl + src[loc[1]:], true
		}
		offset = cut + 1
	}
}

func hasRetiredSpelling(src, name string) bool {
	if i := strings.LastIndex(name, "."); i >= 0 {
		qual, method := name[:i], name[i+1:]
		if qual == "Failure" {
			return strings.Contains(src, "Failure."+method) || strings.Contains(src, "fail."+method+"(")
		}
		// evo.Next / evo.NextCommand are the live ProblemOptions, not the
		// removed methods of the same name.
		return strings.Contains(strings.ReplaceAll(src, "evo."+method+"(", ""), "."+method+"(")
	}
	return containsEvoIdent(src, name)
}

func containsEvoIdent(src, name string) bool {
	needle := "evo." + name
	for i := 0; i+len(needle) <= len(src); i++ {
		if src[i:i+len(needle)] != needle {
			continue
		}
		if i+len(needle) < len(src) && isIdentContinue(src[i+len(needle)]) {
			continue
		}
		return true
	}
	return false
}

func isIdentContinue(b byte) bool {
	return b == '_' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}
