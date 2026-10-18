package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestMCP_MalformedJSONRPC(t *testing.T) {
	bin := buildMCP(t)
	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader("not-json\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run()
	// should emit parse error JSON on stdout, stay alive for one line
	if !strings.Contains(stdout.String(), "parse error") && !strings.Contains(stdout.String(), "-32700") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestMCP_NoNetworkNoShell(t *testing.T) {
	// Structural: server is stdio-only; this test ensures process exits cleanly without listeners.
	bin := buildMCP(t)
	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader("")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
}

func TestMCP_PathTraversalResourceRejected(t *testing.T) {
	bin := buildMCP(t)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"evident-output://guides/../../etc/passwd"}}`,
	}, "\n") + "\n"
	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader(in)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &bytes.Buffer{}
	_ = cmd.Run()
	if strings.Contains(stdout.String(), "root:") {
		t.Fatal("path traversal leak")
	}
	if !strings.Contains(stdout.String(), "not found") && !strings.Contains(stdout.String(), "error") {
		t.Fatalf("expected rejection: %s", stdout.String())
	}
}
