package engine

import (
	"context"
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/change"
	"github.com/zachbornheimer/evident-output/internal/process"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// ProcessCommand is one resolved external command Exec is about to spawn.
type ProcessCommand = process.Command

// ProcessOutcome is one spawned command's terminal, already-observed result.
type ProcessOutcome = process.Outcome

// ProcessRunner is the facade every evo.Exec spawn goes through instead of
// exec.Cmd/os/exec directly (facade rule): the real runner in ordinary use,
// a scripted testkit fake in tests.
type ProcessRunner = process.Runner

// Spawn wires one Exec spawn's capture: stdout/stderr both feed the task's
// Capture ring (sanitized, redacted, bounded), and each completed line
// becomes the task's current Doing activity (spec §23) — never parsed for
// totals, only narrated. Cancelling ctx kills the child (ProcessRunner's
// contract); Close flushes any trailing partial line into Capture before the
// result reads it back. The returned ExecResult's Stdout/Stderr come from
// that same Capture ring (sanitized, redacted, bounded), never a second
// unbounded copy; it is zero-valued alongside a spawn error. A spawn or
// Capture-flush failure is wrapped with the resolved executable path here
// (rather than left bare) since the caller's own wrap only knows
// ExecSpec.Executable, not the path Evo actually resolved and tried to run.
func (h changeHost) Spawn(ctx context.Context, id record.TaskID, cmd change.SpawnCommand) (change.ExecResult, error) {
	o := h.out
	task := &TaskHandle{out: o, id: string(id)}
	ev := task.Capture(activityFeed(func(line string) { task.Doing(line) }))

	outcome, runErr := o.cfg.processRunner.Run(ctx, ProcessCommand{
		Path:   cmd.Path,
		Args:   cmd.Args,
		Dir:    cmd.Dir,
		Env:    cmd.Env,
		Stdout: ev.Stdout(),
		Stderr: ev.Stderr(),
	})
	if closeErr := ev.Close(); closeErr != nil && runErr == nil {
		runErr = fmt.Errorf("flush capture: %w", closeErr)
	}
	if runErr != nil {
		return change.ExecResult{}, fmt.Errorf("spawn %q: %w", cmd.Path, runErr)
	}
	return change.ExecResult{
		Ran:       true,
		ExitCode:  outcome.ExitCode,
		Stdout:    ev.streamText(CaptureStreamStdout),
		Stderr:    ev.streamText(CaptureStreamStderr),
		Truncated: ev.wasTruncated(),
	}, nil
}
