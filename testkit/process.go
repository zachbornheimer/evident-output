package testkit

import (
	"context"
	"fmt"
	"sync"

	"github.com/zachbornheimer/evident-output/internal/process"
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

// ProcessRunner is a deterministic process.Runner fake.
type ProcessRunner struct {
	mu      sync.Mutex
	scripts map[string]ScriptedProcess
	calls   []process.Command
}

func NewProcessRunner() *ProcessRunner {
	return &ProcessRunner{scripts: map[string]ScriptedProcess{}}
}

func (r *ProcessRunner) Script(executable string, resp ScriptedProcess) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scripts[executable] = resp
}

func (r *ProcessRunner) Calls() []process.Command {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]process.Command(nil), r.calls...)
}

func (r *ProcessRunner) Run(ctx context.Context, cmd process.Command) (process.Outcome, error) {
	r.mu.Lock()
	resp, ok := r.scripts[cmd.Path]
	r.calls = append(r.calls, cmd)
	r.mu.Unlock()
	if !ok {
		return process.Outcome{}, fmt.Errorf("testkit: no scripted process for %q", cmd.Path)
	}
	if resp.Gate != nil {
		select {
		case <-resp.Gate:
		case <-ctx.Done():
			return process.Outcome{}, ctx.Err()
		}
	}
	if ctx.Err() != nil {
		return process.Outcome{}, ctx.Err()
	}
	if resp.SpawnErr != nil {
		return process.Outcome{}, resp.SpawnErr
	}
	for _, line := range resp.Stdout {
		_, _ = fmt.Fprintln(cmd.Stdout, line)
	}
	for _, line := range resp.Stderr {
		_, _ = fmt.Fprintln(cmd.Stderr, line)
	}
	return process.Outcome{ExitCode: resp.ExitCode}, nil
}
