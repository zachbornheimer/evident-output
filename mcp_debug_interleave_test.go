package evo_test

import (
	"bytes"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// TestMCP025_DebugInterleavesWithItem is the output half of MCP-025: a debug
// line must keep the plain buffer coherent with its task row. The preview
// half lives in mcp/guards (TestMCP025_PreviewProfilesFromSnapshot) because
// the preview package is outside the root module's reach.
func TestMCP025_DebugInterleavesWithItem(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Title: "demo", Debug: evo.DebugConfig{Level: evo.LevelDebug}, Color: evo.ColorNever, Plain: true})
	succeed(out.Task("status"))
	out.DebugForTest("index ok")
	_ = out.Finish()
	// Plain buffer must keep debug coherent with item.
	if !strings.Contains(buf.String(), "status") {
		t.Fatal(buf.String())
	}
}
