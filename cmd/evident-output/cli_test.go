package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLI_ReviewPreviewExplainParity(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "bad.go")
	code := `package p
import (
  "fmt"
  evo "github.com/zachbornheimer/evident-output"
)
func f() {
  out := evo.Init(evo.Config{})
  t := out.Task("x")
  t.Start()
  fmt.Printf("hi")
}
`
	if err := os.WriteFile(src, []byte(code), 0o600); err != nil {
		t.Fatal(err)
	}

	// review
	out, err := exec.Command(bin, "review", src).CombinedOutput()
	// exit 1 when findings require recheck is OK
	if !strings.Contains(string(out), "API-006") && !strings.Contains(string(out), "STREAM-003") {
		t.Fatalf("review: %v\n%s", err, out)
	}
	var rev map[string]any
	// stdout-only JSON: CombinedOutput mixes stderr; re-run capturing stdout
	cmd := exec.Command(bin, "review", src)
	stdout, _ := cmd.Output()
	if err := json.Unmarshal(stdout, &rev); err != nil {
		// may have failed exit; still try unmarshal stdout
		if len(stdout) == 0 {
			t.Fatalf("no stdout from review: %v", err)
		}
	}
	if rev != nil {
		if _, ok := rev["findings"]; !ok {
			t.Fatalf("review json missing findings: %v", rev)
		}
	}

	// preview
	pout, err := exec.Command(bin, "preview", "--state=blocked", "--subject=demo").Output()
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !strings.Contains(string(pout), "profiles") {
		t.Fatalf("preview: %s", pout)
	}

	// explain
	eout, err := exec.Command(bin, "explain", "API-006").Output()
	if err != nil {
		t.Fatalf("explain: %v %s", err, eout)
	}
	if !strings.Contains(string(eout), "API-006") {
		t.Fatalf("explain: %s", eout)
	}
}

func TestCLI_ReviewDirectoryMergesFiles(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	a := `package a
import evo "github.com/zachbornheimer/evident-output"
func f(out *evo.Output) { out.Plan("a") }
`
	b := `package b
import evo "github.com/zachbornheimer/evident-output"
func f(out *evo.Output) { out.Changes("b") }
`
	if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte(a), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte(b), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "review", dir)
	stdout, _ := cmd.Output()
	if !strings.Contains(string(stdout), "API-032") {
		t.Fatalf("review dir missing API-032: %s", stdout)
	}
	if !strings.Contains(string(stdout), "a.go") || !strings.Contains(string(stdout), "b.go") {
		t.Fatalf("review dir must merge both files: %s", stdout)
	}
}

func TestCLI_AdoptPrintsPagedJSON(t *testing.T) {
	bin := buildCLI(t)
	mixed := filepath.Join("..", "..", "internal", "agent", "adopt", "testdata", "mixed")
	out, err := exec.Command(bin, "adopt", mixed).Output()
	if err != nil {
		t.Fatalf("adopt: %v\n%s", err, out)
	}
	var page map[string]any
	if err := json.Unmarshal(out, &page); err != nil {
		t.Fatalf("adopt json: %v\n%s", err, out)
	}
	for _, key := range []string{"findings", "rung", "remaining", "next_cursor", "next_action"} {
		if _, ok := page[key]; !ok {
			t.Errorf("adopt json missing %s: %s", key, out)
		}
	}
	if page["next_action"] == "clean" {
		t.Fatalf("mixed fixture is not clean: %s", out)
	}
	raw, _ := json.Marshal(page["findings"])
	if !strings.Contains(string(raw), "log.Fatal") && !strings.Contains(string(raw), "fmt.Printf") {
		t.Fatalf("adopt findings missing mixed patterns: %s", out)
	}
}

func buildCLI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "evident-output")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}
