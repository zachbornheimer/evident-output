package evo

import (
	"io"
	"log/slog"
	"os/exec"
	"time"
)

// TestClock is the deterministic TimeSource for evo_test.
type TestClock = fixedClock

// TestSystemClock is the wall-clock TimeSource for evo_test.
type TestSystemClock = systemClock

// TestRedactor is the identity Redactor for evo_test.
type TestRedactor = noopRedactor

// RunForTest executes cmd as this task's subprocess (unexported TaskHandle.run).
func (t *TaskHandle) RunForTest(cmd *exec.Cmd) error {
	return t.run(cmd)
}

// DelayForTest returns a non-nil *time.Duration for Config.VisibilityDelay.
func DelayForTest(d time.Duration) *time.Duration {
	return delay(d)
}

// Test-only re-exports of unexported production helpers. The dialect-surface
// lock ignores _test.go, so evo_test can keep calling these names.
func RenderPlain(s Snapshot, opts PlainOptions) ([]byte, error) {
	return renderPlain(s, opts)
}

func EncodeJSON(s Snapshot) ([]byte, error) { return encodeJSON(s) }

func EncodeJSONL(events []Event) ([]byte, error) { return encodeJSONL(events) }

func EncodeEventJSON(e Event) ([]byte, error) { return encodeEventJSON(e) }

func (o *Output) Events() []Event { return o.copyEvents() }

type TestScope = scope

type TestEvidence = evidence

func (o *Output) ScopeForTest(name string) *TestScope { return o.scope(name) }

func (t *TaskHandle) EvidenceForTest(opts ...EvidenceOption) *TestEvidence {
	return t.evidence(opts...)
}

func (o *Output) EvidenceForTest(opts ...EvidenceOption) *TestEvidence {
	return o.evidence(opts...)
}

func (t *TaskHandle) StepForTest(completed, total int, name string) *TaskHandle {
	return t.step(completed, total, name)
}

func (o *Output) DebugForTest(message string, fields ...Field) {
	o.debug(message, fields...)
}

func KeepLastLines(n int) EvidenceOption { return keepLastLines(n) }

func MaxEvidenceBytes(n int) EvidenceOption { return maxEvidenceBytes(n) }

func MirrorToDiagnostics() EvidenceOption { return mirrorToDiagnostics() }

func MirrorToDebug() EvidenceOption { return mirrorToDebug() }

func AlsoWrite(w io.Writer) Option { return alsoWrite(w) }

func (o *Output) AlsoWriteForTest(w io.Writer) {
	if w == nil {
		return
	}
	o.cfg.extraWriters = append(o.cfg.extraWriters, w)
}

func (o *Output) TaskIdentified(name, key string) *TaskHandle {
	return o.taskScoped(name, "", iD(key))
}

func ReasonConstrained(name string, opts ...ReasonOption) TaxonomyReason {
	return Default().reasonGetOrCreate(name, opts...)
}

func (t *TaskHandle) SkippedWithErrs(reason TaxonomyReason, name string, errs ...error) {
	t.recordTaxonomy(reason, name, dispositionSkip, errs)
}

func (o *Output) SetDiagnosticSharesTerminalForTest() {
	o.cfg.diagnosticSharesTerminal = true
}

func (o *Output) DropDiagnosticForTest() {
	o.cfg.diagnostic = nil
}

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

func (s *scope) TaskIdentified(name, key string) *TaskHandle {
	if s == nil || s.out == nil {
		return &TaskHandle{}
	}
	return s.out.taskScoped(name, s.name, iD(key))
}

func (o *Output) DebugWriterForTest() io.WriteCloser { return o.debugWriter() }

func (o *Output) DeclareDryRunForTest() { o.declareDryRun() }

func (o *Output) AboutForTest(text string) { o.about(text) }

func (o *Output) SubjectForTest(text string) { o.subject(text) }

func (o *Output) SlogHandlerForTest() slog.Handler { return o.slogHandler() }

func SlogHandlerForTest() slog.Handler { return slogHandler() }

func (t *TaskHandle) NextSelfForTest(args ...string) *TaskHandle { return t.nextSelf(args...) }

func (t *TaskHandle) SkipForTest(reason string, args ...any) *TaskHandle {
	return t.skip(reason, args...)
}

func (o *Output) AtForTest(visibility Visibility) *Printer { return o.at(visibility) }

// SchedulerStartOrder is the name of each Task in the order the scheduler
// actually started it.
func (o *Output) SchedulerStartOrder() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.schedStartOrder...)
}

// SchedulerMaxObserved is the highest in-flight count the scheduler reached.
func (o *Output) SchedulerMaxObserved() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.schedMaxObserved
}

func IsCharDevice(w io.Writer) bool { return isCharDevice(w) }

// SwapLookupEnv replaces the process-environment facade. Tests inject a
// map instead of racing os.Setenv. Restore via the returned function.
func SwapLookupEnv(fn func(string) string) func() {
	lookupEnvMu.Lock()
	prev := lookupEnvFn
	lookupEnvFn = fn
	lookupEnvMu.Unlock()
	return func() {
		lookupEnvMu.Lock()
		lookupEnvFn = prev
		lookupEnvMu.Unlock()
	}
}

// MarkWriterAsCharDevice treats w as a TTY during construction so tests can
// exercise live-region / color inference without opening a pty. Only this
// writer matches; other tests' writers still use the OS check.
func MarkWriterAsCharDevice(w io.Writer) func() {
	charDeviceOverride.mu.Lock()
	prevW, prevOn := charDeviceOverride.writer, charDeviceOverride.on
	charDeviceOverride.writer = w
	charDeviceOverride.on = true
	charDeviceOverride.mu.Unlock()
	return func() {
		charDeviceOverride.mu.Lock()
		charDeviceOverride.writer = prevW
		charDeviceOverride.on = prevOn
		charDeviceOverride.mu.Unlock()
	}
}
