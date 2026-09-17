package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// spec §60: the conformance operation returns
// {target_version, findings[{rule,severity,file,line,summary,migration}]}.
const manualFileReconciliationSource = `package p

import (
	"fmt"
	"os"

	evo "github.com/zachbornheimer/evident-output"
)

func writeConfig(out *evo.Output, path string, data []byte) error {
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		return fmt.Errorf("chmod config: %w", err)
	}
	return nil
}
`

func TestConformanceTool_ReturnsSpecShapeWithMigration(t *testing.T) {
	bin := buildMCP(t)
	srcJSON, _ := json.Marshal(manualFileReconciliationSource)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_conformance","arguments":{"source":` + string(srcJSON) + `}}}`,
	}, "\n") + "\n"
	out := runMCP(t, bin, in)

	var response map[string]any
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var msg map[string]any
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if result, ok := msg["result"].(map[string]any); ok {
			if sc, ok := result["structuredContent"].(map[string]any); ok {
				if sc["schema"] == "evident_output_conformance.v1" {
					response = sc
				}
			}
		}
	}
	if response == nil {
		t.Fatalf("no evident_output_conformance.v1 structuredContent in output: %s", out)
	}
	if response["target_version"] != "next" {
		t.Fatalf("target_version = %v, want %q", response["target_version"], "next")
	}
	findings, _ := response["findings"].([]any)
	var found map[string]any
	for _, raw := range findings {
		f, _ := raw.(map[string]any)
		if f["rule"] == "EVO-FILE-001" {
			found = f
		}
	}
	if found == nil {
		t.Fatalf("EVO-FILE-001 missing from conformance findings: %v", findings)
	}
	for _, field := range []string{"rule", "severity", "file", "summary", "migration"} {
		if v, ok := found[field]; !ok || v == "" {
			t.Fatalf("conformance finding missing/empty field %q: %v", field, found)
		}
	}
}

func TestConformanceTool_NoSourceReturnsExplicitError(t *testing.T) {
	bin := buildMCP(t)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_conformance","arguments":{}}}`,
	}, "\n") + "\n"
	out := runMCP(t, bin, in)
	if !strings.Contains(out, "no source to review") {
		t.Fatalf("expected explicit no-source error, got: %s", out)
	}
}
