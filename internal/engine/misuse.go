package engine

import (
	"fmt"

	txt "github.com/zachbornheimer/evident-output/internal/text"
)

func (o *Output) recordMisuse(err error) {
	if err == nil {
		return
	}
	if o.misuse == nil {
		o.misuse = err
	}
	if o.cfg.strict {
		panic(err)
	}
}

// appendMisuseLineLocked renders the one required line for the first
// recorded misuse — an exit code may never disagree with everything the
// caller saw printed (beginner-1). The line is misuseHintFor's corrective
// sentence for the recorded sentinel, never the raw "evo: ..." sentinel text
// (release-gate round 4 finding 2): machine detail stays in JSON/debug, the
// human stream gets told what to do next.
func (o *Output) appendMisuseLineLocked() {
	hint := misuseHintFor(o.misuse, o.misuseSubject, o.misuseRejectedSummary)
	glyph := txt.StyleGlyph(misuseGlyph, txt.SGRYellow, !o.cfg.noColor)
	o.lines = append(o.lines, fmt.Sprintf("%s  %s", glyph, hint))
}

// recordMisuseFor is recordMisuse with the offending entity's name attached,
// so Finish can name it in the one required misuse line (beginner-1) instead
// of an exit code silently disagreeing with everything the caller saw
// rendered.
func (o *Output) recordMisuseFor(subject string, err error) {
	if err == nil {
		return
	}
	if o.misuse == nil {
		o.misuseSubject = subject
	}
	o.recordMisuse(err)
}

// recordAlreadyResolvedLocked records ErrAlreadyResolved for a second
// terminal verb (Done/Fail/Block/Warn/Cancel/Skip) on task name, retaining
// the rejected call's own summary text — when it carried one — so the
// misuse line can show what got dropped instead of only naming the task
// (release-gate round 5 finding 4).
func (o *Output) recordAlreadyResolvedLocked(name, rejectedSummary string) {
	if o.misuse == nil {
		o.misuseRejectedSummary = rejectedSummary
	}
	o.recordMisuseFor(name, ErrAlreadyResolved)
}

// Err returns the first recorded misuse error, if any.
func (o *Output) Err() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.misuse
}
