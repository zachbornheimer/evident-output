package engine

import (
	"io"
	"log/slog"
	"time"

	"github.com/zachbornheimer/evident-output/internal/process"
)

// Exported aliases for the public evo facade.

func To(w io.Writer) Option                  { return to(w) }
func Diagnostics(w io.Writer) Option         { return withDiagnostics(w) }
func ResultStream(w io.Writer) Option        { return resultStream(w) }
func Plain() Option                          { return plain() }
func NoColor() Option                        { return withNoColor() }
func Width(columns int) Option               { return withWidth(columns) }
func Clock(ts TimeSource) Option             { return withClock(ts) }
func VisibilityDelay(d time.Duration) Option { return visibilityDelay(d) }
func MaxFrameRate(n int) Option              { return maxFrameRate(n) }
func Strict() Option                         { return strict() }
func DryRun() Option                         { return dryRun() }
func Stdin(r io.Reader) Option               { return stdin(r) }
func Terminal(driver TerminalDriver) Option  { return withTerminal(driver) }
func DebugLevel(level LogLevel) Option       { return debugLevel(level) }
func DebugAddSource() Option                 { return debugAddSource() }
func MaxEntities(n int) Option               { return maxEntities(n) }
func MaxEvents(n int) Option                 { return maxEvents(n) }
func AlsoWrite(w io.Writer) Option           { return alsoWrite(w) }
func Redact(r Redactor) Option               { return redact(r) }
func Runner(r ProcessRunner) Option          { return withProcessRunner(r) }
func DataProjection() Option                 { return dataProjection() }
func ExternalProjection() Option             { return externalProjection() }
func DebugHistory() Option                   { return debugHistory() }
func DebugPane(opts ...DebugPaneOption) Option {
	return debugPane(opts...)
}
func KeepLastLines(n int) CaptureOption    { return keepLastLines(n) }
func MaxCaptureBytes(n int) CaptureOption  { return maxCaptureBytes(n) }
func MirrorToDiagnostics() CaptureOption   { return mirrorToDiagnostics() }
func MirrorToDebug() CaptureOption         { return mirrorToDebug() }
func PaneHeight(lines int) DebugPaneOption { return paneHeight(lines) }
func NewestFirst() DebugPaneOption         { return newestFirst() }
func OldestFirst() DebugPaneOption         { return oldestFirst() }
func PreserveDebugTail() DebugPaneOption   { return preserveDebugTail() }
func RenderPlain(s Snapshot, opts PlainOptions) ([]byte, error) {
	return renderPlain(s, opts)
}

type (
	Capture      = evidence
	SystemClock  = systemClock
	FixedClock   = fixedClock
	NoopRedactor = noopRedactor
)

// Test helpers reachable through the evo type alias (root export_test.go
// cannot attach methods to engine types).

func (t *TaskHandle) RunForTest(cmd *process.Cmd) error { return t.run(cmd) }

func (t *TaskHandle) StepForTest(completed, total int, name string) *TaskHandle {
	return t.Progress(completed, total).Doing(name)
}

func (t *TaskHandle) CaptureForTest(opts ...CaptureOption) *evidence {
	return t.Capture(opts...)
}
func (o *Output) CaptureForTest(opts ...CaptureOption) *evidence { return o.capture(opts...) }
func (o *Output) Events() []Event                                { return o.copyEvents() }
func (o *Output) DebugForTest(message string, fields ...Field) {
	o.debug(message, fields...)
}

func (o *Output) AlsoWriteForTest(w io.Writer) {
	if w == nil {
		return
	}
	o.cfg.extraWriters = append(o.cfg.extraWriters, w)
}

func (t *TaskHandle) SkippedWithErrs(reason TaxonomyReason, name string, errs ...error) {
	t.recordTaxonomy(reason, name, dispositionSkip, errs)
}
func (o *Output) SetDiagnosticSharesTerminalForTest() { o.cfg.diagnosticSharesTerminal = true }
func (o *Output) DropDiagnosticForTest()              { o.cfg.diagnostic = nil }
func (o *Output) ForceLiveVisibleForTest() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.live == nil {
		o.live = &liveEngine{}
		if live := o.liveLocked(); live != nil {
			o.live.surface = live
		}
	}
	o.live.visible = true
	o.live.paintMu.Lock()
	o.live.liveActive = true
	o.live.paintMu.Unlock()
}
func (o *Output) DebugWriterForTest() io.WriteCloser           { return o.debugWriter() }
func (o *Output) DeclareDryRunForTest()                        { o.declareDryRun() }
func (o *Output) AboutForTest(text string)                     { o.about(text) }
func (o *Output) SubjectForTest(text string)                   { o.subject(text) }
func (o *Output) SlogHandlerForTest() slog.Handler             { return o.slogHandler() }
func (o *Output) NextSelfForTest(args ...string) ProblemOption { return o.nextSelf(args...) }
func (t *TaskHandle) SkipForTest(reason string, args ...any) *TaskHandle {
	return t.skip(reason, args...)
}
func (o *Output) AtForTest(visibility Visibility) *Printer { return o.at(visibility) }
func (o *Output) SchedulerMaxObserved() int {
	return o.graph.MaxObserved()
}

func ReasonConstrained(name string, opts ...ReasonOption) TaxonomyReason {
	return Default().reasonGetOrCreate(name, opts...)
}
