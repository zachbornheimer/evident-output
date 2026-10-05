package docexamples_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/docexamples"
)

func TestExtractGoFences(t *testing.T) {
	md := []byte("# Title\n" +
		"\n" +
		"prose before\n" +
		"\n" +
		"```go\n" +
		"a := 1\n" +
		"b := 2\n" +
		"```\n" +
		"\n" +
		"prose between\n" +
		"\n" +
		"```bash\n" +
		"echo not go\n" +
		"```\n" +
		"\n" +
		"```go\n" +
		"c := 3\n" +
		"```\n")

	got := docexamples.ExtractGoFences(md)
	want := []docexamples.GoFence{
		{Content: "a := 1\nb := 2\n", Line: 5},
		{Content: "c := 3\n", Line: 16},
	}
	if len(got) != len(want) {
		t.Fatalf("ExtractGoFences() returned %d fences, want %d: %#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("fence %d = %#v, want %#v", i, got[i], want[i])
		}
	}
}

func TestExtractGoFences_IgnoresNonGoInfoString(t *testing.T) {
	md := []byte("```gotemplate\nnot go\n```\n```go-ish\nalso not go\n```\n")
	got := docexamples.ExtractGoFences(md)
	if len(got) != 0 {
		t.Fatalf("ExtractGoFences() = %#v, want none (info string must be exactly \"go\")", got)
	}
}

func TestExtractGoFences_NoFences(t *testing.T) {
	got := docexamples.ExtractGoFences([]byte("just prose, no code\n"))
	if len(got) != 0 {
		t.Fatalf("ExtractGoFences() = %#v, want none", got)
	}
}

func TestSnippetRegion(t *testing.T) {
	src := []byte("package fixtures\n\nfunc f() {\n\t// docexamples:snippet start\na := 1\nb := 2\n\t// docexamples:snippet end\n}\n")
	got, err := docexamples.SnippetRegion(src, "fixtures/f.go")
	if err != nil {
		t.Fatalf("SnippetRegion() error = %v", err)
	}
	want := "a := 1\nb := 2\n"
	if got != want {
		t.Fatalf("SnippetRegion() = %q, want %q", got, want)
	}
}

func TestSnippetRegion_MissingMarkers(t *testing.T) {
	_, err := docexamples.SnippetRegion([]byte("package fixtures\n"), "fixtures/f.go")
	if err == nil {
		t.Fatal("SnippetRegion() error = nil, want error for missing markers")
	}
	if !errors.Is(err, docexamples.ErrNoRegion) {
		t.Errorf("SnippetRegion() error = %v, want errors.Is(err, ErrNoRegion)", err)
	}
}

func TestSnippetRegion_MultipleRegionsConcatenate(t *testing.T) {
	src := []byte("package fixtures\n\n" +
		"// docexamples:snippet start\n" +
		"import \"pkg\"\n" +
		"\n" +
		"// docexamples:snippet end\n" +
		"\n" +
		"func f() {\n" +
		"\t// docexamples:snippet start\n" +
		"pkg.Do()\n" +
		"\t// docexamples:snippet end\n" +
		"}\n")
	got, err := docexamples.SnippetRegion(src, "fixtures/f.go")
	if err != nil {
		t.Fatalf("SnippetRegion() error = %v", err)
	}
	want := "import \"pkg\"\n\npkg.Do()\n"
	if got != want {
		t.Fatalf("SnippetRegion() = %q, want %q", got, want)
	}
}

func TestSnippetRegion_UnbalancedMarkers(t *testing.T) {
	if _, err := docexamples.SnippetRegion([]byte("// docexamples:snippet start\na := 1\n"), "fixtures/f.go"); err == nil {
		t.Fatal("SnippetRegion() error = nil, want error for an unclosed region")
	}
}

func TestNormalizeIndent(t *testing.T) {
	got := docexamples.NormalizeIndent("func f() {\n    a := 1\n        b := 2\n    c := 1 + 2\n}\n")
	want := "func f() {\na := 1\nb := 2\nc := 1 + 2\n}"
	if got != want {
		t.Fatalf("NormalizeIndent() = %q, want %q", got, want)
	}
}

func TestNormalizeIndent_TrimsEdgeBlankLinesOnly(t *testing.T) {
	// gofmt inserts a blank line between a function's closing brace and a
	// following floating comment (the end marker); that edge blank line
	// must normalize away. A blank line *between* other content (here,
	// separating two statements) is meaningful and must survive.
	withEdgeBlank := "\na := 1\n\nb := 2\n\n"
	got := docexamples.NormalizeIndent(withEdgeBlank)
	want := "a := 1\n\nb := 2"
	if got != want {
		t.Fatalf("NormalizeIndent(%q) = %q, want %q", withEdgeBlank, got, want)
	}
}

func TestNormalizeIndent_IgnoresIndentDepthDifferences(t *testing.T) {
	// Same code, indented differently (as a doc's 4-space fence and a
	// gofmt'd fixture's tab-indented region are): must normalize equal.
	fourSpace := "if err != nil {\n    return err\n}\n"
	oneTab := "\tif err != nil {\n\t\treturn err\n\t}\n"
	if docexamples.NormalizeIndent(fourSpace) != docexamples.NormalizeIndent(oneTab) {
		t.Fatalf("NormalizeIndent(%q) = %q, NormalizeIndent(%q) = %q, want equal",
			fourSpace, docexamples.NormalizeIndent(fourSpace), oneTab, docexamples.NormalizeIndent(oneTab))
	}
}

func TestNormalizeIndent_IgnoresTrailingCommentAlignment(t *testing.T) {
	// gofmt right-aligns trailing line comments across a comment group by
	// padding with extra internal spaces; that padding must normalize away
	// too, since it carries no semantic information.
	unaligned := "os.Exit(x) // done\n"
	aligned := "os.Exit(x)          // done\n"
	if docexamples.NormalizeIndent(unaligned) != docexamples.NormalizeIndent(aligned) {
		t.Fatalf("NormalizeIndent(%q) = %q, NormalizeIndent(%q) = %q, want equal",
			unaligned, docexamples.NormalizeIndent(unaligned), aligned, docexamples.NormalizeIndent(aligned))
	}
}

func TestFixtureRegion_ConcatenatesAcrossFilesInNameOrder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a_scaffold.go", "package fixtures\n\nimport \"fmt\"\n\nvar _ = fmt.Sprint\n")
	writeFile(t, dir, "b_marked.go", "package fixtures\n\n// docexamples:snippet start\nx := 1\n// docexamples:snippet end\n")
	writeFile(t, dir, "c_marked.go", "package fixtures\n\n// docexamples:snippet start\ny := 2\n// docexamples:snippet end\n")

	got, err := docexamples.FixtureRegion(dir)
	if err != nil {
		t.Fatalf("FixtureRegion() error = %v", err)
	}
	want := "x := 1\ny := 2\n"
	if got != want {
		t.Fatalf("FixtureRegion() = %q, want %q", got, want)
	}
}

func TestFixtureRegion_NoRegionAnywhere(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", "package fixtures\n")
	if _, err := docexamples.FixtureRegion(dir); !errors.Is(err, docexamples.ErrNoRegion) {
		t.Fatalf("FixtureRegion() error = %v, want errors.Is(err, ErrNoRegion)", err)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
