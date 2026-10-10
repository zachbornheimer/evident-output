package docexamples

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// snippetStart and snippetEnd bracket a doc-sourced region inside a fixture
// file. A fixture may use the pair more than once — some fences (e.g. an
// `import` line followed by call-site statements) mix file-scope and
// function-scope Go, which cannot occupy the same physical location in a
// compilable file. Every region's lines, in file order, concatenate back
// into exactly the fence they claim to cover; everything outside any region
// is fixture-only scaffolding (imports, stub types, a wrapping function)
// never asserted against the doc.
const (
	snippetStart = "// docexamples:snippet start"
	snippetEnd   = "// docexamples:snippet end"
)

// ErrNoRegion means a fixture file has no snippetStart/snippetEnd marker
// pair. FixtureRegion treats it as "this file is scaffolding-only, skip
// it"; a caller checking a single file treats it as a fixture authoring
// defect.
var ErrNoRegion = errors.New("no marker region")

// DocFixture pins one fenced code block in a source doc to the fixture
// package that proves it still compiles.
type DocFixture struct {
	// Doc is the source doc's path, relative to the module root — the file
	// a maintainer actually edits (docs/*.md), not a generated copy.
	Doc string
	// FenceIndex is the fence's 0-based position among the ```go blocks in
	// Doc, in document order.
	FenceIndex int
	// Fixture is the compiled fixture package's directory, relative to this
	// package directory (mcp/internal/docexamples) — e.g.
	// "fixtures/readme_quickstart". A fence that mixes file-scope and
	// function-scope Go (an `import` line followed by call-site statements)
	// needs two files: goimports merges any two import declarations it
	// finds in the same file regardless of marker boundaries, so splitting
	// them into separate files in the same package is what keeps each
	// independently gofmt/goimports-stable. FixtureRegion reads every .go
	// file in the directory, in name order, so either shape works
	// transparently.
	Fixture string
	// TextOnly marks a fence pinned as exact text only, never proven to
	// compile: a "Before" snippet documenting API removed in 1.0
	// (evo.MainWith, Group.Each, evo.New, a signature without ctx) or a
	// fence that elides a real declaration's initializer for brevity (the
	// Projection const list). Its Fixture directory's snippet file carries
	// a `//go:build ignore` tag so `go build ./...`/`go vet ./...` never
	// try to type-check text that is either gone from the API or was
	// never complete Go in the first place, alongside a plain buildable
	// file (conventionally doc.go) so the directory still has a package.
	// TestHistoricalFixturesMatchTextOnly checks the tag is actually
	// present when TextOnly is true, and absent when it is false, so a
	// fixture can't drift out of sync with its own TextOnly flag.
	TextOnly bool
}

// SnippetRegion returns the concatenation of every snippetStart/snippetEnd
// marked region in a fixture file's source, in file order, in the same
// format ExtractGoFences produces (marker lines excluded, trailing newline
// preserved). It returns an error wrapping ErrNoRegion when the file has no
// marker pair, and a plain error naming the fixture path when markers are
// unbalanced or a region is empty — each is a fixture authoring defect, not
// a doc drift, and must fail loudly rather than compare successfully
// against nothing.
func SnippetRegion(fixtureSrc []byte, fixturePath string) (string, error) {
	lines := strings.Split(string(fixtureSrc), "\n")

	var collected []string
	collecting := false
	regions := 0
	for _, line := range lines {
		switch strings.TrimSpace(line) {
		case snippetStart:
			if collecting {
				return "", fmt.Errorf("%s: nested %q marker", fixturePath, snippetStart)
			}
			collecting = true
			regions++
		case snippetEnd:
			if !collecting {
				return "", fmt.Errorf("%s: %q without a preceding %q", fixturePath, snippetEnd, snippetStart)
			}
			collecting = false
		default:
			if collecting {
				collected = append(collected, line)
			}
		}
	}
	if collecting {
		return "", fmt.Errorf("%s: %q without a matching %q", fixturePath, snippetStart, snippetEnd)
	}
	if regions == 0 {
		return "", fmt.Errorf("%s: %w", fixturePath, ErrNoRegion)
	}
	if len(collected) == 0 {
		return "", fmt.Errorf("%s: empty snippet region", fixturePath)
	}

	return strings.Join(collected, "\n") + "\n", nil
}

// FixtureRegion returns the concatenation, in filename order, of every
// SnippetRegion found across the non-test .go files in a fixture package
// directory. A file with no marker pair (ErrNoRegion) is scaffolding-only
// and contributes nothing; at least one file in the directory must have a
// region, or this is a fixture authoring defect.
func FixtureRegion(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("read fixture dir %s: %w", dir, err)
	}

	var names []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	var regions []string
	for _, name := range names {
		path := filepath.Join(dir, name)
		src, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read fixture file %s: %w", path, err)
		}
		region, err := SnippetRegion(src, path)
		switch {
		case errors.Is(err, ErrNoRegion):
			continue // scaffolding-only file: nothing to contribute
		case err != nil:
			return "", err
		}
		regions = append(regions, region)
	}
	if len(regions) == 0 {
		return "", fmt.Errorf("%s: %w in any file", dir, ErrNoRegion)
	}

	return strings.Join(regions, ""), nil
}

// NormalizeIndent collapses every run of horizontal whitespace on each line
// — leading indent and internal padding alike — to a single space, and
// trims the result. Docs indent fenced Go with four spaces (Markdown
// convention); a fixture's marked region is reformatted by gofmt every time
// the pre-commit hook runs, to whatever nesting depth its surrounding
// function/import requires and with its own trailing-comment column
// alignment padding — different, and differently-changing, horizontal
// whitespace on each side that carries no semantic information (Go itself
// is whitespace-insensitive between tokens). Normalizing it away before
// comparing lets TestDocFencesMatchFixtures catch real content drift (an
// identifier, a call shape, a removed method, an added or missing line)
// without also tripping on gofmt reformatting a fixture, which would
// otherwise be a false alarm on every commit.
func NormalizeIndent(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	// A leading or trailing blank line is also whitespace gofmt is free to
	// add or remove at a marker boundary (it inserts one between a
	// function's closing brace and a following floating comment) — trim
	// the edges the same way, while leaving blank lines *between* other
	// content (e.g. separating an import block from a func) meaningful.
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}
