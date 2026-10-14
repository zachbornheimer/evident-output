package review

import (
	"go/ast"
	"go/importer"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/rules"
)

// fixtureRoot holds one directory per registered file detector. A smell
// detector has bad*.go (a program that compiles against the 2.0 surface and
// exhibits the smell) and, where the clean shape is not obvious, good*.go
// (its clean twin). A migration detector has legacy*.go: dirty input that
// the detector must map onto the 2.0 surface.
const fixtureRoot = "testdata/dialect_2_0"

// twoZeroPins are the dialects a 2.0 fixture is reviewed at: the current
// dialect (empty) and the first release of the 2.0 contract.
var twoZeroPins = []string{"", "2.0.0"}

// TestEveryDetectorHasA20Fixture fails when a registered detector has no
// fixture that proves it still detects its smell in the 2.0 dialect. A
// detector that can only match 1.x shapes belongs in rules.RetiredRules,
// not here.
func TestEveryDetectorHasA20Fixture(t *testing.T) {
	imp := importer.ForCompiler(token.NewFileSet(), "source", nil)
	registered := map[string]bool{}
	for _, d := range fileDetectors {
		if d.name == "" {
			t.Fatalf("detector registered without a name: %+v", d)
		}
		if registered[d.name] {
			t.Fatalf("detector %s registered twice", d.name)
		}
		registered[d.name] = true
		t.Run(d.name, func(t *testing.T) { checkDetectorFixtures(t, d, imp) })
	}
	t.Run("no orphan fixture directories", func(t *testing.T) {
		dirs, err := os.ReadDir(fixtureRoot)
		if err != nil {
			t.Fatalf("read %s: %v", fixtureRoot, err)
		}
		for _, e := range dirs {
			if e.IsDir() && !registered[e.Name()] {
				t.Errorf("%s/%s has no registered detector; delete it with the detector", fixtureRoot, e.Name())
			}
		}
	})
}

func checkDetectorFixtures(t *testing.T, d detector, imp types.Importer) {
	t.Helper()
	positive := "bad"
	if d.migration {
		positive = "legacy"
	}
	dir := filepath.Join(fixtureRoot, d.name)
	fires, err := filepath.Glob(filepath.Join(dir, positive+"*.go"))
	if err != nil || len(fires) == 0 {
		t.Fatalf("detector %s has no %s*.go fixture under %s: port it to the 2.0 dialect with a red fixture, or retire it in rules.RetiredRules", d.name, positive, dir)
	}
	for _, path := range fires {
		src := readFixture(t, path)
		if !d.migration {
			requireCompiles2_0(t, path, src, imp)
		}
		for _, pin := range twoZeroPins {
			findings := runDetectorAt(t, d, path, src, pin)
			if len(findings) == 0 {
				t.Errorf("%s: detector %s found nothing at dialect %q; a 2.0 fixture must produce its finding", path, d.name, pin)
			}
			for _, f := range findings {
				if hits := rules.UnexplainedRetired(f.Suggestion + " " + f.Message); len(hits) > 0 {
					t.Errorf("%s: %s teaches retired %s (%q)", path, f.RuleID, hits[0].Symbol.Contract, hits[0].Match)
				}
			}
		}
	}
	cleans, _ := filepath.Glob(filepath.Join(dir, "good*.go"))
	for _, path := range cleans {
		src := readFixture(t, path)
		requireCompiles2_0(t, path, src, imp)
		for _, pin := range twoZeroPins {
			if findings := runDetectorAt(t, d, path, src, pin); len(findings) > 0 {
				t.Errorf("%s: clean 2.0 twin still triggers %s at dialect %q: %+v", path, d.name, pin, findings[0])
			}
		}
	}
}

func readFixture(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return string(b)
}

// runDetectorAt runs only d over src, the way reviewFile would at pin.
func runDetectorAt(t *testing.T, d detector, path, src, pin string) []Finding {
	t.Helper()
	fset, f, err := parseSourceFile(filepath.Base(path), src)
	if err != nil {
		t.Fatalf("parse fixture %s: %v", path, err)
	}
	in := fileInput{
		filename:       filepath.Base(path),
		src:            src,
		desiredVersion: pin,
		file:           f,
		fset:           fset,
		hasEvo:         evoImportName(f) != "",
	}
	if !d.admits(in) {
		return nil
	}
	return admitDialect(d.run(in), pin)
}

// requireCompiles2_0 type-checks src against the real evo package, so a
// fixture can never keep a removed or invented 1.x name alive. Packages the
// module cannot resolve (a CLI framework) are tolerated; nothing else is.
func requireCompiles2_0(t *testing.T, path, src string, imp types.Importer) {
	t.Helper()
	fset, f, err := parseSourceFile(filepath.Base(path), src)
	if err != nil {
		t.Fatalf("parse fixture %s: %v", path, err)
	}
	var problems []string
	conf := types.Config{Importer: imp, Error: func(err error) {
		if msg := err.Error(); !strings.Contains(msg, "could not import") {
			problems = append(problems, msg)
		}
	}}
	_, _ = conf.Check("fixture", fset, []*ast.File{f}, nil)
	if len(problems) > 0 {
		t.Errorf("%s does not compile against the 2.0 surface:\n  %s", path, strings.Join(slices.Compact(problems), "\n  "))
	}
}
