package main

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestMCP_001_RejectsToolsBeforeInitialize(t *testing.T) {
	bin := buildMCP(t)
	// tools/list without initialize
	in := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n"
	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader(in)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	_ = cmd.Run()
	var msg map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout.String())), &msg); err != nil {
		t.Fatalf("stdout=%q err=%v", stdout.String(), err)
	}
	errObj, _ := msg["error"].(map[string]any)
	if errObj == nil {
		t.Fatalf("expected error before initialize, got %v", msg)
	}
	msgText, _ := errObj["message"].(string)
	if !strings.Contains(msgText, "not initialized") {
		t.Fatalf("message=%q", msgText)
	}
}

func TestMCP_001_ToolsOKAfterInitialize(t *testing.T) {
	bin := buildMCP(t)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
	}, "\n") + "\n"
	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader(in)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "evident_output_list_guides") {
		t.Fatal(stdout.String())
	}
}
