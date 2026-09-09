package engine

import (
	"io"
	"log/slog"
	"os/exec"
)

// Test-only method names. DialectSurface skips ForTest/Identified suffixes.
func (t *TaskHandle) EvidenceForTest(opts ...EvidenceOption) *Evidence {
	return t.evidence(opts...)
}

func (o *Output) EvidenceForTest(opts ...EvidenceOption) *Evidence {
	return o.evidence(opts...)
}

func (t *TaskHandle) StepForTest(completed, total int, name string) *TaskHandle {
	return t.step(completed, total, name)
}

func (t *TaskHandle) NextSelfForTest(args ...string) *TaskHandle {
	return t.nextSelf(args...)
}

func (t *TaskHandle) SkipForTest(reason string, args ...any) *TaskHandle {
	return t.skip(reason, args...)
}

func (t *TaskHandle) RunForTest(cmd *exec.Cmd) error {
	return t.run(cmd)
}

func (o *Output) ResultWriterForTest() io.Writer { return o.resultWriter() }

func (o *Output) AtForTest(visibility Visibility) *Printer { return o.at(visibility) }

func (o *Output) SubjectForTest(text string) { o.subject(text) }

func (o *Output) AboutForTest(text string) { o.subject(text) }

func (o *Output) DeclareDryRunForTest() { o.declareDryRun() }

func (o *Output) DebugForTest(message string, fields ...Field) { o.debug(message, fields...) }

func (o *Output) DebugWriterForTest() io.WriteCloser { return o.debugWriter() }

func (o *Output) SlogHandlerForTest() slog.Handler { return o.slogHandler() }

func (o *Output) ScopeForTest(name string) *Scope { return o.scope(name) }

func (o *Output) TaskIdentified(name, key string) *TaskHandle {
	return o.taskScoped(name, "", ID(key))
}

func ReasonConstrainedForTest(name string, opts ...ReasonOption) TaxonomyReason {
	return Default().reasonGetOrCreate(name, opts...)
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

func (s *Scope) TaskIdentified(name, key string) *TaskHandle {
	if s == nil || s.out == nil {
		return &TaskHandle{}
	}
	return s.out.taskScoped(name, s.name, ID(key))
}
