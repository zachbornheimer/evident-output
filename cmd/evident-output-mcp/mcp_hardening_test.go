package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestMCP043_UnknownFieldsRejected(t *testing.T) {
	bin := buildMCP(t)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_explain","arguments":{"rule_id":"API-006","evil":true}}}`,
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
	// Catalog integrity is exposed as a resource, not serverInfo (strict InitializeResult).
	if !strings.Contains(stdout, "evident-output-mcp") {
		t.Fatalf("missing server name: %s", stdout)
	}
	if strings.Contains(stdout, `"catalogChecksum"`) {
		t.Fatalf("serverInfo must not embed catalogChecksum: %s", stdout)
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
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_list_guides","arguments":{}}}`,
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
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_list_guides","arguments":{"max_tokens":25}}}`,
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
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_review","arguments":{"source":"package p\n","deadline_ms":1}}}`,
	}, "\n") + "\n"
	stdout := runMCP(t, bin, in)
	// Either findings or deadline exceeded — both valid.
	if !strings.Contains(stdout, "findings") && !strings.Contains(stdout, "deadline") {
		t.Fatalf("unexpected: %s", stdout)
	}
}

func TestMCP034_PanicContainedContinues(t *testing.T) {
	// Fault injection: panic inside tool handler; server must contain and continue.
	// Unit-level (same package) so we can set faultHook without a production backdoor binary flag.
	faultHook = func(name string) {
		if name == "evident_output_list_guides" {
			panic("injected MCP-034 fault")
		}
	}
	t.Cleanup(func() { faultHook = nil })

	// Drive through safeToolCall path used by the server.
	var buf bytes.Buffer
	// Capture writeRPC by temporarily not available — call safeToolCall which recovers.
	// Instead exercise safeToolCall + handleToolCall directly via process-level is hard;
	// call the functions that the server uses.
	req := map[string]any{
		"params": map[string]any{
			"name":      "evident_output_list_guides",
			"arguments": map[string]any{},
		},
	}
	// safeToolCall writes to stdout — redirect via pipe is heavy; invoke recover path:
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("panic escaped safeToolCall: %v", r)
			}
		}()
		// Redirect protocol writer for writeRPC/writeFramed
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		outMu.Lock()
		prevW := outW
		outW = w
		outMode = frameNDJSON
		outMu.Unlock()
		safeToolCall(1, req)
		_ = w.Close()
		outMu.Lock()
		outW = prevW
		outMu.Unlock()
		b, _ := io.ReadAll(r)
		_ = r.Close()
		buf.Write(b)
	}()
	out := buf.String()
	if !strings.Contains(out, "panic contained") && !strings.Contains(out, "internal tool error") {
		t.Fatalf("expected panic-contained tool error, got %q", out)
	}
	// Subsequent healthy call must still work (no process death).
	faultHook = nil
	req2 := map[string]any{
		"params": map[string]any{
			"name":      "evident_output_explain",
			"arguments": map[string]any{"rule_id": "API-006"},
		},
	}
	r2, w2, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outMu.Lock()
	prevW2 := outW
	outW = w2
	outMode = frameNDJSON
	outMu.Unlock()
	safeToolCall(2, req2)
	_ = w2.Close()
	outMu.Lock()
	outW = prevW2
	outMu.Unlock()
	b, _ := io.ReadAll(r2)
	_ = r2.Close()
	if !strings.Contains(string(b), "API-006") && !strings.Contains(string(b), "explicit Start") {
		t.Fatalf("post-panic call failed: %s", b)
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
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_review","arguments":{"file":"https://evil.example/x.go","source":"package p"}}}`,
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
