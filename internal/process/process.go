// Package process is the one place Evo spawns child processes and reads this
// process's own environment, arguments and signals.
package process

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"time"
)

// Command is one resolved external command Exec is about to spawn: the
// OS-facing argv/dir/env after ExecSpec's path/workspace resolution
// (spec §8.4), immutable once built. Stdout/Stderr are the evidence writers
// Exec wires so a Runner never owns capture/redaction policy itself.
type Command struct {
	Path   string
	Args   []string
	Dir    string
	Env    []string
	Stdout io.Writer
	Stderr io.Writer
}

// Outcome is one spawned command's terminal, already-observed result — a
// nonzero ExitCode is not itself an error (Exec decides what a nonzero exit
// means); a Runner.Run error means the process never produced a terminal
// exit status at all (spawn failure or ctx cancellation).
type Outcome struct {
	ExitCode int
}

// Runner is the facade every evo.Exec spawn goes through: System() in
// ordinary use, a scripted testkit fake in tests. Run must honor ctx
// cancellation by killing the child rather than leaking it.
type Runner interface {
	Run(ctx context.Context, cmd Command) (Outcome, error)
}

// System returns the Runner that spawns real child processes.
func System() Runner { return systemRunner{} }

// systemRunner is Runner's real implementation: exec.CommandContext, which
// already kills the child on ctx cancellation (Cmd.Cancel defaults to
// Process.Kill since Go 1.20).
type systemRunner struct{}

func (systemRunner) Run(ctx context.Context, cmd Command) (Outcome, error) {
	c := exec.CommandContext(ctx, cmd.Path, cmd.Args...)
	c.Dir = cmd.Dir
	c.Env = cmd.Env
	c.Stdout = cmd.Stdout
	c.Stderr = cmd.Stderr
	isolateProcessGroup(c)
	c.WaitDelay = waitDelay
	err := c.Run()
	// A grandchild still holding the pipes after the direct child exited
	// makes Run report ErrWaitDelay; the child's own status is what counts.
	if c.ProcessState != nil && (err == nil || errors.Is(err, exec.ErrWaitDelay) || isExitError(err)) {
		return Outcome{ExitCode: c.ProcessState.ExitCode()}, nil
	}
	return Outcome{}, err
}

// waitDelay bounds how long Run waits for output pipes to drain after the
// child exits or is killed.
const waitDelay = time.Second

func isExitError(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}
