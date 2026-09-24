package engine

import (
	"math"
	"slices"

	"github.com/zachbornheimer/evident-output/internal/render"
)

// unownedLedgerOrder sorts a ledger section that no declared Task or
// container owns after every section that one does.
const unownedLedgerOrder = math.MaxInt

// ledgerIndex answers the two questions ledger sections ask by name —
// "which declaration is this subject named for?" and "does this subject
// have a section?" — in O(1), maintained as Tasks, containers, and
// sections are declared. Guarded by o.mu.
type ledgerIndex struct {
	firstDeclared map[string]int
	sections      map[string]bool
}

// declared records that a Task or container named name was declared at
// declaration; the earliest declaration of a name wins.
func (x *ledgerIndex) declared(name string, declaration int) {
	if x.firstDeclared == nil {
		x.firstDeclared = map[string]int{}
	}
	if prev, ok := x.firstDeclared[name]; !ok || declaration < prev {
		x.firstDeclared[name] = declaration
	}
}

// sectionOpened records that a [changed] or [planned] section exists for
// subject.
func (x *ledgerIndex) sectionOpened(subject string) {
	if x.sections == nil {
		x.sections = map[string]bool{}
	}
	x.sections[subject] = true
}

// appendTaskLocked adds st to the run's Tasks in declaration order and
// indexes its name for ledger ordering. Caller must hold o.mu.
func (o *Output) appendTaskLocked(st *taskState) {
	o.tasks = append(o.tasks, st)
	o.ledger.declared(st.name, st.declaration)
}

// ledgerOrderLocked is where subject's [changed]/[planned] section sits in
// the ledger: the declaration index of the Task (or container) it is named
// for. Sections print in declaration order, so the ledger reads the same on
// every run regardless of which Task finished first. Caller must hold o.mu.
func (o *Output) ledgerOrderLocked(subject string) int {
	if order, ok := o.ledger.firstDeclared[subject]; ok {
		return order
	}
	return unownedLedgerOrder
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
	return !o.ledger.sections[t.Name]
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
