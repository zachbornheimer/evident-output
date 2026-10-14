// Command sync-release-pins rewrites install pins on portable surfaces to
// match wire.Release. Run after changing Release in internal/project/wire/stream.go.
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
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		fail(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		fail(fmt.Errorf("run from module root: %w", err))
	}

	want, err := publishedReleaseFrom(root)
	if err != nil {
		fail(err)
	}
	fmt.Fprintf(os.Stderr, "sync-release-pins: target %s\n", want)

	// Any previous semver pin on this module → want.
	rootPin := regexp.MustCompile(
		`github\.com/zachbornheimer/evident-output(?:/v2)?@(?:v\d+\.\d+\.\d+|latest)`,
	)
	mcpPin := regexp.MustCompile(
		`github\.com/zachbornheimer/evident-output/mcp(?:/v2)?(/cmd/[a-z0-9-]+)@(?:v\d+\.\d+\.\d+|latest)`,
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
	for _, pin := range literalPins {
		updated, err := pin.sync(root, want)
		if err != nil {
			fail(err)
		}
		if updated {
			fmt.Fprintf(os.Stderr, "  updated %s\n", pin.path)
			changed++
		}
	}
	for _, path := range paths {
		body, err := fs.ReadFile(repo, path)
		if err != nil {
			fail(err)
		}
		orig := string(body)
		next := rootPin.ReplaceAllString(orig, "github.com/zachbornheimer/evident-output/v2@"+want)
		next = mcpPin.ReplaceAllString(next, "github.com/zachbornheimer/evident-output/mcp/v2${1}@"+want)
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

// literalPin is a release literal in a source file (not prose) that must
// equal the root release: group 1 precedes the version, group 2 follows it.
type literalPin struct {
	path string
	re   *regexp.Regexp
}

var literalPins = []literalPin{
	// The MCP binary's fallback release when it carries no build info.
	{"mcp/internal/buildversion/buildversion.go", regexp.MustCompile(`(PublishedRelease = ")v\d+\.\d+\.\d+(")`)},
	// The MCP module's require of the root module it ships against.
	{"mcp/go.mod", regexp.MustCompile(`(github\.com/zachbornheimer/evident-output/v2 )v\d+\.\d+\.\d+()`)},
}

func (p literalPin) sync(root, want string) (bool, error) {
	file := filepath.Join(root, filepath.FromSlash(p.path))
	body, err := fs.ReadFile(os.DirFS(root), p.path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", p.path, err)
	}
	if !p.re.Match(body) {
		return false, fmt.Errorf("release literal not found in %s", p.path)
	}
	next := p.re.ReplaceAll(body, []byte("${1}"+want+"${2}"))
	if string(next) == string(body) {
		return false, nil
	}
	if err := os.WriteFile(file, next, 0o600); err != nil {
		return false, fmt.Errorf("write %s: %w", p.path, err)
	}
	return true, nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

// releaseFile declares the release this build reports (wire.Release).
const releaseFile = "internal/project/wire/stream.go"

func publishedReleaseFrom(root string) (string, error) {
	// Relative path: this command already required cwd to be the module root.
	body, err := os.ReadFile(releaseFile)
	if err != nil {
		return "", fmt.Errorf("read %s in %s: %w", releaseFile, root, err)
	}
	m := regexp.MustCompile(`(?m)^const Release = "([^"]+)"`).FindSubmatch(body)
	if m == nil {
		return "", fmt.Errorf("const Release not found in %s", releaseFile)
	}
	return string(m[1]), nil
}
