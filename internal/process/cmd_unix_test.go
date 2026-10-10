//go:build unix

package process_test

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/process"
)

func TestCmdExposesTheResolvedProgramAndItsArguments(t *testing.T) {
	cmd := process.NewCmd("/bin/sh", "-c", "true")
	if cmd.Path() != "/bin/sh" {
		t.Fatalf("Path() = %q, want /bin/sh", cmd.Path())
	}
	if got := strings.Join(cmd.Args(), " "); got != "/bin/sh -c true" {
		t.Fatalf("Args() = %q, want %q", got, "/bin/sh -c true")
	}
}

func TestCmdRunWiresStdoutAndStderrSeparately(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := process.NewCmd("/bin/sh", "-c", "echo out; echo err 1>&2")
	cmd.SetStdout(&stdout)
	cmd.SetStderr(&stderr)
	if cmd.Stdout() != &stdout || cmd.Stderr() != &stderr {
		t.Fatal("Stdout()/Stderr() did not return the writers just set")
	}
	if err := cmd.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stdout.String() != "out\n" || stderr.String() != "err\n" {
		t.Fatalf("stdout = %q, stderr = %q; want %q and %q", stdout.String(), stderr.String(), "out\n", "err\n")
	}
}

func TestCmdRunReturnsAnExitErrorForANonzeroExit(t *testing.T) {
	err := process.NewCmd("/bin/sh", "-c", "exit 3").Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 3 {
		t.Fatalf("Run() = %v, want an *exec.ExitError with code 3", err)
	}
}

func TestCmdRunsInTheDirectoryAndEnvironmentItIsGiven(t *testing.T) {
	dir := t.TempDir()
	cmd := process.NewCmd("/bin/sh", "-c", `pwd -P; echo "$EVO_CMD_PROBE"`)
	cmd.SetDir(dir)
	cmd.SetEnv([]string{"EVO_CMD_PROBE=value"})
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("CombinedOutput: %v\n%s", err, out)
	}
	wantDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := wantDir + "\nvalue\n"; string(out) != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

func TestCombinedOutputInterleavesStdoutAndStderr(t *testing.T) {
	out, err := process.NewCmd("/bin/sh", "-c", "echo out; echo err 1>&2").CombinedOutput()
	if err != nil {
		t.Fatalf("CombinedOutput: %v", err)
	}
	if got := string(out); got != "out\nerr\n" {
		t.Fatalf("CombinedOutput = %q, want %q", got, "out\nerr\n")
	}
}
