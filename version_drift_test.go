package evo_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// Portable guidance that must recommend PublishedRelease (not an older pin).
var pinRequiredFiles = []string{
	"README.md",
	"skills/cli-output/SKILL.md",
	"skills/evident-output/SKILL.md",
	"integrations/grok/README.md",
	"integrations/claude-code/README.md",
	"integrations/codex/README.md",
	"integrations/gemini/README.md",
	"integrations/opencode/README.md",
}

// Paths that must not appear in portable guidance (machine-specific).
var forbiddenPathFragments = []string{
	"Developer/Personal/evident-output",
}

// Install-ish lines must not use @latest.
var installLatest = regexp.MustCompile(`evident-output[^` + "`" + `\s]*@latest`)

// Older release pins that must not appear as install targets in pin files.
var staleInstallPin = regexp.MustCompile(`evident-output(@|/cmd/[^@\s]+@)v0\.2\.[0-9]+`)

func TestPublishedRelease_IsSemverTag(t *testing.T) {
	if !regexp.MustCompile(`^v\d+\.\d+\.\d+$`).MatchString(evo.PublishedRelease) {
		t.Fatalf("PublishedRelease = %q, want vMAJOR.MINOR.PATCH", evo.PublishedRelease)
	}
}

func TestVersionDrift_PortablePinsMatchPublishedRelease(t *testing.T) {
	root := moduleRoot(t)
	want := evo.PublishedRelease

	for _, rel := range pinRequiredFiles {
		path := filepath.Join(root, rel)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		text := string(body)

		if !strings.Contains(text, want) {
			t.Errorf("%s: must mention PublishedRelease %s", rel, want)
		}

		// Stale install pins (any v0.2.N that is not the current release).
		for _, m := range staleInstallPin.FindAllString(text, -1) {
			if !strings.Contains(m, want) {
				t.Errorf("%s: stale install pin %q (want %s)", rel, m, want)
			}
		}

		if installLatest.MatchString(text) {
			t.Errorf("%s: forbids module/MCP install @latest", rel)
		}

		for _, frag := range forbiddenPathFragments {
			if strings.Contains(text, frag) {
				t.Errorf("%s: forbids personal-machine path fragment %q", rel, frag)
			}
		}
	}
}

func TestVersionDrift_ConfigClientFallbackUsesPublishedRelease(t *testing.T) {
	root := moduleRoot(t)
	path := filepath.Join(root, "cmd/evident-output-mcp/config_client.go")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	// Must reference evo.PublishedRelease rather than a hardcoded older tag.
	if !strings.Contains(text, "evo.PublishedRelease") {
		t.Fatal("config_client.go must fall back to evo.PublishedRelease")
	}
	if strings.Contains(text, `"v0.2.8"`) || strings.Contains(text, `"v0.2.9"`) {
		t.Fatal("config_client.go must not hardcode a stale release fallback")
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Tests run with package dir as cwd (module root for package evo_test).
	if _, err := os.Stat(filepath.Join(wd, "go.mod")); err != nil {
		t.Fatalf("expected module root at %s: %v", wd, err)
	}
	return wd
}
