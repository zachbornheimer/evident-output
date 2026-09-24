package engine

import (
	"math"
	"slices"

	"github.com/zachbornheimer/evident-output/internal/render"
)

// unownedLedgerOrder sorts a ledger section that no declared Task or
// container owns after every section that one does.
const unownedLedgerOrder = math.MaxInt

// ledgerOrderLocked is where subject's [changed]/[planned] section sits in
// the ledger: the declaration index of the Task (or container) it is named
// for. Sections print in declaration order, so the ledger reads the same on
// every run regardless of which Task finished first. Caller must hold o.mu.
func (o *Output) ledgerOrderLocked(subject string) int {
	order := unownedLedgerOrder
	for _, st := range o.tasks {
		if st.name == subject {
			order = min(order, st.declaration)
		}
	}
	for _, col := range o.tasksByRef {
		if col.name == subject {
			order = min(order, col.declaration)
		}
	}
	return order
}

// heldBackAsNoOpLocked reports whether a resolved root Task is a
// zero-information row (render.IsProvenNoOpRootTask) with no ledger section
// of its own. Such a row is not committed when it resolves: Finish decides
// whether the run has anything else to show, and prints the row only if not.
// Caller must hold o.mu.
func (o *Output) heldBackAsNoOpLocked(t TaskSnapshot) bool {
	if !render.IsProvenNoOpRootTask(render.TaskAtVerbosity(t, o.cfg.verbosity >= VerbosityVerbose)) {
		return false
	}
	named := func(subject string) bool { return subject == t.Name }
	return !slices.ContainsFunc(o.changes, func(c *changesState) bool { return named(c.subject) }) &&
		!slices.ContainsFunc(o.plans, func(p *planState) bool { return named(p.subject) })
}

// insertByLedgerOrder places section after every section whose order is
// less than or equal to its own, keeping first-declared ties in arrival
// order.
func insertByLedgerOrder[S any](sections []S, section S, orderOf func(S) int) []S {
	at := slices.IndexFunc(sections, func(existing S) bool { return orderOf(existing) > orderOf(section) })
	if at < 0 {
		return append(sections, section)
	}
	return slices.Insert(sections, at, section)
}
