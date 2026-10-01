package docexamples_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// discouragedShapeFixtures compile a doc fence that shows the old or
// discouraged shape on purpose, so review is expected to flag them.
var discouragedShapeFixtures = map[string]bool{
	"migration_1_0_duplicate_sibling": true, // the repeated-call shape beside its fix
	"migration_1_0_file_spec":         true, // frozen 1.0 FileSpec/FSPath fence, retired in 1.2 (docs/zys-1382/KNOWN_BROKEN.md)
}

func showsDiscouragedShape(name string) bool {
	return strings.Contains(name, "_before") || discouragedShapeFixtures[name]
}

// TestDocFixturesReviewClean pins E-088: every shipped doc fence compiles
// as a fixture, and every one that teaches the current dialect must also
// pass the release's own review. development.md taught Config.Options
// while review flagged it as superseded, and applying review's rewrite
// broke the code.
func TestDocFixturesReviewClean(t *testing.T) {
	dirs, err := os.ReadDir("fixtures")
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	for _, d := range dirs {
		if !d.IsDir() || showsDiscouragedShape(d.Name()) {
			continue
		}
		dir, err := filepath.Abs(filepath.Join("fixtures", d.Name()))
		if err != nil {
			t.Fatalf("resolve %s: %v", d.Name(), err)
		}
		res, err := review.GoDirectory(dir)
		if err != nil {
			t.Fatalf("review %s: %v", d.Name(), err)
		}
		for _, f := range res.Findings {
			t.Errorf("%s:%d %s: %s (suggestion: %s)", d.Name(), f.Line, f.RuleID, f.Message, f.Suggestion)
		}
	}
}
