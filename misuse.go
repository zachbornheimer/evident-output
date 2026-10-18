package evo

import (
	"github.com/zachbornheimer/evident-output/v2/internal/misuse"
	txt "github.com/zachbornheimer/evident-output/v2/internal/project"
)

func (r *Run) recordMisuse(err error) {
	r.misuse.Record(err)
}

func (r *Run) recordMisuseFor(subject string, err error) {
	r.misuse.RecordMisuseFor(subject, err)
}

func (r *Run) firstMisuse() error {
	return r.misuse.First()
}

// allMisuse is every distinct misuse recorded, joined in record order.
func (r *Run) allMisuse() error {
	return r.misuse.All()
}

// appendMisuseLineLocked renders the one required line for the first recorded misuse, so an exit code
// never disagrees with what was printed. It is the corrective sentence; machine detail stays in JSON/debug.
func (r *Run) appendMisuseLineLocked() {
	r.graph.Root().Log(txt.MisuseLine(r.misuse.Hint(), !r.cfg.noColor))
}

// recordAlreadyResolvedLocked records ErrAlreadyResolved for a second terminal verb on task name.
func (r *Run) recordAlreadyResolvedLocked(name, rejectedSummary string) {
	r.misuse.Note(misuse.ErrAlreadyResolved, name, rejectedSummary)
}
