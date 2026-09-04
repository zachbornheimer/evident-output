package sections_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/sections"
)

// TestEveryListedSectionIDResolves is the list↔get invariant this package
// exists to hold: an agent that walks List() and calls Get(id) on every
// entry must never hit a miss — the exact defect that made the catalog's
// guide↔rule invariant necessary (internal/agent/catalog/catalog_test.go).
func TestEveryListedSectionIDResolves(t *testing.T) {
	all := sections.List()
	if len(all) == 0 {
		t.Fatal("expected at least one section")
	}
	for _, s := range all {
		got, ok := sections.Get(s.ID)
		if !ok {
			t.Errorf("List() advertised id %q but Get(%q) failed", s.ID, s.ID)
		}
		if got.Body == "" {
			t.Errorf("section %q resolved with empty body", s.ID)
		}
	}
}

// TestEmbeddedDocsMatchSource is the drift guard for the generate-at-build
// contract: the embedded copies under embedded/ must be byte-identical to
// the docs/*.md files they were generated from (tools/gensections). A stale
// embedded copy would silently serve outdated prose from a self-contained
// binary — this fails loudly instead, pointing at `go generate ./...`.
func TestEmbeddedDocsMatchSource(t *testing.T) {
	root := repoRoot(t)
	cases := map[string]string{
		"reference.md":       filepath.Join(root, "docs", "reference.md"),
		"development.md":     filepath.Join(root, "docs", "development.md"),
		"mcp.md":             filepath.Join(root, "docs", "mcp.md"),
		"adoption-ladder.md": filepath.Join(root, "docs", "guides", "teaching-ladder.md"),
	}
	for embeddedName, srcPath := range cases {
		want, err := os.ReadFile(srcPath)
		if err != nil {
			t.Fatalf("read source %s: %v", srcPath, err)
		}
		id := sectionIDForFile(embeddedName)
		got, ok := sections.Get(id)
		if !ok {
			t.Fatalf("no section for embedded file %s (id %s)", embeddedName, id)
		}
		if got.Body != string(want) {
			t.Errorf("embedded copy for %s is stale relative to %s — run `go generate ./internal/agent/sections`", embeddedName, srcPath)
		}
	}
}

func sectionIDForFile(file string) string {
	switch file {
	case "reference.md":
		return "reference"
	case "development.md":
		return "development"
	case "mcp.md":
		return "mcp"
	case "adoption-ladder.md":
		return "adoption-ladder"
	default:
		return ""
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above test working directory")
		}
		dir = parent
	}
}
