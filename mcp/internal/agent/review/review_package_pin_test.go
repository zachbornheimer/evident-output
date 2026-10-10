package review_test

import (
	"slices"
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/review"
)

// objectFirstDeleteSrc is the 1.0 object-first mutation verb: current at a
// v1.0.0 pin, rewritten to evo.Effect only from the 1.1 dialect.
const objectFirstDeleteSrc = `package p
import evo "github.com/zachbornheimer/evident-output"
func f(g *evo.GroupHandle, p string) {
	g.Task(p).Delete("worktree", func() error { return remove(p) })
}
`

// The package kind honors the pin the same way the file kind does: a
// v1.0.0 consumer is never steered to a 1.1-only API.
func TestGoPackageAtHonorsPinnedDialect(t *testing.T) {
	files := map[string]string{"prune.go": objectFirstDeleteSrc}
	if got := findAPI032(review.GoSourceAt("prune.go", objectFirstDeleteSrc, "v1.0.0")); len(got) != 0 {
		t.Fatalf("file kind at v1.0.0: want 0 API-032, got %+v", got)
	}
	if got := findAPI032(review.GoPackageAt(files, "v1.0.0")); len(got) != 0 {
		t.Fatalf("package kind at v1.0.0: want 0 API-032, got %+v", got)
	}
	if got := findAPI032(review.GoPackage(files)); len(got) == 0 {
		t.Fatal("package kind at the current dialect must still flag the 1.0 spelling")
	}
}

// Findings keep one order across calls, so review → apply → review sees
// the same list for the same source.
func TestGoPackageFindingsAreStable(t *testing.T) {
	files := map[string]string{}
	for _, name := range []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go"} {
		files[name] = objectFirstDeleteSrc
	}
	first := review.GoPackage(files).Findings
	for range 20 {
		if again := review.GoPackage(files).Findings; !slices.Equal(first, again) {
			t.Fatalf("GoPackage findings changed order between calls:\n%+v\n%+v", first, again)
		}
	}
}
