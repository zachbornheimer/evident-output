package engine

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/zachbornheimer/evident-output/internal/process"
)

// ProcessCommand is one resolved external command Exec is about to spawn.
type ProcessCommand = process.Command

// ProcessOutcome is one spawned command's terminal, already-observed result.
type ProcessOutcome = process.Outcome

// ProcessRunner is the facade every evo.Exec spawn goes through instead of
// exec.Cmd/os/exec directly (facade rule): the real runner in ordinary use,
// a scripted testkit fake in tests.
type ProcessRunner = process.Runner

// processEnviron is the facade mergedExecEnv reads the process's own
// environment through (facade rule).
var processEnviron = process.Environ

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
		if before, after, ok := strings.Cut(kv, "="); ok {
			merged[before] = after
		}
	}
	maps.Copy(merged, overrides)
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
// task's Capture ring (sanitized, redacted, bounded), and each completed
// line becomes the task's current Doing activity (spec §23) — never parsed
// for totals, only narrated. Cancelling ctx kills the child (ProcessRunner's
// contract); Close flushes any trailing partial line into Capture before
// the result reads it back. The returned ExecResult's Stdout/Stderr come
// from that same Capture ring (sanitized, redacted, bounded), never a
// second unbounded copy; it is zero-valued alongside a spawn error. A spawn or Capture-flush failure is wrapped
// with the resolved executable path here (rather than left bare) since the
// caller's own wrap only knows ExecSpec.Executable, not the path Evo
// actually resolved and tried to run.
func (o *Output) spawnExec(ctx context.Context, taskID string, spec ExecSpec, target execTarget) (ExecResult, error) {
	task := &TaskHandle{out: o, id: taskID}
	ev := task.Capture(activityFeed(func(line string) { task.Doing(line) }))

	cmd := ProcessCommand{
		Path:   target.ExecutablePath,
		Args:   append([]string(nil), spec.Args...),
		Dir:    target.Dir,
		Env:    mergedExecEnv(spec.Env),
		Stdout: ev.Stdout(),
		Stderr: ev.Stderr(),
	}
	outcome, runErr := o.cfg.processRunner.Run(ctx, cmd)
	if closeErr := ev.Close(); closeErr != nil && runErr == nil {
		runErr = fmt.Errorf("flush capture: %w", closeErr)
	}
	if runErr != nil {
		return ExecResult{}, fmt.Errorf("spawn %q: %w", target.ExecutablePath, runErr)
	}
	return ExecResult{
		Ran:       true,
		ExitCode:  outcome.ExitCode,
		Stdout:    ev.streamText(CaptureStreamStdout),
		Stderr:    ev.streamText(CaptureStreamStderr),
		Truncated: ev.wasTruncated(),
	}, nil
}
