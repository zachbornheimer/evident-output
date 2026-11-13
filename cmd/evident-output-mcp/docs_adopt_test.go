package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

// TestMCP_DocsAndAdoptTools exercises the three tools this work order added
// through a real stdio handshake against the built binary — the same path
// TestMCP_ReviewAndPreviewTools proves for the pre-existing tools.
func TestMCP_DocsAndAdoptTools(t *testing.T) {
	bin := buildMCP(t)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_list_sections","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"evident_output_get_documentation","arguments":{"ids":["reference","adoption-ladder","nope"]}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"evident_output_adopt_plan","arguments":{"directory":"../../internal/agent/adopt/testdata/mixed"}}}`,
	}, "\n") + "\n"

	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader(in)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v stderr=%s", err, stderr.String())
	}
	out := stdout.String()

	if !strings.Contains(out, "evident_output.sections.v1") {
		t.Fatalf("list_sections missing schema: %s", out)
	}
	if !strings.Contains(out, `"id":"reference"`) {
		t.Fatalf("list_sections missing reference section: %s", out)
	}
	if !strings.Contains(out, "evident_output.documentation.v1") {
		t.Fatalf("get_documentation missing schema: %s", out)
	}
	if !strings.Contains(out, "adoption-ladder") || !strings.Contains(out, `"nope"`) {
		t.Fatalf("get_documentation missing found/missing ids: %s", out)
	}
	if !strings.Contains(out, "evident_output_adopt_plan.v1") {
		t.Fatalf("adopt_plan missing schema: %s", out)
	}
	if !strings.Contains(out, "log.Fatal") {
		t.Fatalf("adopt_plan missing expected fixture findings: %s", out)
	}
	if !strings.Contains(out, `"next_action"`) || !strings.Contains(out, `"remaining"`) {
		t.Fatalf("adopt_plan missing paged fields: %s", out)
	}
}

// TestMCP_InitializeInstructionsDriveReviewLoop pins the Svelte-mold
// contract: the server's initialize instructions must direct the agent to
// use the server and to re-run review until it is clean.
func TestMCP_InitializeInstructionsDriveReviewLoop(t *testing.T) {
	bin := buildMCP(t)
	in := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"
	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader(in)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v stderr=%s", err, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "MUST be used whenever CLI output") {
		t.Fatalf("instructions missing MUST-use directive: %s", out)
	}
	if !strings.Contains(out, "call evident_output_review again") {
		t.Fatalf("instructions missing re-run-until-clean directive: %s", out)
	}
	if strings.Contains(out, "list_guides") {
		t.Fatalf("instructions must not mention list_guides: %s", out)
	}
}

func TestMCP_DocsAliasesRemainCallable(t *testing.T) {
	bin := buildMCP(t)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_list_guides","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"evident_output_get_guidance","arguments":{"ids":["common-api"]}}}`,
	}, "\n") + "\n"
	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader(in)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v stderr=%s", err, stderr.String())
	}
	out := stdout.String()
	if strings.Contains(out, "unknown tool") {
		t.Fatalf("aliases must stay callable: %s", out)
	}
	if !strings.Contains(out, "evident_output.guides.v1") {
		t.Fatalf("list_guides alias missing catalog schema: %s", out)
	}
	if !strings.Contains(out, "evident_output.guidance.v1") {
		t.Fatalf("get_guidance alias missing guidance schema: %s", out)
	}
}
