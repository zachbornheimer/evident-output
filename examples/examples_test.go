package examples_test

import (
	"os"
	"os/exec"
	"path/filepath"
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
	}
	for _, s := range specs {
		s := s
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

func joinArgs(args []string) string {
	out := ""
	for i, a := range args {
		if i > 0 {
			out += "_"
		}
		out += a
	}
	return out
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
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
