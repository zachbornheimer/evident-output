package evo_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// ExampleExecSpec declares one managed-state subprocess invocation —
// constructing it performs no I/O; passing it to Exec is what skips
// spawning when a prior record proves the operation current, or runs the
// child and verifies its declared Outputs afterward.
func ExampleExecSpec() {
	spec := evo.ExecSpec{
		Executable: "python3",
		Args:       []string{"generate.py", "input.xlsx", "out.bin"},
		Basis:      []evo.Fingerprint{evo.FSPath("input.xlsx")},
		Outputs:    []string{"out.bin"},
	}
	fmt.Println(spec.Executable, len(spec.Args))
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
	out := evo.Init(evo.Config{
		Isolated: true, Stdout: &buf, Stderr: io.Discard, Plain: true, StateDir: dir,
		Options: []evo.Option{evo.Runner(runner)},
	})
	task := out.Task("generate")
	task.Define(func(ctx context.Context) error {
		return evo.Exec(ctx, evo.ExecSpec{
			Executable: "/usr/bin/tool",
			Args:       []string{"--out", outPath},
			Outputs:    []string{outPath},
		})
	})
	_ = task.Wait()
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ generate
	// [changed] generate  ran /usr/bin/tool
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

// ExampleRunner installs a ProcessRunner other than the real spawner — the
// seam every evo.Exec test in this repo uses to replace the OS process with
// a deterministic testkit fake.
func ExampleRunner() {
	runner := testkit.NewProcessRunner()
	runner.Script("/usr/bin/tool", testkit.ScriptedProcess{ExitCode: 0})

	var buf bytes.Buffer
	out := evo.Init(evo.Config{
		Isolated: true, Stdout: &buf, Stderr: io.Discard, Plain: true,
		Options: []evo.Option{evo.Runner(runner)},
	})
	out.Task("demo").Done()
	_ = out.Finish()
	fmt.Print(buf.String())
	// Output:
	// ✓ demo
}
