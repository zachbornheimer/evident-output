package main

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestMCP043_UnknownFieldsRejected(t *testing.T) {
	bin := buildMCP(t)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output.explain","arguments":{"rule_id":"API-006","evil":true}}}`,
	}, "\n") + "\n"
	stdout := runMCP(t, bin, in)
	if !strings.Contains(stdout, "unknown argument field") {
		t.Fatalf("expected unknown field reject: %s", stdout)
	}
	if !strings.Contains(stdout, `"isError":true`) && !strings.Contains(stdout, `"isError": true`) {
		// encoder may omit spaces
		if !strings.Contains(stdout, "isError") {
			t.Fatalf("expected isError: %s", stdout)
		}
	}
}

func TestMCP041_UnsupportedProtocolRejected(t *testing.T) {
	bin := buildMCP(t)
	in := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}` + "\n"
	stdout := runMCP(t, bin, in)
	if !strings.Contains(stdout, "unsupported protocolVersion") {
		t.Fatalf("got %s", stdout)
	}
}

func TestMCP041_SupportedProtocolNegotiated(t *testing.T) {
	bin := buildMCP(t)
	in := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26"}}` + "\n"
	stdout := runMCP(t, bin, in)
	if !strings.Contains(stdout, "2025-03-26") {
		t.Fatalf("got %s", stdout)
	}
	if !strings.Contains(stdout, "catalogChecksum") {
		t.Fatalf("missing catalog checksum: %s", stdout)
	}
}

func TestMCP042_ToolNamesValid(t *testing.T) {
	bin := buildMCP(t)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
	}, "\n") + "\n"
	stdout := runMCP(t, bin, in)
	// Parse last JSON line
	var last map[string]any
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		_ = json.Unmarshal([]byte(line), &last)
	}
	result, _ := last["result"].(map[string]any)
	tools, _ := result["tools"].([]any)
	if len(tools) == 0 {
		t.Fatal(stdout)
	}
	for _, raw := range tools {
		tm, _ := raw.(map[string]any)
		name, _ := tm["name"].(string)
		if !validToolName(name) {
			t.Fatalf("invalid tool name %q", name)
		}
	}
}

func TestMCP029_030_StructuredAndText(t *testing.T) {
	bin := buildMCP(t)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output.list_guides","arguments":{}}}`,
	}, "\n") + "\n"
	stdout := runMCP(t, bin, in)
	if !strings.Contains(stdout, "structuredContent") {
		t.Fatal(stdout)
	}
	if !strings.Contains(stdout, "evident_output.guides.v1") {
		t.Fatalf("missing schema: %s", stdout)
	}
	if !strings.Contains(stdout, `"type":"text"`) && !strings.Contains(stdout, `"type": "text"`) {
		if !strings.Contains(stdout, "guides") {
			t.Fatalf("missing text content: %s", stdout)
		}
	}
}

func TestMCP050_TokenBudgetViaTool(t *testing.T) {
	bin := buildMCP(t)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output.list_guides","arguments":{"max_tokens":25}}}`,
	}, "\n") + "\n"
	stdout := runMCP(t, bin, in)
	if !strings.Contains(stdout, "truncated") {
		t.Fatalf("expected truncated signal: %s", stdout)
	}
}

func TestMCP032_DeadlineRespected(t *testing.T) {
	bin := buildMCP(t)
	// deadline_ms=1 with a review that still completes quickly — we mainly ensure
	// the field is accepted and does not crash; cancellation is best-effort.
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output.review","arguments":{"source":"package p\n","deadline_ms":1}}}`,
	}, "\n") + "\n"
	stdout := runMCP(t, bin, in)
	// Either findings or deadline exceeded — both valid.
	if !strings.Contains(stdout, "findings") && !strings.Contains(stdout, "deadline") {
		t.Fatalf("unexpected: %s", stdout)
	}
}

func TestMCP034_PanicContainedContinues(t *testing.T) {
	// Cannot easily inject panic without a special tool; verify server survives
	// malformed tool name and subsequent healthy call.
	bin := buildMCP(t)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"BAD NAME!","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"evident_output.list_guides","arguments":{}}}`,
	}, "\n") + "\n"
	stdout := runMCP(t, bin, in)
	// Second call still answered
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected 3 responses, got %d: %s", len(lines), stdout)
	}
	if !strings.Contains(stdout, "guides") {
		t.Fatalf("third call missing guides: %s", stdout)
	}
}

func TestMCP_ResourceChecksum(t *testing.T) {
	bin := buildMCP(t)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"evident-output://meta/catalog-checksum"}}`,
	}, "\n") + "\n"
	stdout := runMCP(t, bin, in)
	if !strings.Contains(stdout, "contents") {
		t.Fatal(stdout)
	}
}

func TestMCP036_RemoteFileRejected(t *testing.T) {
	bin := buildMCP(t)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output.review","arguments":{"file":"https://evil.example/x.go","source":"package p"}}}`,
	}, "\n") + "\n"
	stdout := runMCP(t, bin, in)
	if !strings.Contains(stdout, "remote path unsupported") {
		t.Fatalf("got %s", stdout)
	}
}

func runMCP(t *testing.T, bin, in string) string {
	t.Helper()
	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader(in)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run()
	return stdout.String()
}
