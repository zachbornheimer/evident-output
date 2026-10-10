package guards_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/rules"
)

// godocStaleAPIExempt holds the one package whose comments must name
// retired API: the table of retired names itself.
var godocStaleAPIExempt = []string{"mcp/internal/agent/rules/retired"}

// TestGoCommentsCarryNoStaleAPI extends TestDocsCarryNoStaleAPI to Go
// comments. Engine godoc is read by agents as surely as the docs are, so a
// comment that demonstrates a removed call (task.Run, .Done(),
// TaskHandle.Record, DisplayGroup) teaches it. A comment may name a retired
// symbol only alongside "removed in <release>".
func TestGoCommentsCarryNoStaleAPI(t *testing.T) {
	root := moduleRoot(t)
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, "_test.go") || godocExempt(rel) {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			t.Errorf("parse %s: %v", rel, err)
			return nil
		}
		for _, group := range file.Comments {
			for _, hit := range rules.UnexplainedRetired(group.Text()) {
				t.Errorf("%s:%d: comment teaches retired API %q without \"removed in %s\" (use %s)",
					rel, fset.Position(group.Pos()).Line, hit.Match, hit.Symbol.RemovedIn, hit.Symbol.Replacement)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}

func godocExempt(rel string) bool {
	for _, prefix := range godocStaleAPIExempt {
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	return false
}
