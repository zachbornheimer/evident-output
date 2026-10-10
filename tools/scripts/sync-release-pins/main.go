// Command sync-release-pins rewrites install pins on portable surfaces to
// match evo.PublishedRelease. Run after changing PublishedRelease in format.go.
//
//	go run ./tools/scripts/sync-release-pins
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	evo "github.com/zachbornheimer/evident-output"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		fail(fmt.Errorf("run from module root: %w", err))
	}

	want := evo.PublishedRelease
	fmt.Fprintf(os.Stderr, "sync-release-pins: target %s\n", want)

	// Any previous semver pin on this module → want.
	modulePin := regexp.MustCompile(
		`github\.com/zachbornheimer/evident-output((?:/mcp/cmd/[a-z0-9-]+)?)@(?:v\d+\.\d+\.\d+|latest)`,
	)
	// Prose chrome pins.
	prosePin := regexp.MustCompile(
		`(?m)(\*\*Release:\*\*\s*\*\*|Pinned release:\s*` + "`" + `|^\*\*Pin:\*\*\s*` + "`" + `)v\d+\.\d+\.\d+`,
	)

	repo := os.DirFS(root)
	var paths []string
	paths = append(paths, "README.md")
	paths = append(paths, "mcp/docs/mcp.md")
	for _, dir := range []string{"mcp/skills", "mcp/integrations"} {
		_ = fs.WalkDir(repo, dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if strings.HasSuffix(d.Name(), ".md") {
				paths = append(paths, path)
			}
			return nil
		})
	}

	changed := 0
	for _, path := range paths {
		body, err := fs.ReadFile(repo, path)
		if err != nil {
			fail(err)
		}
		orig := string(body)
		next := modulePin.ReplaceAllString(orig, "github.com/zachbornheimer/evident-output$1@"+want)
		next = prosePin.ReplaceAllString(next, "${1}"+want)
		// Common alternate prose forms.
		next = regexp.MustCompile("`v\\d+\\.\\d+\\.\\d+` \\(never").ReplaceAllString(next, "`"+want+"` (never")
		next = regexp.MustCompile("\\*\\*Pin:\\*\\* `v\\d+\\.\\d+\\.\\d+`").ReplaceAllString(next, "**Pin:** `"+want+"`")
		next = regexp.MustCompile("\\*\\*Pinned release:\\*\\* `v\\d+\\.\\d+\\.\\d+`").ReplaceAllString(next, "**Pinned release:** `"+want+"`")

		if next != orig {
			if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), []byte(next), 0o600); err != nil {
				fail(err)
			}
			fmt.Fprintf(os.Stderr, "  updated %s\n", path)
			changed++
		}
	}
	fmt.Fprintf(os.Stderr, "sync-release-pins: %d file(s) updated\n", changed)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
