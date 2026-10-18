package guards_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/v2/internal/agent/rules"
)

// godocStaleAPIExempt holds the code whose comments must name retired API:
// the table of retired names itself, the review detector files listed here
// by name (each recognises removed 1.x shapes in a consumer's source; a
// new or other review file is NOT exempt, so stale prose there is caught),
// the review fixtures, and the 1.0 migration fixtures that pin removed text
// on purpose.
var godocStaleAPIExempt = []string{
	"mcp/internal/agent/rules/retired",
	"mcp/internal/agent/review/detectors.go",
	"mcp/internal/agent/review/dialect.go",
	"mcp/internal/agent/review/first_paint.go",
	"mcp/internal/agent/review/review_call_site.go",
	"mcp/internal/agent/review/review_dag_sequence.go",
	"mcp/internal/agent/review/review_define_callbacks.go",
	"mcp/internal/agent/review/review_define_context.go",
	"mcp/internal/agent/review/review_deprecated.go",
	"mcp/internal/agent/review/review_evidence_dag.go",
	"mcp/internal/agent/review/review_fanout_wait.go",
	"mcp/internal/agent/review/review_narration.go",
	"mcp/internal/agent/review/review_output_bindings.go",
	"mcp/internal/agent/review/review_problem_accumulation.go",
	"mcp/internal/agent/review/review_reason_summary.go",
	"mcp/internal/agent/review/review_rec_done.go",
	"mcp/internal/agent/review/review_rec_mutation.go",
	"mcp/internal/agent/review/review_rec_next.go",
	"mcp/internal/agent/review/review_rec.go",
	"mcp/internal/agent/review/review_record_migration.go",
	"mcp/internal/agent/review/review_remedy_receivers.go",
	"mcp/internal/agent/review/review_repeated_disposition.go",
	"mcp/internal/agent/review/review_sibling_names.go",
	"mcp/internal/agent/review/review_task_bindings.go",
	"mcp/internal/agent/review/review_task_naming.go",
	"mcp/internal/agent/review/review_topology_builder_purity.go",
	"mcp/internal/agent/review/review_unresolved_task.go",
	"mcp/internal/agent/review/review_verb_style.go",
	"mcp/internal/agent/review/testdata/",
	"mcp/internal/docexamples/fixtures/migration_",
}

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
