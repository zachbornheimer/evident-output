package main

import (
	"strings"
	"testing"
)

func TestConfigClient_GrokPrintsTOML(t *testing.T) {
	body, err := clientConfig("grok")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"[mcp_servers.evident-output]",
		`command = "evident-output-mcp"`,
		"enabled = true",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in:\n%s", want, body)
		}
	}
	// Persistent command line must not be unpinned @latest (comments may mention go install @latest).
	for _, line := range strings.Split(body, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "command") && strings.Contains(trim, "@latest") {
			t.Fatalf("persistent command must not use @latest:\n%s", line)
		}
	}
}

func TestConfigClient_AllHosts(t *testing.T) {
	for _, c := range []string{"claude-code", "codex", "gemini", "grok", "opencode"} {
		body, err := clientConfig(c)
		if err != nil {
			t.Fatalf("%s: %v", c, err)
		}
		if !strings.Contains(body, "evident-output") {
			t.Fatalf("%s: missing server name:\n%s", c, body)
		}
	}
}

func TestConfigClient_Unknown(t *testing.T) {
	if _, err := clientConfig("nope"); err == nil {
		t.Fatal("expected error")
	}
}
