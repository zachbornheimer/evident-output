package testkit

import (
	"context"
	"fmt"
	"io"
	"slices"
	"sync"
)

// ScriptedProcess is one canned Exec response: predetermined output
// lines, an exit code, and — for a cancellation test — a Gate that must
// close (or the run's ctx must cancel first) before Run returns.
type ScriptedProcess struct {
	Stdout   []string
	Stderr   []string
	ExitCode int
	SpawnErr error
	Gate     chan struct{}
}

// ProcessCall is one command ProcessRunner was asked to spawn.
type ProcessCall struct {
	Argv []string
	Dir  string
	Env  []string
}

// ProcessRunner is a deterministic evo.Process fake, scripted per argv[0].
type ProcessRunner struct {
	mu      sync.Mutex
	scripts map[string]ScriptedProcess
	calls   []ProcessCall
}

func NewProcessRunner() *ProcessRunner {
	return &ProcessRunner{scripts: map[string]ScriptedProcess{}}
}

// Script sets the response to a spawn whose argv[0] is executable.
func (r *ProcessRunner) Script(executable string, resp ScriptedProcess) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scripts[executable] = resp
}

// Calls is every spawn Run was asked for, in order.
func (r *ProcessRunner) Calls() []ProcessCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.calls)
}

// Run plays the script for argv[0].
func (r *ProcessRunner) Run(ctx context.Context, argv []string, dir string, env []string, stdout, stderr io.Writer) (int, error) {
	executable := ""
	if len(argv) > 0 {
		executable = argv[0]
	}
	r.mu.Lock()
	resp, ok := r.scripts[executable]
	r.calls = append(r.calls, ProcessCall{Argv: slices.Clone(argv), Dir: dir, Env: slices.Clone(env)})
	r.mu.Unlock()
	if !ok {
		return 0, fmt.Errorf("testkit: no scripted process for %q", executable)
	}
	if resp.Gate != nil {
		select {
		case <-resp.Gate:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if resp.SpawnErr != nil {
		return 0, resp.SpawnErr
	}
	for _, line := range resp.Stdout {
		_, _ = fmt.Fprintln(stdout, line)
	}
	for _, line := range resp.Stderr {
		_, _ = fmt.Fprintln(stderr, line)
	}
	return resp.ExitCode, nil
}
