package evo

import (
	"context"
	"fmt"
	"io"
	iofs "io/fs"
	"time"

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
)

// --- folded from process.go ---

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
func (e runHost) Spawn(ctx context.Context, id record.TaskID, cmd change.SpawnCommand) (change.ExecResult, error) {
	o := e.out
	task := &Task{out: o, id: string(id)}
	ev := task.capture(activityFeed(func(line string) { task.Doing(line) }))

	argv := append([]string{cmd.Path}, cmd.Args...)
	exitCode, runErr := o.cfg.processRunner.Run(ctx, argv, cmd.Dir, cmd.Env, ev.Stdout(), ev.Stderr())
	if closeErr := ev.Close(); closeErr != nil && runErr == nil {
		runErr = fmt.Errorf("flush capture: %w", closeErr)
	}
	if runErr != nil {
		return change.ExecResult{}, fmt.Errorf("spawn %q: %w", cmd.Path, runErr)
	}
	return change.ExecResult{
		Ran:       true,
		ExitCode:  exitCode,
		Stdout:    ev.StreamText(process.StreamStdout),
		Stderr:    ev.StreamText(process.StreamStderr),
		Truncated: ev.Truncated(),
	}, nil
}

// captureOption configures the Capture a Task's first capture call creates.
type captureOption func(o *Run, taskName string, cfg *record.CaptureConfig)

// activityFeed makes each kept line also become the Task's activity.
func activityFeed(fn func(text string)) captureOption {
	return func(_ *Run, _ string, cfg *record.CaptureConfig) { cfg.OnLine = fn }
}

// capture is the Task's Capture, created on first use with opts: Exec's
// spawns and the phase writer share it, so the tail a failure shows holds
// every line either wrote.
func (t *Task) capture(opts ...captureOption) *record.Capture {
	if t == nil || t.out == nil {
		return newCapture(nil, "", opts)
	}
	o := t.out
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[t.id]
	if st == nil {
		return newCapture(o, "", opts)
	}
	return st.node.Capture(func() *record.Capture { return newCapture(o, st.name, opts) })
}

// newCapture is a Capture that redacts through o's Redactor.
func newCapture(o *Run, taskName string, opts []captureOption) *record.Capture {
	var cfg record.CaptureConfig
	if o != nil {
		cfg.Redact = o.redactString
	}
	for _, opt := range opts {
		if opt != nil {
			opt(o, taskName, &cfg)
		}
	}
	return record.NewCapture(cfg)
}

// detailTail is the Problem option that attaches c's tail to a failure.
func detailTail(c *record.Capture) ProblemOption {
	return problemOption{record.WithEvidenceTail(c.DetailText)}
}

// Clock is the time source a Run reads: Now is the current time.
type Clock interface {
	Now() time.Time
}

// Terminal is the terminal driver that owns a Run's control sequences. ID
// names the driver.
type Terminal interface {
	ID() string
}

// Redactor redacts secrets from captured text before the journal, Capture
// retention and human rendering see it.
type Redactor interface {
	RedactString(s string) string
}

// Process spawns the child processes Exec runs. Run starts argv[0] (an
// absolute path) with argv[1:] in dir with exactly env, writes the child's
// output to stdout and stderr, and returns its exit status. A nonzero
// exitCode is not an error; a non-nil error means the child produced no
// exit status: it never started, or ctx ended it. Run kills the child when
// ctx ends.
type Process interface {
	Run(ctx context.Context, argv []string, dir string, env []string, stdout, stderr io.Writer) (exitCode int, err error)
}

// FS is the filesystem File inspects and writes through.
type FS interface {
	// Lstat inspects path without following a final symlink.
	Lstat(path string) (iofs.FileInfo, error)
	// ReadFile reads path's contents.
	ReadFile(path string) ([]byte, error)
	// WriteAtomic replaces path's contents with contents at mode, so no
	// reader ever sees a partial write.
	WriteAtomic(path string, contents []byte, mode iofs.FileMode) error
	// Chmod sets path's permission bits.
	Chmod(path string, mode iofs.FileMode) error
}

// Facades is every system boundary Evo touches. A zero field selects the
// real one.
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
