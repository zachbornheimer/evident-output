package evo

import (
	"fmt"

	"github.com/zachbornheimer/evident-output/internal/misuse"
	txt "github.com/zachbornheimer/evident-output/internal/project"
)

// misuseGlyph prefixes the one required misuse line the same way the
// existing "! already mutated: ..." row does (writeAlreadyMutated) — "!" is
// attention-only per evo-rec.md's glyph vocabulary, and a misuse line always
// demands attention.
const misuseGlyph = "!"

func (o *Output) recordMisuse(err error) {
	o.misuse.Record(err)
}

func (o *Output) recordMisuseFor(subject string, err error) {
	o.misuse.RecordMisuseFor(subject, err)
}

func (o *Output) firstMisuse() error {
	return o.misuse.First()
}

// appendMisuseLineLocked renders the one required line for the first
// recorded misuse — an exit code may never disagree with everything the
// caller saw printed (beginner-1). The line is the recorder's corrective
// sentence for the recorded sentinel, never the raw "evo: ..." sentinel
// text (release-gate round 4 finding 2): machine detail stays in
// JSON/debug, the human stream gets told what to do next.
func (o *Output) appendMisuseLineLocked() {
	hint := o.misuse.Hint()
	glyph := txt.StyleGlyph(misuseGlyph, txt.SGRYellow, !o.cfg.noColor)
	o.rec.AppendLine(fmt.Sprintf("%s  %s", glyph, hint))
}

// recordAlreadyResolvedLocked records ErrAlreadyResolved for a second
// terminal verb (Fail/Block/Warn/Cancel/Skipped) on task name, retaining
// the rejected call's own summary text — when it carried one — so the
// misuse line can show what got dropped instead of only naming the task
// (release-gate round 5 finding 4).
func (o *Output) recordAlreadyResolvedLocked(name, rejectedSummary string) {
	o.misuse.Note(misuse.ErrAlreadyResolved, name, rejectedSummary)
}

// Err returns the first recorded misuse error, if any.
func (o *Output) Err() error {
	return o.firstMisuse()
}
