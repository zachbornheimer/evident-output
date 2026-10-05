package fix_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/fix"
)

// wantDiagnostics extracts every `// want `regexp“ annotation from src,
// the same convention analysistest uses, so these fixtures still document
// which line raises which rule with what message, without requiring the
// analysistest stub-package layout (which collides with zq's pre-commit
// `go fix` step type-checking testdata against the real, current evo
// package: see AGENTS.md and the 1.1-fixer slice notes).
func wantDiagnostics(src string) map[int]string {
	want := map[int]string{}
	for i, line := range strings.Split(src, "\n") {
		_, after, ok := strings.Cut(line, "// want `")
		if !ok {
			continue
		}
		rest := after
		end := strings.LastIndex(rest, "`")
		if end < 0 {
			continue
		}
		want[i+1] = rest[:end]
	}
	return want
}

// matchMessage reports whether got matches the `// want` annotation's
// regexp, the same semantics golang.org/x/tools/go/analysis/analysistest
// uses for its own `// want` comments — analysistest anchors neither end,
// so a message containing the pattern anywhere satisfies it.
func matchMessage(t *testing.T, name string, line int, wantPattern, got string) {
	t.Helper()
	re, err := regexp.Compile(wantPattern)
	if err != nil {
		t.Fatalf("%s: line %d: invalid want regexp %q: %v", name, line, wantPattern, err)
		return
	}
	if !re.MatchString(got) {
		t.Errorf("%s: line %d: diagnostic message %q does not match want `%s`", name, line, got, wantPattern)
	}
}

// runFixture type-checks src against the real evo module (via a throwaway
// module with a replace directive, run_test.go's writeModule pattern),
// diagnoses it, asserts every `// want` line fired, then applies the
// fixes and compares the result to golden.
func runFixture(t *testing.T, name, src, golden string) {
	t.Helper()
	dir := t.TempDir()
	writeModule(t, dir, src)

	want := wantDiagnostics(src)

	pkgs, err := fix.Load(dir, ".")
	if err != nil {
		t.Fatalf("%s: Load: %v", name, err)
	}
	results, err := fix.Diagnose(pkgs, false)
	if err != nil {
		t.Fatalf("%s: Diagnose: %v", name, err)
	}
	if len(results) != 1 {
		t.Fatalf("%s: want 1 package result, got %d", name, len(results))
	}
	gotByLine := map[int]string{}
	for _, d := range results[0].Diagnostics {
		gotByLine[d.Line] = d.Message
	}
	for line, wantPattern := range want {
		got, ok := gotByLine[line]
		if !ok {
			t.Errorf("%s: line %d: want a diagnostic, got none", name, line)
			continue
		}
		matchMessage(t, name, line, wantPattern, got)
	}
	for line := range gotByLine {
		if _, ok := want[line]; !ok {
			t.Errorf("%s: line %d: unexpected diagnostic", name, line)
		}
	}

	if _, err := fix.Diagnose(pkgs, true); err != nil {
		t.Fatalf("%s: Diagnose(apply): %v", name, err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "main.go"))
	if err != nil {
		t.Fatalf("%s: read fixed file: %v", name, err)
	}
	if string(got) != golden {
		t.Errorf("%s: fixed output mismatch\n--- got ---\n%s\n--- want ---\n%s", name, got, golden)
	}
}

// TestAnalyzersAgainstGoldenFixtures runs each 1.1-migration analyzer
// end to end (Load -> Diagnose -> apply) against fixtures covering
// arbitrary receiver names, chained calls, method values, method
// expressions, calls inside and outside Define callbacks, multi-line
// arguments, and the negative same-named non-evo method — the same
// coverage run_test.go's TestDiagnoseFindsEveryRemovedName exercises for
// "every removed name fires something", but per-analyzer and against
// exact golden fixed source.
//
// Fixtures type-check against the real evo package via a throwaway
// module (writeModule, shared with run_test.go) rather than an
// analysistest stub under testdata/src: zq's pre-commit `go fix` step
// type-checks testdata fixtures against the real top-level evo package
// because it invokes `go fix` on explicit changed paths, not `./...`, so
// Go's normal "skip testdata dirs" convention doesn't apply. A stub
// package under testdata/src/github.com/zachbornheimer/evident-output
// that lacks 1.1 members like Problem/Progress/Skipped/Capture fails
// that check. Fixtures live as string constants instead.
func TestAnalyzersAgainstGoldenFixtures(t *testing.T) {
	t.Run("warn", func(t *testing.T) { runFixture(t, "warn", warnFixtureSrc, warnFixtureGolden) })
	t.Run("step", func(t *testing.T) { runFixture(t, "step", stepFixtureSrc, stepFixtureGolden) })
	t.Run("kept", func(t *testing.T) { runFixture(t, "kept", keptFixtureSrc, keptFixtureGolden) })
	t.Run("capture", func(t *testing.T) { runFixture(t, "capture", captureFixtureSrc, captureFixtureGolden) })
	t.Run("reasonoption", func(t *testing.T) {
		runFixture(t, "reasonoption", reasonOptionFixtureSrc, reasonOptionFixtureGolden)
	})
	t.Run("options", func(t *testing.T) { runFixture(t, "options", goldenOptionsFixtureSrc, goldenOptionsFixtureGolden) })
}
