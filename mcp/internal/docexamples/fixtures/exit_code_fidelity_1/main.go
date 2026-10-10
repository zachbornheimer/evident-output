// Package main compiles mcp/docs/exit-code-fidelity.md's capture-then-
// override pattern verbatim (see TestDocFencesMatchFixtures in the parent
// package). This file is a documentation fixture only — go build proves it
// type-checks against the shipped API; nothing in this package is ever
// executed, so the exec.Command call it demonstrates is the doc's own
// subject matter (propagating a child process's real exit code), not
// production process-invocation logic that would need an injectable
// facade.
package main

// The doc fence below assumes these imports are already in scope (the
// prose above it says so); declared as fixture scaffolding here so the
// marked region matches the doc byte-for-byte. fmt is additionally
// needed by the run helper below, not by the fence itself.
import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"

	evo "github.com/zachbornheimer/evident-output"
)

// docexamples:snippet start
func main() {
	ctx := context.Background()
	out := evo.Init(evo.Config{Title: "runner", Isolated: true})

	var childErr *exec.ExitError
	code := out.Run(ctx, func(ctx context.Context) error {
		cmd := out.Task("build")
		err := run(cmd, exec.Command("make", "build")) // wires cmd.Stdout/Stderr into evo evidence
		errors.As(err, &childErr)
		return err
	}).ExitCode()
	if childErr != nil {
		code = childErr.ExitCode()
	}
	os.Exit(code)
}

// docexamples:snippet end

// run is the reader's own helper implied by the doc's "run(cmd, ...)" call:
// it wires cmd.Stdout/Stderr through task.Writer() (see the "Evidence"
// fence in teaching-ladder.md) and runs cmd, propagating cmd.Run's error —
// including *exec.ExitError when the child exits non-zero, wrapped so
// errors.As above can still unwrap it. The fixture supplies a trivial
// stand-in purely so the doc snippet above type-checks; it is never
// invoked outside this never-run fixture binary.
func run(task *evo.TaskHandle, cmd *exec.Cmd) error {
	cmd.Stdout = task.Writer()
	cmd.Stderr = task.Writer()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run %s: %w", cmd.Path, err)
	}
	return nil
}
