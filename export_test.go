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

type TestClock = engine.FixedClock
type TestSystemClock = engine.SystemClock
type TestRedactor = engine.NoopRedactor
type TestEvidence = engine.Evidence

func DelayForTest(d time.Duration) *time.Duration { return Delay(d) }
func ReasonConstrained(name string, opts ...ReasonOption) TaxonomyReason {
	return TaxonomyReason{inner: engine.ReasonConstrained(name, opts...)}
}
func SlogHandlerForTest() slog.Handler { return SlogHandler() }

type Scope struct{ inner *engine.Scope }

func (o *Output) ScopeForTest(name string) *Scope {
	if o == nil || o.inner == nil {
		return nil
	}
	return &Scope{inner: o.inner.ScopeForTest(name)}
}

func (s *Scope) Name() string {
	if s == nil || s.inner == nil {
		return ""
	}
	return s.inner.Name()
}

func (s *Scope) Task(name string) *TaskHandle {
	if s == nil || s.inner == nil {
		return nil
	}
	return wrapTask(s.inner.Task(name))
}

func (s *Scope) TaskIdentified(name, key string) *TaskHandle {
	if s == nil || s.inner == nil {
		return nil
	}
	return wrapTask(s.inner.TaskIdentified(name, key))
}

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

func (o *Output) EvidenceForTest(opts ...EvidenceOption) *Evidence {
	if o == nil || o.inner == nil {
		return nil
	}
	return o.inner.EvidenceForTest(opts...)
}

func (o *Output) Events() []Event {
	if o == nil || o.inner == nil {
		return nil
	}
	return o.inner.Events()
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

func (o *Output) TaskIdentified(name, key string) *TaskHandle {
	if o == nil || o.inner == nil {
		return nil
	}
	return wrapTask(o.inner.TaskIdentified(name, key))
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

func (o *Output) SchedulerStartOrder() []string {
	if o == nil || o.inner == nil {
		return nil
	}
	return o.inner.SchedulerStartOrder()
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
	return t.Step(completed, total, name)
}

func (t *TaskHandle) EvidenceForTest(opts ...EvidenceOption) *Evidence {
	if t == nil || t.inner == nil {
		return nil
	}
	return t.inner.EvidenceForTest(opts...)
}

func (t *TaskHandle) SkippedWithErrs(reason TaxonomyReason, name string, errs ...error) {
	if t != nil && t.inner != nil {
		t.inner.SkippedWithErrs(reason.inner, name, errs...)
	}
}

func (t *TaskHandle) NextSelfForTest(args ...string) *TaskHandle {
	if t == nil || t.inner == nil {
		return t
	}
	t.inner.NextSelfForTest(args...)
	return t
}

func (t *TaskHandle) SkipForTest(reason string, args ...any) *TaskHandle {
	if t == nil || t.inner == nil {
		return t
	}
	t.inner.SkipForTest(reason, args...)
	return t
}
