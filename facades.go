package evo

import (
	"context"
	"fmt"
	"io"
	iofs "io/fs"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/change"
	"github.com/zachbornheimer/evident-output/v2/internal/clock"
	"github.com/zachbornheimer/evident-output/v2/internal/process"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// The root package keeps compiling through these aliases until it speaks internal/clock directly.
type (
	// timeSource provides the current time for deterministic tests.
	timeSource = clock.Clock
	// systemClock uses the real wall clock.
	systemClock = clock.Wall
)

// processRunner is the facade every evo.Exec spawn goes through instead of os/exec.
type processRunner = process.Runner

// Spawn wires one Exec spawn's capture: stdout and stderr both feed the task's Capture ring (sanitized,
// redacted, bounded), and each completed line becomes the task's Doing activity, never parsed for totals.
// Cancelling ctx kills the child. ExecResult output comes from that ring and is zero-valued alongside a
// spawn error. Failures are wrapped with the resolved executable path, which the caller's wrap does not know.
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

type captureOption func(o *Run, taskName string, cfg *record.CaptureConfig)

func activityFeed(fn func(text string)) captureOption {
	return func(_ *Run, _ string, cfg *record.CaptureConfig) { cfg.OnLine = fn }
}

// capture is the Task's Capture, created on first use with opts: Exec's spawns and the phase writer share it,
// so the tail a failure shows holds every line either wrote.
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
// absolute path) with argv[1:] in dir with exactly env and returns its exit
// status. A nonzero exitCode is not an error; a non-nil error means no exit
// status: the child never started, or ctx ended it (Run kills it).
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

// flusher is a writer that buffers until told to flush.
type flusher interface {
	Flush() error
}

// humanSinks are the writers human text reaches: the primary stream, its mirrors, and the diagnostic stream.
type humanSinks struct{ o *Run }

func (s humanSinks) Human(text string) {
	cfg := &s.o.cfg
	writers := make([]io.Writer, 0, 1+len(cfg.extraWriters))
	if cfg.primary != nil {
		writers = append(writers, cfg.primary)
	}
	writers = append(writers, cfg.extraWriters...)
	for _, w := range writers {
		if w != nil {
			writeFlushed(w, text)
		}
	}
}

func (s humanSinks) Diagnostic(text string) bool {
	if s.o.cfg.diagnostic == nil {
		return false
	}
	writeFlushed(s.o.cfg.diagnostic, text)
	return true
}

func (s humanSinks) Separate() bool {
	cfg := &s.o.cfg
	return cfg.diagnostic != nil && cfg.primary != nil && cfg.diagnostic != cfg.primary
}

func (s humanSinks) SharesTerminal() bool { return s.o.cfg.diagnosticSharesTerminal }

// writeFlushed writes text to w and flushes it when it buffers.
func writeFlushed(w io.Writer, text string) {
	_, _ = io.WriteString(w, text)
	if f, ok := w.(flusher); ok {
		_ = f.Flush()
	}
}
