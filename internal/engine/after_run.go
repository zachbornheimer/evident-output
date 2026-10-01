package engine

import (
	"fmt"
	"io"
)

// AfterRunFunc writes human text that belongs after the run's footer. w is
// the human stream (never the machine stream), r is the finished Result.
type AfterRunFunc func(w io.Writer, r Result)

// lateWriteNotice is the one line printed when text is written after the run
// ended without an AfterRun hook to carry it.
const lateWriteNotice = "output written after the run finished was dropped; register it with AfterRun before Run"

// AfterRun registers fn to render after the run's footer, in registration
// order, once Run has finished and before the Output closes. It is how a
// caller prints a final summary below the footer: Print/Println after the
// run are dropped and reported as misuse.
//
// The hook writes to the human stream only. Machine projections (JSON,
// JSONL, StreamJSON) never receive it, so stdout stays parseable; a
// machine consumer reads the same facts from the document itself.
func (o *Output) AfterRun(fn AfterRunFunc) {
	if o == nil || fn == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return
	}
	o.afterRun = append(o.afterRun, fn)
}

// AfterRun registers fn on the package-level default Output (see Output.AfterRun).
func AfterRun(fn AfterRunFunc) { Default().AfterRun(fn) }

// runAfterRun renders the registered hooks to the human stream. Called by
// the Run lifecycle after Finish and before Close.
func (o *Output) runAfterRun(r Result) {
	o.mu.Lock()
	hooks := o.afterRun
	o.afterRun = nil
	machineOnly := o.cfg.projection.suppressesHuman()
	w := o.humanWriterLocked()
	o.mu.Unlock()
	if machineOnly || w == nil {
		return
	}
	for _, fn := range hooks {
		fn(w, r)
	}
}

// humanWriterLocked fans writes out to the primary writer and every
// AlsoWrite mirror, or returns nil when there is no human stream.
func (o *Output) humanWriterLocked() io.Writer {
	writers := make([]io.Writer, 0, 1+len(o.cfg.extraWriters))
	if o.cfg.primary != nil {
		writers = append(writers, o.cfg.primary)
	}
	writers = append(writers, o.cfg.extraWriters...)
	if len(writers) == 0 {
		return nil
	}
	return io.MultiWriter(writers...)
}

// reportLateWriteLocked tells the caller, once, that text written after the
// run finished was dropped. A silent drop reads as a rendering bug; the
// notice names the fix.
func (o *Output) reportLateWriteLocked() {
	if !o.finished || o.lateWriteReported || o.cfg.projection.suppressesHuman() {
		return
	}
	o.lateWriteReported = true
	if w := o.humanWriterLocked(); w != nil {
		_, _ = fmt.Fprintln(w, lateWriteNotice)
	}
}
