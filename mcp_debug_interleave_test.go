package evo

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// TestMCP025_DebugInterleavesWithItem is the output half of MCP-025: a debug
// line must keep the plain buffer coherent with its task row. The preview
// half lives in mcp/guards (TestMCP025_PreviewProfilesFromSnapshot) because
// the preview package is outside the root module's reach.
func TestMCP025_DebugInterleavesWithItem(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &buf, stderr: io.Discard, title: "demo", Debug: debugConfig{level: levelDebug}, color: colorNever, plain: true})
	succeed(out.Task("status"))
	out.debugForTest("index ok")
	_ = out.finish()
	// Plain buffer must keep debug coherent with item.
	if !strings.Contains(buf.String(), "status") {
		t.Fatal(buf.String())
	}
}
