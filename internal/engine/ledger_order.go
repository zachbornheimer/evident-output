package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/ordered"
	"github.com/zachbornheimer/evident-output/internal/render"
)

// ledgerIndex finds the run's ledger sections by owner and by the name they
// are shown under, in O(1). Guarded by o.mu.
type ledgerIndex struct {
	byOwner map[ledgerSectionKey]*ledgerSection
	byName  map[string][]*ledgerSection
}

// opened indexes a newly opened section.
func (x *ledgerIndex) opened(key ledgerSectionKey, s *ledgerSection) {
	if x.byOwner == nil {
		x.byOwner = map[ledgerSectionKey]*ledgerSection{}
		x.byName = map[string][]*ledgerSection{}
	}
	x.byOwner[key] = s
	x.byName[s.owner.name] = append(x.byName[s.owner.name], s)
}

// appendTaskLocked adds st to the run's Tasks in declaration order. Caller
// must hold o.mu.
func (o *Output) appendTaskLocked(st *taskState) {
	o.tasks = append(o.tasks, st)
	if st.collection == nil {
		o.rootColumn.add(st.name)
	}
}

// heldBackAsNoOpLocked reports whether a resolved root Task is a
// zero-information row (core.IsProvenNoOpTask) with no ledger section
// of its own. Such a row is not committed when it resolves: Finish decides
// whether the run has anything else to show, and prints the row only if not.
// Caller must hold o.mu.
func (o *Output) heldBackAsNoOpLocked(t TaskSnapshot) bool {
	if !core.IsProvenNoOpTask(render.TaskAtVerbosity(t, o.cfg.verbosity >= VerbosityVerbose)) {
		return false
	}
	return !o.hasLedgerSectionLocked(t.ID)
}

// insertByLedgerOrder places section after every section whose order is
// less than or equal to its own, keeping ties in arrival order.
func insertByLedgerOrder(sections []*ledgerSection, section *ledgerSection) []*ledgerSection {
	return ordered.Insert(sections, section, (*ledgerSection).order)
}
