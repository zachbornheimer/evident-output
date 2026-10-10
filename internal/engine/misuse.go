package engine

import (
	"fmt"

	txt "github.com/zachbornheimer/evident-output/internal/text"
)

// misuseSink lets the graph report the misuse it finds into the Output. The
// graph calls it from its own goroutines, with or without Output.mu held, so
// recording misuse takes only the leaf lock that guards the first misuse
// (misuseMu), never Output.mu.
type misuseSink struct{ o *Output }

func (s misuseSink) RecordMisuseFor(subject string, err error) { s.o.recordMisuseFor(subject, err) }

func (o *Output) recordMisuse(err error) {
	o.noteMisuse(err, "", "")
}

// noteMisuse records err as the run's misuse when it is the first, with the
// entity and rejected summary that name it. Any goroutine may call it.
func (o *Output) noteMisuse(err error, subject, rejectedSummary string) {
	if err == nil {
		return
	}
	o.misuseMu.Lock()
	if o.misuse == nil {
		o.misuse, o.misuseSubject, o.misuseRejectedSummary = err, subject, rejectedSummary
	}
	o.misuseMu.Unlock()
	if o.cfg.strict {
		panic(err)
	}
}

// firstMisuse is the first misuse recorded, nil when there is none.
func (o *Output) firstMisuse() error {
	o.misuseMu.Lock()
	defer o.misuseMu.Unlock()
	return o.misuse
}

// appendMisuseLineLocked renders the one required line for the first
// recorded misuse — an exit code may never disagree with everything the
// caller saw printed (beginner-1). The line is misuseHintFor's corrective
// sentence for the recorded sentinel, never the raw "evo: ..." sentinel text
// (release-gate round 4 finding 2): machine detail stays in JSON/debug, the
// human stream gets told what to do next.
func (o *Output) appendMisuseLineLocked() {
	o.misuseMu.Lock()
	hint := misuseHintFor(o.misuse, o.misuseSubject, o.misuseRejectedSummary)
	o.misuseMu.Unlock()
	glyph := txt.StyleGlyph(misuseGlyph, txt.SGRYellow, !o.cfg.noColor)
	o.rec.AppendLine(fmt.Sprintf("%s  %s", glyph, hint))
}

// recordMisuseFor is recordMisuse with the offending entity's name attached,
// so Finish can name it in the one required misuse line (beginner-1) instead
// of an exit code silently disagreeing with everything the caller saw
// rendered.
func (o *Output) recordMisuseFor(subject string, err error) {
	o.noteMisuse(err, subject, "")
}

// recordAlreadyResolvedLocked records ErrAlreadyResolved for a second
// terminal verb (Fail/Block/Warn/Cancel/Skipped) on task name, retaining
// the rejected call's own summary text — when it carried one — so the
// misuse line can show what got dropped instead of only naming the task
// (release-gate round 5 finding 4).
func (o *Output) recordAlreadyResolvedLocked(name, rejectedSummary string) {
	o.noteMisuse(ErrAlreadyResolved, name, rejectedSummary)
}

// Err returns the first recorded misuse error, if any.
func (o *Output) Err() error {
	return o.firstMisuse()
}
