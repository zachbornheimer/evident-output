package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// evoViolationSource is real Go source with a genuine evo misuse (STREAM-003:
// fmt.Printf alongside an evo Init) — reading it from disk must reach the
// same review outcome as pasting it inline via `source`.
const evoViolationSource = `package p

import (
	"fmt"

	evo "github.com/zachbornheimer/evident-output"
)

func run() {
	out := evo.Init(evo.Config{})
	fmt.Printf("progress\n")
	_ = out
}
`

func TestReview_FileParamReadsAbsolutePath(t *testing.T) {
	bin := buildMCP(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "violation.go")
	if err := os.WriteFile(path, []byte(evoViolationSource), 0o600); err != nil {
		t.Fatal(err)
	}
	pathJSON, _ := json.Marshal(path)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_review","arguments":{"file":` + string(pathJSON) + `}}}`,
	}, "\n") + "\n"
	out := runMCP(t, bin, in)
	if strings.Contains(out, "parse error") {
		t.Fatalf("file param did not read disk content, got a parse error instead: %s", out)
	}
	if !strings.Contains(out, "STREAM-003") {
		t.Fatalf("expected STREAM-003 finding from the real file content: %s", out)
	}
}

func TestReview_FileParamMissingPathReturnsExplicitError(t *testing.T) {
	bin := buildMCP(t)
	missing := filepath.Join(t.TempDir(), "does-not-exist.go")
	pathJSON, _ := json.Marshal(missing)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_review","arguments":{"file":` + string(pathJSON) + `}}}`,
	}, "\n") + "\n"
	out := runMCP(t, bin, in)
	if strings.Contains(out, "parse error") {
		t.Fatalf("expected an honest read-failure error, got a generic parse error: %s", out)
	}
	if !strings.Contains(out, "cannot read") || !strings.Contains(out, missing) {
		t.Fatalf("expected an explicit cannot-read error naming the path, got: %s", out)
	}
}

func TestReview_FilesMapReadsAbsolutePaths(t *testing.T) {
	bin := buildMCP(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "violation.go")
	if err := os.WriteFile(path, []byte(evoViolationSource), 0o600); err != nil {
		t.Fatal(err)
	}
	pathJSON, _ := json.Marshal(path)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_review","arguments":{"kind":"package","files":{"violation.go":` + string(pathJSON) + `}}}}`,
	}, "\n") + "\n"
	out := runMCP(t, bin, in)
	if strings.Contains(out, "parse error") || strings.Contains(out, "no files provided") {
		t.Fatalf("files map did not read disk content: %s", out)
	}
	if !strings.Contains(out, "STREAM-003") {
		t.Fatalf("expected STREAM-003 finding from the real file content via files map: %s", out)
	}
}

func TestReview_FilesMapUnreadablePathReturnsExplicitError(t *testing.T) {
	bin := buildMCP(t)
	missing := filepath.Join(t.TempDir(), "does-not-exist.go")
	pathJSON, _ := json.Marshal(missing)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_review","arguments":{"kind":"package","files":{"violation.go":` + string(pathJSON) + `}}}}`,
	}, "\n") + "\n"
	out := runMCP(t, bin, in)
	if !strings.Contains(out, "cannot read") || !strings.Contains(out, missing) {
		t.Fatalf("expected an explicit cannot-read error naming the path, got: %s", out)
	}
}

func TestReview_FilesMapEmptyShapeReturnsExplicitError(t *testing.T) {
	bin := buildMCP(t)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_review","arguments":{"kind":"package","files":{"a.go":123}}}}`,
	}, "\n") + "\n"
	out := runMCP(t, bin, in)
	if strings.Contains(out, "parse error") {
		t.Fatalf("expected an explicit empty-decode error, got a generic parse error: %s", out)
	}
	if !strings.Contains(out, "empty source after decode") {
		t.Fatalf("expected explicit empty-decode error, got: %s", out)
	}
}
