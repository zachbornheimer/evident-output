package evo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/zachbornheimer/evident-output/testkit"
)

// ExampleExecSpec declares one managed-state subprocess invocation —
// constructing it performs no I/O; passing it to Exec is what skips
// spawning when a prior record proves the operation current, or runs the
// child and verifies its declared Outputs afterward.
func ExampleExecSpec() {
	spec := ExecSpec{
		executable: "python3",
		args:       []string{"generate.py", "input.xlsx", "out.bin"},
		basis:      []fingerprint{fSPath("input.xlsx")},
		Outputs:    []string{"out.bin"},
	}
	fmt.Println(spec.executable, len(spec.args))
	// Output:
	// python3 3
}

// ExampleExec reconciles one managed-state subprocess invocation from
// inside a Task's Define callback, through the same ProcessRunner facade a
// test replaces with testkit.ProcessRunner.
func ExampleExec() {
	dir, err := os.MkdirTemp("", "evo-example-exec")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()
	outPath := filepath.Join(dir, "out.bin")

	runner := testkit.NewProcessRunner()
	runner.Script("/usr/bin/tool", testkit.ScriptedProcess{ExitCode: 0})
	// The scripted runner never actually writes outPath, so declare it
	// ahead of time the way a real generator's own subprocess would.
	if err := os.WriteFile(outPath, []byte("generated"), 0o644); err != nil {
		fmt.Println(err)
		return
	}

	var buf bytes.Buffer
	out := initRun(Config{
		isolated: true, stdout: &buf, stderr: io.Discard, plain: true, stateDir: dir,
		processRunner: runner,
	})
	task := out.Task("generate")
	task.Define(func(ctx context.Context) error {
		_, err := Exec(ctx, ExecSpec{
			executable: "/usr/bin/tool",
			args:       []string{"--out", outPath},
			Outputs:    []string{outPath},
		})
		return err
	})
	_ = task.wait()
	_ = out.finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ generate
	// [changed] generate  ran /usr/bin/tool
	//
	// [changed]
}

// ExampleExecResult shows a caller parsing a completed Exec attempt's
// captured stdout into a structured Problem, instead of rebuilding
// subprocess capture around Evo — Exec still owns spawning, capture,
// liveness, and cancellation; the caller only inspects the returned
// ExecResult (spec §8.4/ZYS-850). A nonzero exit wraps ErrExecNonzeroExit
// but still returns the captured ExecResult, so a linter's own findings can
// become a structured Fail instead of a flattened text blob.
func ExampleExecResult() {
	runner := testkit.NewProcessRunner()
	runner.Script("/usr/bin/lint", testkit.ScriptedProcess{
		ExitCode: 1,
		Stdout:   []string{"file.go:10: unused variable", "file.go:22: missing return"},
	})

	// go/doc Example functions take no *testing.T (they are not run via
	// t.Run), so t.TempDir is unavailable here — os.MkdirTemp + a deferred
	// RemoveAll is this file's own established substitute (see ExampleExec
	// above): without an explicit StateDir, Exec's manifest lock would
	// resolve to this workspace's one shared cache-dir path and collide
	// with any other Exec call running against it.
	dir, err := os.MkdirTemp("", "evo-example-exec-result")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()

	var buf bytes.Buffer
	out := initRun(Config{
		isolated: true, stdout: &buf, stderr: io.Discard, plain: true, stateDir: dir,
		processRunner: runner,
	})
	task := out.Task("lint")
	task.Define(func(ctx context.Context) error {
		result, err := Exec(ctx, ExecSpec{executable: "/usr/bin/lint"})
		if !errors.Is(err, ErrExecNonzeroExit) {
			return err
		}
		findings := strings.Split(strings.TrimSpace(result.Stdout), "\n")
		return fmt.Errorf("%d lint finding(s) (exit %d)", len(findings), result.ExitCode)
	})
	_ = task.wait()
	_ = out.finish()
	fmt.Print(buf.String())
	// Output:
	// ◐ lint  file.go:10: unused variable
	// ◐ lint  file.go:22: missing return
	// ✗ lint  2 lint finding(s) (exit 1)
	//    └─ Last 2 lines:
	//       file.go:10: unused variable
	//       file.go:22: missing return
	//
	// [failed]
}

// ExampleProcessRunner is the facade every Exec spawn goes through
// instead of exec.Cmd/os/exec directly — testkit.ProcessRunner satisfies
// it deterministically for tests; production uses the real spawner.
func ExampleProcessRunner() {
	runner := testkit.NewProcessRunner()
	runner.Script("/usr/bin/tool", testkit.ScriptedProcess{ExitCode: 0})

	outcome, err := runner.Run(context.Background(), processCommand{Path: "/usr/bin/tool"})
	fmt.Println(err == nil, outcome.ExitCode)
	// Output:
	// true 0
}

// ExampleRunner installs a ProcessRunner other than the real spawner — the
// seam every Exec test in this repo uses to replace the OS process with
// a deterministic testkit fake.
func ExampleConfig_runner() {
	runner := testkit.NewProcessRunner()
	runner.Script("/usr/bin/tool", testkit.ScriptedProcess{ExitCode: 0})

	var buf bytes.Buffer
	out := initRun(Config{
		isolated: true, stdout: &buf, stderr: io.Discard, plain: true,
		processRunner: runner,
	})
	out.Task("demo").Define(func(context.Context) error { return nil })
	_ = out.finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ demo
}
