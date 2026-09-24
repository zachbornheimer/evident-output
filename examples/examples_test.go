package examples_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Non-TTY smoke: every example package builds and runs under NO_COLOR.
func TestExamples_NonTTYSmoke(t *testing.T) {
	root := findRepoRoot(t)
	type spec struct {
		name string
		args []string
		// allowExit lists acceptable process exit codes (0 always implied if empty means 0 only).
		allowExit []int
	}
	specs := []spec{
		{name: "print"},
		{name: "verbose"},
		{name: "verbose", args: []string{"--verbose"}},
		{name: "repo-status", args: []string{"--fast"}, allowExit: []int{0, 1}},
		{name: "install-pipeline", args: []string{"--fast"}},
		{name: "install-pipeline", args: []string{"--fast", "--fail-tests"}, allowExit: []int{0, 1, 2}},
		{name: "migrate"},
		{name: "migrate", args: []string{"--apply", "--fail"}, allowExit: []int{0, 1, 2}},
		{name: "doctor", args: []string{"--fast"}, allowExit: []int{0, 1, 2}},
		{name: "doctor", args: []string{"--fast", "--verbose"}, allowExit: []int{0, 1, 2}},
		{name: "data-command"},
		{name: "scope-plugin"},
		{name: "live-progress", args: []string{"--fast"}},
		{name: "debug-history", args: []string{"--fast"}},
		{name: "debug-pane", args: []string{"--fast"}},
		{name: "debug-pane", args: []string{"--fast", "--fail"}, allowExit: []int{0, 1}},
		{name: "terminal-driver", args: []string{"--fast", "--frames"}},
		{name: "launch-agent-http"},
		{name: "launch-agent-http", args: []string{"--format", "json"}},
	}
	for _, s := range specs {
		label := s.name
		if len(s.args) > 0 {
			label = s.name + "/" + joinArgs(s.args)
		}
		t.Run(label, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(root, "examples", s.name)
			bin := filepath.Join(t.TempDir(), s.name)
			build := exec.Command("go", "build", "-o", bin, ".")
			build.Dir = dir
			if out, err := build.CombinedOutput(); err != nil {
				t.Fatalf("build: %v\n%s", err, out)
			}
			cmd := exec.Command(bin, s.args...)
			cmd.Env = append(os.Environ(), "NO_COLOR=1")
			out, err := cmd.CombinedOutput()
			code := 0
			if err != nil {
				ee, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatalf("run: %v\n%s", err, out)
				}
				code = ee.ExitCode()
			}
			ok := code == 0
			for _, a := range s.allowExit {
				if code == a {
					ok = true
				}
			}
			if !ok {
				t.Fatalf("exit %d not allowed; allow %v\n%s", code, s.allowExit, out)
			}
		})
	}
}

// newCommand is the sole exec.Command call site examples_test.go uses,
// injectable so a test can swap in a fake process launcher instead of
// spawning a real one — the facade buildExampleBinary/runExampleBinary
// previously lacked (ZYS-823 review: a wrapper with no injection point is
// not a facade). TestExamples_NonTTYSmoke keeps its own direct
// exec.Command calls: that test's whole point is exercising the real `go
// build`/binary lifecycle across many specs, not a boundary anything mocks.
var newCommand = exec.Command

// buildExampleBinary and runExampleBinary name the two subprocess call
// sites TestDataCommand_StdoutCarriesOnlyResultPayload needs: this test's
// whole point is observing a *real compiled binary's* independent OS
// stdout/stderr streams (spec §32.1's separation) — the process launch
// itself is real, but goes through newCommand so it stays swappable.
func buildExampleBinary(bin, dir string) *exec.Cmd {
	cmd := newCommand("go", "build", "-o", bin, ".")
	cmd.Dir = dir
	return cmd
}

func runExampleBinary(bin string, args ...string) *exec.Cmd {
	return newCommand(bin, args...)
}

// TestDataCommand_StdoutCarriesOnlyResultPayload is ZYS-823 gap 5: run the
// real data-command binary (evo.FormatData, spec §32.1's stdout-is-the-
// payload contract) with stdout and stderr captured on separate pipes —
// the smoke test above merges them with CombinedOutput and so cannot prove
// this. stdout must decode as exactly one BuildResult JSON object and
// nothing else (no evo task rows spliced in); the human ledger must appear
// only on stderr.
func TestDataCommand_StdoutCarriesOnlyResultPayload(t *testing.T) {
	root := findRepoRoot(t)
	dir := filepath.Join(root, "examples", "data-command")
	bin := filepath.Join(t.TempDir(), "data-command")
	build := buildExampleBinary(bin, dir)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	cmd := runExampleBinary(bin)
	cmd.Env = append(os.Environ(), "NO_COLOR=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}

	var payload struct {
		Artifact string `json:"artifact"`
		Packages int    `json:"packages"`
		Duration string `json:"duration"`
	}
	raw := append([]byte(nil), stdout.Bytes()...)
	dec := json.NewDecoder(&stdout)
	if err := dec.Decode(&payload); err != nil {
		t.Fatalf("stdout must decode as one BuildResult JSON object: %v\nstdout:\n%s", err, raw)
	}
	if dec.More() {
		t.Fatalf("stdout must carry exactly one JSON document, found trailing content:\n%s", raw)
	}
	if payload.Artifact == "" {
		t.Fatalf("decoded payload is missing its domain fields: %+v", payload)
	}

	if strings.Contains(string(raw), "compile") || strings.Contains(string(raw), "✓") {
		t.Fatalf("evo task presentation leaked into stdout:\n%s", raw)
	}
	if stderr.Len() == 0 {
		t.Fatalf("human presentation vanished; it must still reach stderr")
	}
}

func joinArgs(args []string) string {
	var out strings.Builder
	for i, a := range args {
		if i > 0 {
			out.WriteString("_")
		}
		out.WriteString(a)
	}
	return out.String()
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for range 6 {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			// When tests run as package examples_test from examples/, go.mod is parent.
			if filepath.Base(dir) == "examples" {
				return filepath.Dir(dir)
			}
			if _, err := os.Stat(filepath.Join(dir, "examples")); err == nil {
				return dir
			}
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("repo root not found")
	return ""
}
