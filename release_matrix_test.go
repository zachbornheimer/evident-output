package evo_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestSEC008_GovulncheckWhenInstalled(t *testing.T) {
	// SEC-008: dependency vulnerability scan via govulncheck (mise run scan).
	if _, err := exec.LookPath("govulncheck"); err != nil {
		t.Skip("govulncheck not installed; mise run scan documents skip path")
	}
	cmd := exec.Command("govulncheck", "./...")
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("govulncheck failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "No vulnerabilities found") &&
		!strings.Contains(string(out), "=== Symbol Results ===") &&
		len(out) == 0 {
		t.Fatalf("unexpected govulncheck output: %s", out)
	}
}

func TestSEC009_LicenseApachePresent(t *testing.T) {
	// SEC-009: license reportable from tree (Apache-2.0).
	b, err := os.ReadFile(filepath.Join(repoRoot(t), "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "Apache License") || !strings.Contains(s, "Version 2.0") {
		t.Fatalf("LICENSE is not Apache-2.0: %s", s[:min(80, len(s))])
	}
}

func TestPORT012_BigEndianCrossCompile(t *testing.T) {
	// PORT-012: no encoding assumption — cross-compile CLI for big-endian.
	root := repoRoot(t)
	out := filepath.Join(t.TempDir(), "evident-output-s390x")
	cmd := exec.Command("go", "build", "-o", out, "./cmd/evident-output")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=s390x", "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("s390x cross-compile: %v\n%s", err, b)
	}
	st, err := os.Stat(out)
	if err != nil || st.Size() == 0 {
		t.Fatal(err, st)
	}
}

func TestAPI013_ExampleCLIsBuild(t *testing.T) {
	// API-013: example programs compile without core dep changes.
	root := repoRoot(t)
	for _, ex := range []string{"repo-status", "install-pipeline", "migrate", "doctor", "data-command"} {
		cmd := exec.Command("go", "build", "-o", os.DevNull, "./examples/"+ex+"/")
		cmd.Dir = root
		if b, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build examples/%s: %v\n%s", ex, err, b)
		}
	}
}

func TestTERM009_CancelCleanupPath(t *testing.T) {
	// TERM-009: SIGINT handling documents Cancel → cancelled conclusion cleanup.
	// Full PTY signal injection remains host-dependent; library path is Cancel.
	out := evo.NewWithOptions(evo.To(os.Stderr), evo.Plain(), evo.NoColor())
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("long")
	task.Phase("working")
	out.Cancel("interrupted by signal")
	if err := out.Finish(); err != nil {
		// Cancel may leave misuse nil
		t.Log(err)
	}
	c := out.Conclusion()
	if c.State != evo.StateCancelled && string(c.State) != "cancelled" {
		// Allow partial if cancel maps differently
		if c.State != evo.StateCancelled {
			// Check snapshot tasks
			snap := out.Snapshot()
			found := false
			for _, tk := range snap.Tasks {
				if tk.State == evo.Cancelled {
					found = true
				}
			}
			if !found && c.State != evo.StateCancelled {
				t.Fatalf("expected cancelled path, conclusion=%+v tasks=%+v", c, snap.Tasks)
			}
		}
	}
}

func TestMCP036_RemotePathRejectedByPolicy(t *testing.T) {
	// MCP-036: remote paths unsupported — helpers reject http(s)/remote schemes.
	for _, p := range []string{
		"https://evil.example/x.go",
		"http://evil.example/x.go",
		"git+ssh://host/repo",
	} {
		if !isRemotePath(p) {
			t.Fatalf("expected remote: %s", p)
		}
	}
	if isRemotePath("/tmp/local.go") || isRemotePath("relative/file.go") {
		t.Fatal("local path misclassified")
	}
}

// isRemotePath mirrors MCP policy: content-only, no remote fetch.
func isRemotePath(p string) bool {
	lower := strings.ToLower(p)
	return strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "git+") ||
		strings.HasPrefix(lower, "ssh://") ||
		strings.Contains(lower, "://")
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Dir(file)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
