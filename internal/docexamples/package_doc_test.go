package docexamples_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/docexamples"
	"github.com/zachbornheimer/evident-output/internal/gitenv"
)

const packageDocFixture = "fixtures/package_doc_quickstart"

// packageDocCode is the first indented code block of doc.go's package
// comment, the quickstart `go doc` shows first, with its trailing blank
// line (the comment's paragraph break) included.
func packageDocCode(src string) string {
	var code []string
	for line := range strings.SplitSeq(src, "\n") {
		body, isCode := strings.CutPrefix(line, "//\t")
		if line == "//" && len(code) > 0 {
			body, isCode = "", true // a blank line inside the block
		}
		if !isCode {
			if len(code) > 0 {
				break
			}
			continue
		}
		code = append(code, body)
	}
	return strings.Join(code, "\n") + "\n"
}

// TestPackageDocQuickstartMatchesFixture proves the fixture that
// TestPackageDocQuickstartRuns runs is doc.go's quickstart verbatim.
func TestPackageDocQuickstartMatchesFixture(t *testing.T) {
	root := repoRoot(t)
	src, err := os.ReadFile(filepath.Join(root, "doc.go"))
	if err != nil {
		t.Fatal(err)
	}
	region, err := docexamples.FixtureRegion(filepath.Join(root, "internal", "docexamples", packageDocFixture))
	if err != nil {
		t.Fatal(err)
	}
	doc := packageDocCode(string(src))
	if docexamples.NormalizeIndent(region) != docexamples.NormalizeIndent(doc) {
		t.Fatalf("doc.go quickstart drifted from %s:\n--- doc.go ---\n%s\n--- fixture ---\n%s", packageDocFixture, doc, region)
	}
}

// TestPackageDocQuickstartRuns runs doc.go's quickstart and fails when a
// row is left unresolved: following the first example `go doc` shows
// must conclude ready, with no misuse hint. The program runs in a fresh
// git work tree it owns, so the result never depends on whether the
// module itself is checked out (a source archive, the module cache, or a
// CI checkout owned by another user is not).
func TestPackageDocQuickstartRuns(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs a program")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("the quickstart runs git, which is not installed")
	}
	binary := filepath.Join(t.TempDir(), "quickstart")
	build := exec.Command("go", "build", "-buildvcs=false", "-o", binary, "./internal/docexamples/"+packageDocFixture)
	build.Dir = repoRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build quickstart: %v\n%s", err, out)
	}
	workTree := t.TempDir()
	if out, err := gitenv.Command("", "init", "--quiet", workTree).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	cmd := exec.Command(binary)
	cmd.Dir = workTree
	cmd.Env = gitenv.Scrub(os.Environ())
	got, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("quickstart: %v\n%s", err, got)
	}
	for _, defect := range []string{"incomplete", "partial", "call Define"} {
		if strings.Contains(string(got), defect) {
			t.Fatalf("quickstart output contains %q:\n%s", defect, got)
		}
	}
}
