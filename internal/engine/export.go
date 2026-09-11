package engine

import (
	"io"
	"log/slog"
	"os/exec"
	"time"
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
func DataProjection() Option                 { return dataProjection() }
func ExternalProjection() Option             { return externalProjection() }
func DebugHistory() Option                   { return debugHistory() }
func DebugPane(opts ...DebugPaneOption) Option {
	return debugPane(opts...)
}
func KeepLastLines(n int) EvidenceOption    { return keepLastLines(n) }
func MaxEvidenceBytes(n int) EvidenceOption { return maxEvidenceBytes(n) }
func MirrorToDiagnostics() EvidenceOption   { return mirrorToDiagnostics() }
func MirrorToDebug() EvidenceOption         { return mirrorToDebug() }
func PaneHeight(lines int) DebugPaneOption  { return paneHeight(lines) }
func NewestFirst() DebugPaneOption          { return newestFirst() }
func OldestFirst() DebugPaneOption          { return oldestFirst() }
func PreserveDebugTail() DebugPaneOption    { return preserveDebugTail() }
func ID(id string) EntityOption             { return iD(id) }
func StartPhase(text string) EntityOption {
	return entityOptionFunc(func(o *entityOpts) { o.phase = text })
}
func RenderPlain(s Snapshot, opts PlainOptions) ([]byte, error) {
	return renderPlain(s, opts)
}
func MainWith(out *Output, run func(*Output) error) { mainWith(out, run) }

type Evidence = evidence
type Scope = scope
type SystemClock = systemClock
type FixedClock = fixedClock
type NoopRedactor = noopRedactor

// Test helpers reachable through the evo type alias (root export_test.go
// cannot attach methods to engine types).

func (t *TaskHandle) RunForTest(cmd *exec.Cmd) error { return t.run(cmd) }
func (t *TaskHandle) StepForTest(completed, total int, name string) *TaskHandle {
	return t.Step(completed, total, name)
}
func (t *TaskHandle) EvidenceForTest(opts ...EvidenceOption) *evidence {
	return t.evidence(opts...)
}
func (o *Output) EvidenceForTest(opts ...EvidenceOption) *evidence { return o.evidence(opts...) }
func (o *Output) Events() []Event                                  { return o.copyEvents() }
func (o *Output) ScopeForTest(name string) *scope                  { return o.scope(name) }
func (o *Output) DebugForTest(message string, fields ...Field) {
	o.debug(message, fields...)
}
func (o *Output) AlsoWriteForTest(w io.Writer) {
	if w == nil {
		return
	}
	o.cfg.extraWriters = append(o.cfg.extraWriters, w)
}
func (o *Output) TaskIdentified(name, key string) *TaskHandle {
	return o.taskScoped(name, "", iD(key))
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
	o.live.liveActive = true
}
func (o *Output) DebugWriterForTest() io.WriteCloser { return o.debugWriter() }
func (o *Output) DeclareDryRunForTest()              { o.declareDryRun() }
func (o *Output) AboutForTest(text string)           { o.about(text) }
func (o *Output) SubjectForTest(text string)         { o.subject(text) }
func (o *Output) SlogHandlerForTest() slog.Handler   { return o.slogHandler() }
func (t *TaskHandle) NextSelfForTest(args ...string) *TaskHandle {
	return t.nextSelf(args...)
}
func (t *TaskHandle) SkipForTest(reason string, args ...any) *TaskHandle {
	return t.skip(reason, args...)
}
func (o *Output) AtForTest(visibility Visibility) *Printer { return o.at(visibility) }
func (o *Output) SchedulerStartOrder() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.schedStartOrder...)
}
func (o *Output) SchedulerMaxObserved() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.schedMaxObserved
}

func ReasonConstrained(name string, opts ...ReasonOption) TaxonomyReason {
	return Default().reasonGetOrCreate(name, opts...)
}

func (s *scope) TaskIdentified(name, key string) *TaskHandle {
	if s == nil || s.out == nil {
		return &TaskHandle{}
	}
	return s.out.taskScoped(name, s.name, iD(key))
}
