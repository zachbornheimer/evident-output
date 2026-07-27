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
  out := evo.NewWithOptions()
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
