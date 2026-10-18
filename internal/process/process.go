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

// Runner is the facade every evo.Exec spawn goes through: System() in
// ordinary use, a scripted testkit fake in tests. Run starts argv[0] (an
// absolute path) with argv[1:] in dir with exactly env, writes the child's
// output to stdout and stderr, and returns its exit status. A nonzero
// exitCode is not itself an error (Exec decides what a nonzero exit means);
// a non-nil error means the child produced no exit status at all: it never
// started, or ctx ended it. Run must honor ctx cancellation by killing the
// child rather than leaking it.
type Runner interface {
	Run(ctx context.Context, argv []string, dir string, env []string, stdout, stderr io.Writer) (exitCode int, err error)
}

// System returns the Runner that spawns real child processes.
func System() Runner { return systemRunner{} }

// systemRunner is Runner's real implementation: exec.CommandContext, which
// already kills the child on ctx cancellation (Cmd.Cancel defaults to
// Process.Kill since Go 1.20).
type systemRunner struct{}

// ErrNoArgv is Run's error for an empty argv: there is nothing to spawn.
var ErrNoArgv = errors.New("process: empty argv")

func (systemRunner) Run(ctx context.Context, argv []string, dir string, env []string, stdout, stderr io.Writer) (int, error) {
	if len(argv) == 0 {
		return 0, ErrNoArgv
	}
	c := exec.CommandContext(ctx, argv[0], argv[1:]...)
	c.Dir = dir
	c.Env = env
	c.Stdout = stdout
	c.Stderr = stderr
	isolateProcessGroup(c)
	c.WaitDelay = waitDelay
	err := c.Run()
	// A grandchild still holding the pipes after the direct child exited
	// makes Run report ErrWaitDelay; the child's own status is what counts.
	if c.ProcessState != nil && (err == nil || errors.Is(err, exec.ErrWaitDelay) || isExitError(err)) {
		return c.ProcessState.ExitCode(), nil
	}
	return 0, err
}

// waitDelay bounds how long Run waits for output pipes to drain after the
// child exits or is killed.
const waitDelay = time.Second

func isExitError(err error) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr)
}
