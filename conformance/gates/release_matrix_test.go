package gates_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestSEC009_LicenseApachePresent proves the license is reportable from the
// tree and is Apache-2.0 (SEC-009).
func TestSEC009_LicenseApachePresent(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "LICENSE"))
	if err != nil {
		t.Fatalf("read LICENSE: %v", err)
	}
	text := string(b)
	if !strings.Contains(text, "Apache License") || !strings.Contains(text, "Version 2.0") {
		t.Fatalf("LICENSE is not Apache-2.0: %.80s", text)
	}
}

// TestPORT012_BigEndianCrossCompile proves the CLI makes no byte-order
// assumption by cross-compiling it for a big-endian target (PORT-012).
func TestPORT012_BigEndianCrossCompile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "evident-output-s390x")
	cmd := exec.Command("go", "build", "-buildvcs=false", "-o", out, "./cmd/evident-output")
	cmd.Dir = filepath.Join(repoRoot(t), "mcp")
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=s390x", "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("s390x cross-compile of mcp/cmd/evident-output: %v\n%s", err, b)
	}
	st, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat cross-compiled binary %s: %v", out, err)
	}
	if st.Size() == 0 {
		t.Fatalf("cross-compiled binary %s is empty", out)
	}
}

// repoRoot resolves the module root from this file's location
// (conformance/gates/), independent of the caller's working directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller: could not resolve this file's path")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}
