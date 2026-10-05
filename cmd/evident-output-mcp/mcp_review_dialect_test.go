package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pinnedDeleteSrc uses the 1.0 object-first Delete(object, fn): current
// for a v1.0.0 pin, rewritten only from the 1.1 dialect (API-032).
const pinnedDeleteSrc = `package main

import evo "github.com/zachbornheimer/evident-output"

func prune(g *evo.GroupHandle, p string) {
	g.Task(p).Delete("worktree", func() error { return nil })
}
`

// v100Module writes a temp module pinned to evident-output v1.0.0 holding
// pinnedDeleteSrc, and returns the module dir and the file path.
func v100Module(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	gomod := "module example.com/consumer\n\ngo 1.24\n\nrequire github.com/zachbornheimer/evident-output v1.0.0\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "prune.go")
	if err := os.WriteFile(path, []byte(pinnedDeleteSrc), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, path
}

// Every kind that reads code from a location lints it as that location's
// go.mod pin, so a v1.0.0 consumer never gets 1.1-only API-032 through any
// door: a file, a package of files, the directory, or conformance.
func TestReview_EveryKindResolvesTheGoModPin(t *testing.T) {
	bin := buildMCP(t)
	dir, path := v100Module(t)
	dirJSON, _ := json.Marshal(dir)
	pathJSON, _ := json.Marshal(path)
	srcJSON, _ := json.Marshal(pinnedDeleteSrc)
	calls := map[string]string{
		"go file":         `{"file":` + string(pathJSON) + `}`,
		"go inline":       `{"file":` + string(pathJSON) + `,"source":` + string(srcJSON) + `}`,
		"package":         `{"kind":"package","files":{"prune.go":` + string(pathJSON) + `}}`,
		"directory":       `{"kind":"directory","directory":` + string(dirJSON) + `}`,
		"conformance go":  `{"file":` + string(pathJSON) + `}`,
		"conformance dir": `{"kind":"directory","directory":` + string(dirJSON) + `}`,
	}
	for name, args := range calls {
		tool := "evident_output_review"
		if strings.HasPrefix(name, "conformance") {
			tool = "evident_output_conformance"
		}
		in := strings.Join([]string{
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"` + tool + `","arguments":` + args + `}}`,
		}, "\n") + "\n"
		out := runMCP(t, bin, in)
		if strings.Contains(out, "API-032") {
			t.Errorf("%s: a v1.0.0 pin must draw no API-032: %s", name, out)
		}
		if tool == "evident_output_review" && !strings.Contains(out, `"module_version":"v1.0.0"`) {
			t.Errorf("%s: must report the pin it linted as: %s", name, out)
		}
	}
}
