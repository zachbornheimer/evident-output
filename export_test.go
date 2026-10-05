package evo

import (
	"io"
	"log/slog"
	"os/exec"
	"time"

	"github.com/zachbornheimer/evident-output/internal/engine"
	"github.com/zachbornheimer/evident-output/internal/render/machine"
)

// The JSON*ForTest aliases and Encode*ForTest functions below keep the
// legacy output.v1/event.v1 detailed-document encoders (internal/render)
// covered from package-external tests during the compatibility window,
// mirroring StepForTest's rationale: the owner vocabulary freeze
// (2026-09-25) removed JSONDocument/EncodeJSON/EncodeJSONL/EncodeEventJSON
// and the JSON* wire types from the public dialect (WriteJSON's v2 "evo.run"
// document is the sanctioned external JSON path), but the internal
// machine.EncodeJSON/EncodeJSONL machinery FormatJSON/FormatJSONL still call
// at Finish (internal/engine/machine.go) is unaffected and stays regression
// tested through these test-only aliases rather than dropped.

type (
	JSONDocumentForTest     = machine.JSONDocument
	JSONMessageForTest      = machine.JSONMessage
	JSONOutputMetaForTest   = machine.JSONOutputMeta
	ConclusionJSONForTest   = machine.ConclusionJSON
	JSONProblemForTest      = machine.JSONProblem
	JSONTaskForTest         = machine.JSONTask
	JSONProgressForTest     = machine.JSONProgress
	JSONCollectionForTest   = machine.JSONCollection
	JSONChangesForTest      = machine.JSONChanges
	JSONPlanForTest         = machine.JSONPlan
	JSONEffectRecordForTest = machine.JSONEffectRecord
	JSONActionForTest       = machine.JSONAction
	JSONCommandForTest      = machine.JSONCommand
	EventJSONForTest        = machine.EventJSON
)

const JSONSchemaVersionForTest = machine.JSONSchemaVersion

func EncodeJSONForTest(s Snapshot) ([]byte, error) { return machine.EncodeJSON(s) }
func EncodeJSONLForTest(events []Event) ([]byte, error) {
	return machine.EncodeJSONL(events)
}
func EncodeEventJSONForTest(e Event) ([]byte, error) { return machine.EncodeEventJSON(e) }

func SwapLookupEnv(fn func(string) string) func() { return engine.SwapLookupEnv(fn) }
func MarkWriterAsCharDevice(w io.Writer) func()   { return engine.MarkWriterAsCharDevice(w) }

type (
	TestClock       = engine.FixedClock
	TestSystemClock = engine.SystemClock
	TestRedactor    = engine.NoopRedactor
	TestCapture     = engine.Capture
)

func DelayForTest(d time.Duration) *time.Duration { return Delay(d) }
func SlogHandlerForTest() slog.Handler            { return SlogHandler() }

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

func (t *TaskHandle) CaptureForTest(opts ...CaptureOption) *Capture {
	if t == nil || t.inner == nil {
		return nil
	}
	return t.inner.CaptureForTest(opts...)
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
