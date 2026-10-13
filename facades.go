package evo

import (
	"context"
	"fmt"
	"io"
	"sync/atomic"

	"github.com/zachbornheimer/evident-output/internal/change"
	"github.com/zachbornheimer/evident-output/internal/clock"
	"github.com/zachbornheimer/evident-output/internal/process"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// Facades is the injectable system boundary for a Run: clock, terminal,
// process, filesystem and redaction. Zero values select production defaults.

// --- folded from clock.go ---

// The root package keeps compiling through these aliases until the root
// speaks internal/clock directly.
type (
	// TimeSource provides the current time for deterministic tests.
	timeSource = clock.Clock
	// Scheduler is TimeSource's optional capability to run fn after a delay.
	scheduler = clock.Scheduler
	// systemClock uses the real wall clock.
	systemClock = clock.Wall
	// fixedClock always returns the same instant.
	fixedClock = clock.Fixed
)

// wall is the real clock for waits that must stay real whatever Clock the
// application injected: spinner cadence, paint cost, throttled-write age.
var wall = clock.System()

// --- folded from facade_slot.go ---

// FacadeSlot holds the one public wrapper of an engine handle, so the root
// package hands out a stable wrapper per handle without a process-global
// table. A table keyed by handle kept every handle, and through it every
// Output with all its state, alive for the life of the process; the slot
// lives and dies with its handle.
type facadeSlot struct {
	wrapper atomic.Pointer[any]
}

// Wrapper returns the wrapper stored in the slot, storing newWrapper()'s
// result first when there is none. Concurrent first calls agree on one.
func (s *facadeSlot) loadWrapper(newWrapper func() any) any {
	if w := s.wrapper.Load(); w != nil {
		return *w
	}
	w := newWrapper()
	if s.wrapper.CompareAndSwap(nil, &w) {
		return w
	}
	return *s.wrapper.Load()
}

// Facade returns o's public-wrapper slot.
func (o *Run) facadeSlot() *facadeSlot { return &o.facade }

// Facade returns t's public-wrapper slot.
func (t *Task) facadeSlot() *facadeSlot {
	if t == nil {
		return nil
	}
	return &t.facade
}

// Facade returns g's public-wrapper slot.
func (g *Group) facadeSlot() *facadeSlot { return &g.facade }

// Facade returns g's public-wrapper slot.
func (g *Sequence) facadeSlot() *facadeSlot { return &g.facade }

// Facade returns p's public-wrapper slot.
func (p *printer) facadeSlot() *facadeSlot { return &p.facade }

// --- folded from process.go ---

// ProcessCommand is one resolved external command Exec is about to spawn.
type processCommand = process.Command

// ProcessOutcome is one spawned command's terminal, already-observed result.
type processOutcome = process.Outcome

// ProcessRunner is the facade every evo.Exec spawn goes through instead of
// exec.Cmd/os/exec directly (facade rule): the real runner in ordinary use,
// a scripted testkit fake in tests.
type processRunner = process.Runner

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
	task := &Task{out: o, id: string(id)}
	ev := task.capture(activityFeed(func(line string) { task.Doing(line) }))

	outcome, runErr := o.cfg.processRunner.Run(ctx, processCommand{
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
		Stdout:    ev.streamText(captureStreamStdout),
		Stderr:    ev.streamText(captureStreamStderr),
		Truncated: ev.wasTruncated(),
	}, nil
}

type (
	Clock    = timeSource
	Process  = processRunner
	FS       = fileFS
	Terminal = terminalDriver
)

type Facades struct {
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
	Clock    Clock
	Terminal Terminal
	Redactor Redactor
	Process  Process
	FS       FS
}
