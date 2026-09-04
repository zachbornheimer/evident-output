// Command gensections copies the docs the MCP server's list_sections /
// get_documentation tools serve into internal/agent/sections/embedded, the
// only directory go:embed can reach from that package. docs/*.md stays the
// single source of truth — this command's only job is to make Go's embed
// restriction (no "../") not force duplicated prose. Run after editing any
// source doc listed in sourceDocs:
//
//	go generate ./internal/agent/sections
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// sourceDocs maps each served section's stable ID to its source-of-truth
// path (relative to the module root) and the embedded copy's filename.
var sourceDocs = map[string]string{
	"reference.md":       filepath.Join("docs", "reference.md"),
	"development.md":     filepath.Join("docs", "development.md"),
	"mcp.md":             filepath.Join("docs", "mcp.md"),
	"adoption-ladder.md": filepath.Join("docs", "guides", "teaching-ladder.md"),
}

func main() {
	root, err := moduleRoot()
	if err != nil {
		fail(err)
	}
	destDir := filepath.Join(root, "internal", "agent", "sections", "embedded")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		fail(err)
	}
	for destName, srcRel := range sourceDocs {
		src := filepath.Join(root, srcRel)
		body, err := os.ReadFile(src)
		if err != nil {
			fail(fmt.Errorf("read %s: %w", srcRel, err))
		}
		dest := filepath.Join(destDir, destName)
		if err := os.WriteFile(dest, body, 0o644); err != nil {
			fail(fmt.Errorf("write %s: %w", dest, err))
		}
		fmt.Fprintf(os.Stderr, "gensections: %s -> %s\n", srcRel, dest)
	}
}

func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gensections:", err)
	os.Exit(1)
}
