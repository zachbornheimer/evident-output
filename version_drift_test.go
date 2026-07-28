package evo_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// Portable surfaces: install guidance must stay on PublishedRelease.
// Discovered automatically so new skills/integrations join the gate without
// a hand-maintained allowlist that itself drifts.
var portableRoots = []string{
	"skills",
	"integrations",
}

var portableFiles = []string{
	"README.md",
}

// Historical / design docs may mention older tags as narrative. They are not
// install surfaces. Do not scan them for pin equality.
var historicalPathFragments = []string{
	"docs/roadmap/",
	"docs/architecture/",
	"docs/adr/",
	"/COMPLETENESS_",
}

// Machine-specific path fragments banned on portable surfaces.
var forbiddenPathFragments = []string{
	"Developer/Personal/evident-output",
	"/Users/zbornheimer/",
	"Local clone (this Mac)",
}

// go get|install|run of this module at a version.
var moduleVersionPin = regexp.MustCompile(
	`github\.com/zachbornheimer/evident-output(?:/cmd/[a-z0-9-]+)?@(v\d+\.\d+\.\d+|latest)`,
)

// Status / pin chrome in prose.
var proseReleasePin = regexp.MustCompile(
	`(?i)(?:\*\*Release:\*\*\s*\*\*|Pinned release:\s*` + "`" + `|^\*\*Pin:\*\*\s*` + "`" + `)(v\d+\.\d+\.\d+)`,
)

// Command lines in generated MCP config (must never be @latest).
var configCommandLatest = regexp.MustCompile(`(?m)^command\s*=\s*.*@latest`)

func TestPublishedRelease_IsSemverTag(t *testing.T) {
	if !regexp.MustCompile(`^v\d+\.\d+\.\d+$`).MatchString(evo.PublishedRelease) {
		t.Fatalf("PublishedRelease = %q, want vMAJOR.MINOR.PATCH", evo.PublishedRelease)
	}
}

func TestVersionDrift_PortableSurface(t *testing.T) {
	root := moduleRoot(t)
	want := evo.PublishedRelease
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
	path := filepath.Join(root, "cmd/evident-output-mcp/config_client.go")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "evo.PublishedRelease") {
		t.Fatal("config_client.go must fall back to evo.PublishedRelease")
	}
	// No string-literal release tags in the generator (class of hardcoded pins).
	if re := regexp.MustCompile(`"(v\d+\.\d+\.\d+)"`); re.MatchString(text) {
		t.Fatalf("config_client.go must not hardcode release tags; use evo.PublishedRelease:\n%s",
			re.FindString(text))
	}
}

func TestVersionDrift_GeneratedConfigNeverPinsLatest(t *testing.T) {
	// Runtime: config printer with Version=dev must emit PublishedRelease, not latest.
	// Covered by package tests under cmd/evident-output-mcp; here we only guard source.
	root := moduleRoot(t)
	path := filepath.Join(root, "cmd/evident-output-mcp/config_client.go")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if configCommandLatest.Match(body) {
		t.Fatal("config_client.go must not emit command = ...@latest")
	}
}

func TestVersionDrift_ReleaseGoIsOnlyPublishedReleaseLiteral(t *testing.T) {
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
		if rel == "release.go" || strings.HasSuffix(rel, "_test.go") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// Hardcoded install tags in non-test Go are the maintenance class.
		if m := regexp.MustCompile(`"(v0\.\d+\.\d+)"`).FindStringSubmatch(string(body)); m != nil {
			// Allow schema-ish versions without leading policy? We use "0.2" not "v0.2".
			t.Errorf("%s: hardcoded release tag %s — use evo.PublishedRelease", rel, m[1])
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

// Silence unused historical list (documented for humans editing the gate).
var _ = historicalPathFragments
