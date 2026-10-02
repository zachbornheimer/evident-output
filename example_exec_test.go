package evo_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// exampleExecutable writes an executable stub named name in dir and returns
// its path: Exec resolves Path on disk before the scripted ProcessRunner
// is asked to run it.
func exampleExecutable(dir, name string) string {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		panic(err)
	}
	return path
}

// ExampleOutputs declares the Files and Trees an Exec produces; Run fails
// when one is missing after a zero exit.
func ExampleOutputs() {
	gen := evo.Exec{
		Path:    "python3",
		Args:    []string{"generate.py", "input.xlsx", "out.bin"},
		Outputs: evo.Outputs{evo.File{Path: "out.bin"}, evo.Tree{Path: "assets"}},
	}
	fmt.Println(gen.Path, len(gen.Args), len(gen.Outputs))
	// Output:
	// python3 3 2
}

// ExampleExec runs one subprocess from inside a Task's Define callback,
// through the same ProcessRunner facade a test replaces with
// testkit.ProcessRunner.
func ExampleExec() {
	dir, err := os.MkdirTemp("", "evo-example-exec")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = os.RemoveAll(dir) }()

	runner := testkit.NewProcessRunner()
	runner.Script(exampleExecutable(dir, "tool"), testkit.ScriptedProcess{ExitCode: 0})
	// The scripted runner never actually writes out.bin, so declare it
	// ahead of time the way a real generator's own subprocess would.
	if err := os.WriteFile(filepath.Join(dir, "out.bin"), []byte("generated"), 0o644); err != nil {
		fmt.Println(err)
		return
	}

	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Stdout: &buf, Stderr: io.Discard, Plain: true, StateDir: dir,
		ProcessRunner: runner,
	})
	task := out.Task("generate")
	task.Define(func(ctx context.Context) error {
		_, err := evo.Exec{
			Path:    "./tool",
			Args:    []string{"--out", "out.bin"},
			Dir:     dir,
			Outputs: evo.Outputs{evo.File{Path: "out.bin"}},
		}.Run(ctx)
		return err
	})
	_ = task.Wait()
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ generate
	// [changed] generate  ran ./tool
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
	runner.Script(exampleExecutable(dir, "lint"), testkit.ScriptedProcess{
		ExitCode: 1,
		Stdout:   []string{"file.go:10: unused variable", "file.go:22: missing return"},
	})

	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Stdout: &buf, Stderr: io.Discard, Plain: true, StateDir: dir,
		ProcessRunner: runner,
	})
	task := out.Task("lint")
	task.Define(func(ctx context.Context) error {
		result, err := evo.Exec{Path: "./lint", Dir: dir}.Run(ctx)
		if !errors.Is(err, evo.ErrExecNonzeroExit) {
			return err
		}
		findings := strings.Split(strings.TrimSpace(result.Stdout), "\n")
		return fmt.Errorf("%d lint finding(s) (exit %d)", len(findings), result.ExitCode)
	})
	_ = task.Wait()
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ◐ lint  file.go:10: unused variable
	// ◐ lint  file.go:22: missing return
	// ✗ lint  2 lint finding(s) (exit 1)
	//    └─ Last 2 lines:
	//       file.go:10: unused variable
	//       file.go:22: missing return
}

// ExampleProcessCommand is the resolved shape evo.Exec passes to a
// ProcessRunner — Path already resolved on PATH, Args exactly as declared,
// Stdout/Stderr wired to the Task's own capture.
func ExampleProcessCommand() {
	cmd := evo.ProcessCommand{
		Path:   "/usr/bin/python3",
		Args:   []string{"generate.py"},
		Stdout: io.Discard,
		Stderr: io.Discard,
	}
	fmt.Println(cmd.Path, len(cmd.Args))
	// Output:
	// /usr/bin/python3 1
}

// ExampleProcessOutcome is one spawned command's terminal, already-observed
// result — a nonzero ExitCode is not itself an error; evo.Exec decides
// what a nonzero exit means.
func ExampleProcessOutcome() {
	outcome := evo.ProcessOutcome{ExitCode: 0}
	fmt.Println(outcome.ExitCode == 0)
	// Output:
	// true
}

// ExampleProcessRunner is the facade every evo.Exec spawn goes through
// instead of exec.Cmd/os/exec directly — testkit.ProcessRunner satisfies
// it deterministically for tests; production uses the real spawner.
func ExampleProcessRunner() {
	runner := testkit.NewProcessRunner()
	runner.Script("/usr/bin/tool", testkit.ScriptedProcess{ExitCode: 0})

	outcome, err := runner.Run(context.Background(), evo.ProcessCommand{Path: "/usr/bin/tool"})
	fmt.Println(err == nil, outcome.ExitCode)
	// Output:
	// true 0
}
