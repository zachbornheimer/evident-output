package evo

import (
	"io"
	"log/slog"
	"os/exec"
	"time"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

func SwapLookupEnv(fn func(string) string) func() { return engine.SwapLookupEnv(fn) }
func MarkWriterAsCharDevice(w io.Writer) func()   { return engine.MarkWriterAsCharDevice(w) }

type (
	TestClock       = engine.FixedClock
	TestSystemClock = engine.SystemClock
	TestRedactor    = engine.NoopRedactor
	TestCapture     = engine.Capture
)

func DelayForTest(d time.Duration) *time.Duration { return Delay(d) }
func ReasonConstrained(name string, opts ...engine.ReasonOption) TaxonomyReason {
	return TaxonomyReason{inner: engine.ReasonConstrained(name, opts...)}
}
func KeepLastLinesForTest(n int) CaptureOption { return engine.KeepLastLines(n) }
func MirrorToDiagnosticsForTest() CaptureOption {
	return engine.MirrorToDiagnostics()
}
func MirrorToDebugForTest() CaptureOption           { return engine.MirrorToDebug() }
func ForSkipForTest() engine.ReasonOption           { return engine.ForSkip() }
func OnTaskForTest(name string) engine.ReasonOption { return engine.OnTask(name) }
func SlogHandlerForTest() slog.Handler              { return SlogHandler() }

func (o *Output) AboutForTest(text string) {
	if o != nil && o.inner != nil {
		o.inner.AboutForTest(text)
	}
}

func (o *Output) SubjectForTest(text string) {
	if o != nil && o.inner != nil {
		o.inner.SubjectForTest(text)
	}
}

func (o *Output) DeclareDryRunForTest() {
	if o != nil && o.inner != nil {
		o.inner.DeclareDryRunForTest()
	}
}

func (o *Output) CaptureForTest(opts ...CaptureOption) *Capture {
	if o == nil || o.inner == nil {
		return nil
	}
	return o.inner.CaptureForTest(opts...)
}

func (o *Output) DebugForTest(message string, fields ...Field) {
	if o != nil && o.inner != nil {
		o.inner.DebugForTest(message, fields...)
	}
}

func (o *Output) AlsoWriteForTest(w io.Writer) {
	if o != nil && o.inner != nil {
		o.inner.AlsoWriteForTest(w)
	}
}

func (o *Output) SetDiagnosticSharesTerminalForTest() {
	if o != nil && o.inner != nil {
		o.inner.SetDiagnosticSharesTerminalForTest()
	}
}

func (o *Output) DropDiagnosticForTest() {
	if o != nil && o.inner != nil {
		o.inner.DropDiagnosticForTest()
	}
}

func (o *Output) ForceLiveVisibleForTest() {
	if o != nil && o.inner != nil {
		o.inner.ForceLiveVisibleForTest()
	}
}

func (o *Output) DebugWriterForTest() io.WriteCloser {
	if o == nil || o.inner == nil {
		return nil
	}
	return o.inner.DebugWriterForTest()
}

func (o *Output) SlogHandlerForTest() slog.Handler {
	if o == nil || o.inner == nil {
		return nil
	}
	return o.inner.SlogHandlerForTest()
}

func (o *Output) AtForTest(visibility Visibility) *Printer {
	if o == nil || o.inner == nil {
		return nil
	}
	return wrapPrinter(o.inner.AtForTest(visibility))
}

func (o *Output) SchedulerMaxObserved() int {
	if o == nil || o.inner == nil {
		return 0
	}
	return o.inner.SchedulerMaxObserved()
}

func (t *TaskHandle) RunForTest(cmd *exec.Cmd) error {
	if t == nil || t.inner == nil {
		return nil
	}
	return t.inner.RunForTest(cmd)
}

func (t *TaskHandle) StepForTest(completed, total int, name string) *TaskHandle {
	return t.Progress(completed, total).Doing(name)
}

func (t *TaskHandle) CaptureForTest(opts ...CaptureOption) *Capture {
	if t == nil || t.inner == nil {
		return nil
	}
	return t.inner.Capture(opts...)
}

func (t *TaskHandle) SkippedWithErrs(reason TaxonomyReason, name string, errs ...error) {
	if t != nil && t.inner != nil {
		t.inner.SkippedWithErrs(reason.inner, name, errs...)
	}
}

func (o *Output) NextSelfForTest(args ...string) ProblemOption {
	return o.inner.NextSelfForTest(args...)
}

func (t *TaskHandle) SkipForTest(reason string, args ...any) *TaskHandle {
	if t == nil || t.inner == nil {
		return t
	}
	t.inner.SkipForTest(reason, args...)
	return t
}
