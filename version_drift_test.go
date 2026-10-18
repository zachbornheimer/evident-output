package evo

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"
)

// Portable surfaces: install guidance must stay on PublishedRelease.
// Discovered automatically so new skills/integrations join the gate without
// a hand-maintained allowlist that itself drifts.
var portableRoots = []string{
	"mcp/skills",
	"mcp/integrations",
}

var portableFiles = []string{
	"README.md",
}

// Machine-specific path fragments banned on portable surfaces.
var forbiddenPathFragments = []string{
	"Developer/Personal/evident-output",
	"/Users/zbornheimer/",
	"Local clone (this Mac)",
}

// go get|install|run of this module at a version.
var moduleVersionPin = regexp.MustCompile(
	`github\.com/zachbornheimer/evident-output(?:/v2|/mcp/v2/cmd/[a-z0-9-]+)?@(v\d+\.\d+\.\d+|latest)`,
)

// Status / pin chrome in prose.
var proseReleasePin = regexp.MustCompile(
	`(?i)(?:\*\*Release:\*\*\s*\*\*|Pinned release:\s*` + "`" + `|^\*\*Pin:\*\*\s*` + "`" + `)(v\d+\.\d+\.\d+)`,
)

// Command lines in generated MCP config (must never be @latest).
var configCommandLatest = regexp.MustCompile(`(?m)^command\s*=\s*.*@latest`)

func TestPublishedRelease_IsSemverTag(t *testing.T) {
	if !regexp.MustCompile(`^v\d+\.\d+\.\d+$`).MatchString(wire.Release) {
		t.Fatalf("PublishedRelease = %q, want vMAJOR.MINOR.PATCH", wire.Release)
	}
}

func TestVersionDrift_PortableSurface(t *testing.T) {
	root := moduleRoot(t)
	want := wire.Release
	files := collectPortableMarkdown(t, root)
	if len(files) < 5 {
		t.Fatalf("expected several portable markdown files, found %d", len(files))
	}

	for _, path := range files {
		rel, _ := filepath.Rel(root, path)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		text := string(body)
		checkPortableText(t, rel, text, want)
	}
}

func TestVersionDrift_ConfigClientUsesPublishedRelease(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "mcp/cmd/evident-output-mcp/config_client.go")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, wire.Release) {
		t.Fatalf("config_client.go must fall back to the library pin %s", wire.Release)
	}
}

func TestVersionDrift_GeneratedConfigNeverPinsLatest(t *testing.T) {
	// Runtime: config printer with Version=dev must emit PublishedRelease, not latest.
	// Covered by package tests under mcp/cmd/evident-output-mcp; here we only guard source.
	root := moduleRoot(t)
	path := filepath.Join(root, "mcp/cmd/evident-output-mcp/config_client.go")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if configCommandLatest.Match(body) {
		t.Fatal("config_client.go must not emit command = ...@latest")
	}
}

func TestVersionDrift_PublishedReleaseHasOneLiteral(t *testing.T) {
	// Prevent a second "source of truth" const drifting beside PublishedRelease.
	root := moduleRoot(t)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "vendor" || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "format.go" || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// Hardcoded install tags in non-test Go are the maintenance class.
		if m := regexp.MustCompile(`"(v0\.\d+\.\d+)"`).FindStringSubmatch(string(body)); m != nil {
			// Allow schema-ish versions without leading policy? We use "0.2" not "v0.2".
			t.Errorf("%s: hardcoded release tag %s — use PublishedRelease", rel, m[1])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func checkPortableText(t *testing.T, rel, text, want string) {
	t.Helper()

	if !strings.Contains(text, want) {
		t.Errorf("%s: must mention PublishedRelease %s", rel, want)
	}

	for _, m := range moduleVersionPin.FindAllStringSubmatch(text, -1) {
		pin := m[1]
		if pin == "latest" {
			t.Errorf("%s: forbids module install @latest (%q)", rel, m[0])
			continue
		}
		if pin != want {
			t.Errorf("%s: stale module pin %q (want %s)", rel, m[0], want)
		}
	}

	for _, m := range proseReleasePin.FindAllStringSubmatch(text, -1) {
		if m[1] != want {
			t.Errorf("%s: prose pin %q (want %s)", rel, m[0], want)
		}
	}

	for _, frag := range forbiddenPathFragments {
		if strings.Contains(text, frag) {
			t.Errorf("%s: forbids personal/portable-hostile fragment %q", rel, frag)
		}
	}
}

func collectPortableMarkdown(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	for _, rel := range portableFiles {
		out = append(out, filepath.Join(root, rel))
	}
	for _, dir := range portableRoots {
		base := filepath.Join(root, dir)
		_ = filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			name := d.Name()
			if strings.HasSuffix(name, ".md") || name == "SKILL.md" {
				out = append(out, path)
			}
			return nil
		})
	}
	return out
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(wd, "go.mod")); err != nil {
		t.Fatalf("expected module root at %s: %v", wd, err)
	}
	return wd
}

func TestVersionDrift_McpModuleShipsAgainstPublishedRelease(t *testing.T) {
	root := moduleRoot(t)
	wants := map[string]string{
		"mcp/internal/buildversion/buildversion.go": `PublishedRelease = "` + wire.Release + `"`,
		"mcp/go.mod": "github.com/zachbornheimer/evident-output/v2 " + wire.Release,
	}
	for rel, want := range wants {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), want) {
			t.Errorf("%s must contain %q (run mise run sync-release-pins)", rel, want)
		}
	}
}
