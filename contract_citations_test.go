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

// citationFinder returns the section numbers one comment's text cites.
type citationFinder func(text string) []string

// contractCitations finds the CONTRACT.md sections a comment cites.
func contractCitations(text string) []string {
	var sections []string
	for _, m := range contractCitation.FindAllStringSubmatch(text, -1) {
		sections = append(sections, m[1])
	}
	return sections
}

// commentCitations returns every section a comment in the Go file at path
// cites, as cites finds them. A comment group is read as one text, so a
// citation wrapped across lines is still found.
func commentCitations(t *testing.T, path string, cites citationFinder) []citation {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var cited []citation
	for _, group := range file.Comments {
		text := strings.Join(strings.Fields(commentText(group)), " ")
		for _, section := range cites(text) {
			cited = append(cited, citation{fset.Position(group.Pos()), section})
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
		for _, c := range commentCitations(t, path, contractCitations) {
			if !sections[c.section] {
				t.Errorf("%s cites CONTRACT.md section %s; CONTRACT.md has %d sections", c.pos, c.section, len(sections))
			}
		}
	}
}

// specCitation matches a comment that cites the 1.x architecture spec by
// section: "spec §11.3", "Spec §11/§12", "spec §8.3/§11.4", "spec §2, §20-21". Group 1 is the
// whole run of sections after "spec".
var specCitation = regexp.MustCompile(`(?i)\bspec\s+(§\s?\d+(?:\.\d+)*(?:\s*(?:/|,|&|and)\s*§\s?\d+(?:\.\d+)*|-\d+)*)`)

var sectionNumber = regexp.MustCompile(`\d+(?:\.\d+)*`)

// specCitations finds the spec sections a comment cites, as written ("11.3").
func specCitations(text string) []string {
	var sections []string
	for _, m := range specCitation.FindAllStringSubmatch(text, -1) {
		sections = append(sections, sectionNumber.FindAllString(m[1], -1)...)
	}
	return sections
}

// specRegistrySection matches the registry files that mirror the numbered
// sections of the spec (s01.json is section 1). The contract-2.0 files (cNN)
// are keyed to CONTRACT.md and mcp.json is a bucket, neither a spec section.
var specRegistrySection = regexp.MustCompile(`^s(\d+)\.json$`)

// specSections is the set of top-level spec section numbers the requirement
// registry under conformance/spec/req has a file for.
func specSections(t *testing.T) map[string]bool {
	t.Helper()
	files, err := os.ReadDir(filepath.Join("conformance", "spec", "req"))
	if err != nil {
		t.Fatalf("read the requirement registry: %v", err)
	}
	sections := map[string]bool{}
	for _, f := range files {
		if m := specRegistrySection.FindStringSubmatch(f.Name()); m != nil {
			sections[strings.TrimLeft(m[1], "0")] = true
		}
	}
	if len(sections) == 0 {
		t.Fatal("conformance/spec/req has no sNN.json section files")
	}
	return sections
}

// goSourcesUnderRepo lists every Go file of the repository (all modules),
// except testdata fixtures that quote code on purpose and tooling directories.
func goSourcesUnderRepo(t *testing.T) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && (d.Name() == "testdata" || d.Name() == "node_modules" || (path != "." && strings.HasPrefix(d.Name(), "."))):
			return fs.SkipDir
		case !d.IsDir() && strings.HasSuffix(path, ".go"):
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk the repository: %v", err)
	}
	return paths
}

// TestCommentsCiteExistingSpecSections: a comment that cites "spec §N" sends
// the reader to a section the requirement registry has. The registry
// (conformance/spec/req/sNN.json) covers sections 1 through 34 of the 1.x
// spec; the document those sections came from is gone, and comments that
// cited its sections 35 through 64 pointed at wire-format and MCP chapters nothing
// in the repository states. Cite the schema, CONTRACT.md or the behaviour
// itself instead.
func TestCommentsCiteExistingSpecSections(t *testing.T) {
	sections := specSections(t)
	for _, path := range goSourcesUnderRepo(t) {
		for _, c := range commentCitations(t, path, specCitations) {
			top, _, _ := strings.Cut(c.section, ".")
			if !sections[top] {
				t.Errorf("%s cites spec section %s; the registry has sections 1 through %d", c.pos, c.section, len(sections))
			}
		}
	}
}

// TestSpecCitationsFindsEveryCitedSection: the finder reads a run of sections
// after "spec" (slashes, commas, ranges), so a late entry in a list cannot
// hide a dangling section.
func TestSpecCitationsFindsEveryCitedSection(t *testing.T) {
	cases := map[string][]string{
		"see spec §11.3 for the lock":          {"11.3"},
		"Spec §2, §8.2, §20-21, §41, §49 hold": {"2", "8.2", "20", "21", "41", "49"},
		"proves spec §8.3/§11.4: two Execs":    {"8.3", "11.4"},
		"spec §36 and §39 describe it":         {"36", "39"},
		"the contract §8 is not a spec cite":   nil,
		"3 items, spec §5, 7 more":             {"5"},
	}
	for text, want := range cases {
		got := specCitations(text)
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("specCitations(%q) = %v, want %v", text, got, want)
		}
	}
}
