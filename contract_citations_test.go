package evo

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// contractHeading matches a numbered section heading of CONTRACT.md.
var contractHeading = regexp.MustCompile(`(?m)^## (\d+)\. `)

// contractCitation matches a comment that cites CONTRACT.md by section:
// "CONTRACT §5", "contract section 7", "CONTRACT.md §8". Citations of any
// other document ("spec §11.3") are not this guard's to judge.
var contractCitation = regexp.MustCompile(`(?i)\bcontract(?:\.md)?\s+(?:§\s?|sections?\s+)(\d+(?:\.\d+)*)`)

// contractSections is the set of section numbers CONTRACT.md has.
func contractSections(t *testing.T) map[string]bool {
	t.Helper()
	body, err := os.ReadFile("CONTRACT.md")
	if err != nil {
		t.Fatalf("read CONTRACT.md: %v", err)
	}
	sections := map[string]bool{}
	for _, m := range contractHeading.FindAllStringSubmatch(string(body), -1) {
		sections[m[1]] = true
	}
	if len(sections) == 0 {
		t.Fatal("CONTRACT.md has no numbered section headings")
	}
	return sections
}

// citation is one comment's reference to a CONTRACT.md section.
type citation struct {
	pos     token.Position
	section string
}

// commentCitations returns every CONTRACT.md section a comment in the Go file
// at path cites. A comment group is read as one text, so a citation wrapped
// across lines is still found.
func commentCitations(t *testing.T, path string) []citation {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var cited []citation
	for _, group := range file.Comments {
		text := strings.Join(strings.Fields(commentText(group)), " ")
		for _, m := range contractCitation.FindAllStringSubmatch(text, -1) {
			cited = append(cited, citation{fset.Position(group.Pos()), m[1]})
		}
	}
	return cited
}

// commentText is a comment group's text without its comment markers.
func commentText(group *ast.CommentGroup) string {
	var b strings.Builder
	for _, c := range group.List {
		b.WriteString(strings.TrimPrefix(strings.TrimPrefix(c.Text, "//"), "/*"))
		b.WriteByte(' ')
	}
	return b.String()
}

// goSourcesCitingContract lists the Go files whose comments this guard reads:
// the root package's own files, and every file under internal and mcp except
// testdata, which holds fixtures that quote code on purpose.
func goSourcesCitingContract(t *testing.T) []string {
	t.Helper()
	var paths []string
	roots, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob root Go files: %v", err)
	}
	paths = append(paths, roots...)
	for _, dir := range []string{"internal", "mcp"} {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			switch {
			case err != nil:
				return err
			case d.IsDir() && d.Name() == "testdata":
				return fs.SkipDir
			case !d.IsDir() && strings.HasSuffix(path, ".go"):
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	return paths
}

// TestCommentsCiteExistingContractSections: a comment that cites CONTRACT.md
// by section sends the reader to a real section. CONTRACT.md has a handful
// of numbered sections, and 1.x-era comments cited §9 through §30 of a longer
// document, so a stale citation reads as authority and points nowhere.
func TestCommentsCiteExistingContractSections(t *testing.T) {
	sections := contractSections(t)
	for _, path := range goSourcesCitingContract(t) {
		for _, c := range commentCitations(t, path) {
			if !sections[c.section] {
				t.Errorf("%s cites CONTRACT.md section %s; CONTRACT.md has %d sections", c.pos, c.section, len(sections))
			}
		}
	}
}
