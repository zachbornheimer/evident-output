package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestReadMCPMessage_NDJSON(t *testing.T) {
	r := bufio.NewReader(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}` + "\n"))
	msg, mode, err := readMCPMessage(r)
	if err != nil {
		t.Fatal(err)
	}
	if mode != frameNDJSON {
		t.Fatalf("mode=%v want NDJSON", mode)
	}
	var m map[string]any
	if err := json.Unmarshal(msg, &m); err != nil {
		t.Fatal(err)
	}
	if m["method"] != "ping" {
		t.Fatalf("got %v", m)
	}
}

func TestReadMCPMessage_ContentLength(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`
	raw := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	r := bufio.NewReader(strings.NewReader(raw))
	msg, mode, err := readMCPMessage(r)
	if err != nil {
		t.Fatal(err)
	}
	if mode != frameContentLength {
		t.Fatalf("mode=%v want ContentLength", mode)
	}
	if string(msg) != body {
		t.Fatalf("body mismatch:\n%s\nvs\n%s", msg, body)
	}
}

func TestWriteFramed_ContentLengthRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	outMu.Lock()
	outMode = frameContentLength
	outW = &buf
	outMu.Unlock()
	t.Cleanup(func() {
		outMu.Lock()
		outMode = frameNDJSON
		outW = os.Stdout
		outMu.Unlock()
	})

	writeRPC(1, map[string]any{"ok": true})
	got := buf.String()
	if !strings.HasPrefix(got, "Content-Length:") {
		t.Fatalf("expected Content-Length frame, got %q", got)
	}
	// Parse back
	r := bufio.NewReader(strings.NewReader(got))
	msg, mode, err := readMCPMessage(r)
	if err != nil {
		t.Fatal(err)
	}
	if mode != frameContentLength {
		t.Fatalf("mode=%v", mode)
	}
	var resp map[string]any
	if err := json.Unmarshal(msg, &resp); err != nil {
		t.Fatal(err)
	}
	if resp["jsonrpc"] != "2.0" {
		t.Fatalf("missing jsonrpc: %v", resp)
	}
}

func TestInitializeResult_NoExtraServerInfoFields(t *testing.T) {
	// Strict hosts reject unknown InitializeResult / serverInfo properties.
	// catalogChecksum must live on a resource, not serverInfo.
	var out bytes.Buffer
	in := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}` + "\n")
	// run one message only by closing after
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Use limited reader that EOF after one message is processed... runStdioServer blocks until EOF.
		runStdioServer(in, &out)
	}()
	<-done
	line := bytes.TrimSpace(out.Bytes())
	var resp map[string]any
	if err := json.Unmarshal(line, &resp); err != nil {
		t.Fatalf("unmarshal %q: %v", line, err)
	}
	result, _ := resp["result"].(map[string]any)
	si, _ := result["serverInfo"].(map[string]any)
	if _, ok := si["catalogChecksum"]; ok {
		t.Fatalf("serverInfo must not include catalogChecksum: %v", si)
	}
	if si["name"] != "evident-output-mcp" {
		t.Fatalf("name=%v", si["name"])
	}
}
