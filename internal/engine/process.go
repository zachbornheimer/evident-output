package engine

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
)

// ProcessCommand is one resolved external command Exec is about to spawn:
// the OS-facing argv/dir/env after ExecSpec's path/workspace resolution
// (spec §8.4), immutable once built. Stdout/Stderr are the evidence writers
// Exec wires so a ProcessRunner never owns capture/redaction policy itself.
type ProcessCommand struct {
	Path   string
	Args   []string
	Dir    string
	Env    []string
	Stdout io.Writer
	Stderr io.Writer
}

// ProcessOutcome is one spawned command's terminal, already-observed
// result — a nonzero ExitCode is not itself an error (Exec decides what a
// nonzero exit means); a ProcessRunner.Run error means the process never
// produced a terminal exit status at all (spawn failure or ctx cancellation).
type ProcessOutcome struct {
	ExitCode int
}

// ProcessRunner is the facade every evo.Exec spawn goes through instead of
// exec.Cmd/os/exec directly (facade rule): the real osProcessRunner in
// ordinary use, a scripted testkit fake in tests. Run must honor ctx
// cancellation by killing the child rather than leaking it.
type ProcessRunner interface {
	Run(ctx context.Context, cmd ProcessCommand) (ProcessOutcome, error)
}

// osProcessRunner is ProcessRunner's real implementation: exec.CommandContext,
// which already kills the child on ctx cancellation (Cmd.Cancel defaults to
// Process.Kill since Go 1.20).
type osProcessRunner struct{}

func (osProcessRunner) Run(ctx context.Context, cmd ProcessCommand) (ProcessOutcome, error) {
	c := exec.CommandContext(ctx, cmd.Path, cmd.Args...)
	c.Dir = cmd.Dir
	c.Env = cmd.Env
	c.Stdout = cmd.Stdout
	c.Stderr = cmd.Stderr
	err := c.Run()
	if err == nil {
		return ProcessOutcome{ExitCode: 0}, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return ProcessOutcome{ExitCode: exitErr.ExitCode()}, nil
	}
	return ProcessOutcome{}, err
}

// processEnviron is the facade mergedExecEnv reads the process's own
// environment through, instead of os.Environ directly (facade rule).
var processEnviron = os.Environ

// mergedExecEnv merges ExecSpec.Env onto the process's own environment,
// later entries overriding a repeated name (spec §8.4) — returned sorted so
// the child's own env order is deterministic across runs.
func mergedExecEnv(overrides map[string]string) []string {
	base := processEnviron()
	if len(overrides) == 0 {
		return base
	}
	merged := make(map[string]string, len(base)+len(overrides))
	for _, kv := range base {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			merged[kv[:i]] = kv[i+1:]
		}
	}
	for k, v := range overrides {
		merged[k] = v
	}
	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, k := range keys {
		env = append(env, k+"="+merged[k])
	}
	return env
}

// spawnExec wires one Exec spawn's capture: stdout/stderr both feed the
// task's evidence ring (sanitized, redacted, bounded), and each completed
// line becomes the task's current Doing activity (spec §23) — never parsed
// for totals, only narrated. Cancelling ctx kills the child (ProcessRunner's
// contract); Close flushes any trailing partial line into evidence.
func (o *Output) spawnExec(ctx context.Context, taskID string, spec ExecSpec, target execTarget) (ProcessOutcome, error) {
	task := &TaskHandle{out: o, id: taskID}
	ev := task.evidence(activityFeed(func(line string) { task.Doing(line) }))
	defer func() { _ = ev.Close() }()

	cmd := ProcessCommand{
		Path:   target.ExecutablePath,
		Args:   append([]string(nil), spec.Args...),
		Dir:    target.Dir,
		Env:    mergedExecEnv(spec.Env),
		Stdout: ev.Stdout(),
		Stderr: ev.Stderr(),
	}
	return o.cfg.processRunner.Run(ctx, cmd)
}
