package evaltask

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
)

// Command is one process to run.
type Command struct {
	Dir  string
	Env  []string // extra KEY=VALUE entries on top of the inherited environment
	Name string
	Args []string
}

// CommandResult is what a finished process produced.
type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Runner is the process boundary of the grader; tests substitute a fake.
// A non-zero exit is a result, not an error: err means the process could
// not be run at all.
type Runner interface {
	Run(ctx context.Context, cmd Command) (CommandResult, error)
}

// ExecRunner runs real processes.
type ExecRunner struct{}

// Run executes cmd with the inherited environment plus cmd.Env.
func (ExecRunner) Run(ctx context.Context, cmd Command) (CommandResult, error) {
	proc := exec.CommandContext(ctx, cmd.Name, cmd.Args...)
	proc.Dir = cmd.Dir
	proc.Env = append(os.Environ(), cmd.Env...)
	var stdout, stderr bytes.Buffer
	proc.Stdout = &stdout
	proc.Stderr = &stderr
	err := proc.Run()
	result := CommandResult{Stdout: stdout.String(), Stderr: stderr.String()}
	var exit *exec.ExitError
	switch {
	case err == nil:
		return result, nil
	case errors.As(err, &exit):
		result.ExitCode = exit.ExitCode()
		return result, nil
	default:
		return result, fmt.Errorf("run %s %v in %s: %w", cmd.Name, cmd.Args, cmd.Dir, err)
	}
}
