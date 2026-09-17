package testkit

import (
	"context"
	"fmt"
	"sync"

	evo "github.com/zachbornheimer/evident-output"
)

// ScriptedProcess is one canned evo.Exec response: predetermined output
// lines, an exit code, and — for a cancellation test — a Gate that must
// close (or the run's ctx must cancel first) before Run returns.
type ScriptedProcess struct {
	Stdout   []string
	Stderr   []string
	ExitCode int
	// SpawnErr, when set, makes Run report a spawn-level failure (the
	// executable could not run at all) instead of any exit code.
	SpawnErr error
	// Gate, when non-nil, must be closed before Run proceeds past it — the
	// deterministic way to prove ctx cancellation kills a still-running
	// child (spec §8.4).
	Gate chan struct{}
}

// ProcessRunner is a deterministic evo.ProcessRunner fake: each call is
// matched to a scripted response by resolved executable path, so a test can
// prove Exec's skip protocol, capture, and cancellation behavior without
// spawning a real process.
type ProcessRunner struct {
	mu      sync.Mutex
	scripts map[string]ScriptedProcess
	calls   []evo.ProcessCommand
}

// NewProcessRunner returns an empty ProcessRunner — script each executable
// path the test under it will invoke via Script before running.
func NewProcessRunner() *ProcessRunner {
	return &ProcessRunner{scripts: map[string]ScriptedProcess{}}
}

// Script registers resp as the response for a spawn whose resolved
// executable path is executable.
func (r *ProcessRunner) Script(executable string, resp ScriptedProcess) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.scripts[executable] = resp
}

// Calls returns every ProcessCommand Run has observed so far, in order.
func (r *ProcessRunner) Calls() []evo.ProcessCommand {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]evo.ProcessCommand(nil), r.calls...)
}

// Run implements evo.ProcessRunner.
func (r *ProcessRunner) Run(ctx context.Context, cmd evo.ProcessCommand) (evo.ProcessOutcome, error) {
	r.mu.Lock()
	resp, ok := r.scripts[cmd.Path]
	r.calls = append(r.calls, cmd)
	r.mu.Unlock()
	if !ok {
		return evo.ProcessOutcome{}, fmt.Errorf("testkit: no scripted process for %q", cmd.Path)
	}
	if resp.Gate != nil {
		select {
		case <-resp.Gate:
		case <-ctx.Done():
			return evo.ProcessOutcome{}, ctx.Err()
		}
	}
	if ctx.Err() != nil {
		return evo.ProcessOutcome{}, ctx.Err()
	}
	if resp.SpawnErr != nil {
		return evo.ProcessOutcome{}, resp.SpawnErr
	}
	for _, line := range resp.Stdout {
		_, _ = fmt.Fprintln(cmd.Stdout, line)
	}
	for _, line := range resp.Stderr {
		_, _ = fmt.Fprintln(cmd.Stderr, line)
	}
	return evo.ProcessOutcome{ExitCode: resp.ExitCode}, nil
}
