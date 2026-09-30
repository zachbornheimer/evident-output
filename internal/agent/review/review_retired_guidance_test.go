package review

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/rules"
)

// detectionKey matches a literal that is a bare identifier or call
// spelling ("MainWith", ".Done(") — a key a detector matches old code
// with, not guidance a reader follows.
var detectionKey = regexp.MustCompile(`^[\w.]+\(?$`)

// TestDetectorGuidanceNeverTeachesARetiredSymbol scans every string
// literal the review and adopt detectors build messages and suggestions
// from, and fails when one names a retired API outside a "removed in
// <release>" note.
func TestDetectorGuidanceNeverTeachesARetiredSymbol(t *testing.T) {
	for _, dir := range []string{".", "../adopt"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			checkLiteralsTeachNoRetiredSymbol(t, path)
		}
	}
}

func checkLiteralsTeachNoRetiredSymbol(t *testing.T, path string) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		text, err := strconv.Unquote(lit.Value)
		if err != nil || detectionKey.MatchString(text) {
			return true
		}
		for _, h := range rules.UnexplainedRetired(text) {
			t.Errorf("%s: literal names retired %q without \"removed in %s\" (use %s): %.120q",
				fset.Position(lit.Pos()), h.Match, h.Symbol.RemovedIn, h.Symbol.Replacement, text)
		}
		return true
	})
}
