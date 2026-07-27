package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCP_ReviewAndPreviewTools(t *testing.T) {
	bin := buildMCP(t)
	src := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func f() {
  out := evo.NewWithOptions()
  t := out.Task("x")
  t.Start()
  fmt.Printf("x")
}
`
	// escape for JSON
	b, _ := json.Marshal(src)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"evident_output_review","arguments":{"source":` + string(b) + `,"file":"x.go"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"evident_output_preview","arguments":{"subject":"demo","item":"status","state":"blocked"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"evident_output_get_guidance","arguments":{"ids":["common-api","nope"]}}}`,
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
	if !strings.Contains(out, "API-006") && !strings.Contains(out, "STREAM-003") {
		t.Fatalf("review findings missing: %s", out)
	}
	if !strings.Contains(out, "profiles") && !strings.Contains(out, "wide") {
		t.Fatalf("preview missing profiles: %s", out)
	}
	if !strings.Contains(out, "missing") && !strings.Contains(out, "nope") {
		t.Logf("get_guidance: %s", out)
	}
}

func TestMCP_ResourceRead(t *testing.T) {
	bin := buildMCP(t)
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"evident-output://guides/common-api"}}`,
	}, "\n") + "\n"
	cmd := exec.Command(bin)
	cmd.Stdin = strings.NewReader(in)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "evo.For") && !strings.Contains(stdout.String(), "Items") {
		t.Fatalf("resource body: %s", stdout.String())
	}
}

// ensure buildMCP is shared — defined in mcp_test.go same package
var _ = filepath.Separator
var _ = os.DevNull
