package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCP_InitializeAndToolsListStdoutPurity(t *testing.T) {
	bin := buildMCP(t)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"evident_output.list_guides","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"evident_output.explain","arguments":{"rule_id":"API-006"}}}`,
	}, "\n") + "\n"

	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader(in)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v stderr=%s", err, stderr.String())
	}
	// MCP-003: every stdout line is JSON-RPC
	for i, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line == "" {
			continue
		}
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatalf("stdout line %d not JSON: %q err=%v", i, line, err)
		}
		if msg["jsonrpc"] != "2.0" {
			t.Fatalf("missing jsonrpc on line %d: %v", i, msg)
		}
	}
	// MCP-004: logs on stderr only
	if !strings.Contains(stderr.String(), "evident-output-mcp") {
		t.Fatalf("expected stderr log, got %q", stderr.String())
	}
	if strings.Contains(stdout.String(), "starting (stdio)") {
		t.Fatal("server log leaked to stdout")
	}
	if !strings.Contains(stdout.String(), "evident_output.list_guides") {
		t.Fatal("tools/list missing guide tool")
	}
	if !strings.Contains(stdout.String(), "API-006") && !strings.Contains(stdout.String(), "explicit Start") {
		t.Fatalf("explain missing API-006: %s", stdout.String())
	}
}

func buildMCP(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "evident-output-mcp")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "."
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}
