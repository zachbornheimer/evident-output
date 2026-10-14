package project

import (
	"slices"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// verdict folds g's own Tasks and its nested collections (P3's recursive
// nesting) into one state: a nested collection contributes exactly as one
// more Task would, so a failure three levels deep still surfaces at the
// root header.
func (r *Rows) verdict(g *DeclaredGroup) record.State {
	if g.Node.BuilderFailed() {
		return record.Failed
	}
	if len(g.Tasks) == 0 && len(g.Groups) == 0 {
		if g.Node.BuilderNotStarted() {
			return record.NotStarted
		}
		return record.Empty
	}
	counts := r.lens.States(g.ID)
	v := foldCounts(counts)
	started, unstarted := len(g.Tasks)-counts.NotStarted, counts.NotStarted
	for _, child := range g.Groups {
		switch s := r.verdict(child); s {
		case record.Empty:
		case record.NotStarted:
			unstarted++
		default:
			started++
			v.add(s)
		}
	}
	// A NotStarted child borrows its collection's verdict from the sibling
	// that failed first. When every child is NotStarted there is no such
	// sibling — another subject stopped the run — and folding to Done would
	// put a check over a subject that never ran.
	if started == 0 && unstarted > 0 {
		return record.NotStarted
	}
	return v.state()
}

// summary is g's Summary as its row shows it in state: when its builder
// failed or never ran, the cause that left there; otherwise only when it succeeded
// with no warning or failure anywhere beneath it.
func (r *Rows) summary(g *DeclaredGroup, state record.State) string {
	if g.Node.BuilderFailed() || g.Node.BuilderNotStarted() {
		return g.Rec.Summary()
	}
	if !state.Succeeded() || g.Rec.Summary() == "" || r.warnedOrFailedBelow(g) {
		return ""
	}
	return g.Rec.Summary()
}

// warnedOrFailedBelow reports a Failed or Cancelled Task, or one with a
// warning, in g or any collection nested in it (E2.5 finding 1): a success
// summary must not paper over trouble several levels deep.
func (r *Rows) warnedOrFailedBelow(g *DeclaredGroup) bool {
	if s := r.lens.States(g.ID); s.Failed > 0 || s.Cancelled > 0 || s.Warned > 0 {
		return true
	}
	return slices.ContainsFunc(g.Groups, r.warnedOrFailedBelow)
}

// verdictFold accumulates member states into one collection verdict. A
// Refused member counts as a Failed one does for the Tasks after the
// collection: it finished and did not succeed, so its header never reads
// Pending for it.
type verdictFold struct {
	running, failed, refused, cancelled, unresolved bool
}

// foldCounts is the verdict of a collection's direct Tasks alone.
func foldCounts(s StateCounts) verdictFold {
	return verdictFold{
		running: s.Running > 0, failed: s.Failed > 0, refused: s.Refused > 0,
		cancelled: s.Cancelled > 0, unresolved: s.Unresolved > 0,
	}
}

func (v *verdictFold) add(s record.State) {
	switch s {
	case record.Running:
		v.running = true
	case record.Failed:
		v.failed = true
	case record.Refused:
		v.refused = true
	case record.Cancelled:
		v.cancelled = true
	case record.Succeeded, record.Satisfied, record.Excluded, record.NotStarted:
	default:
		v.unresolved = true
	}
}

func (v verdictFold) state() record.State {
	switch {
	case v.running:
		return record.Running
	case v.failed:
		return record.Failed
	case v.refused:
		return record.Refused
	case v.cancelled:
		return record.Cancelled
	case v.unresolved:
		return record.Pending
	default:
		return record.Succeeded
	}
}
