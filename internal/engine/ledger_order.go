package engine

import (
	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/render"
)

// appendTaskLocked adds st to the run's Tasks in declaration order. Caller
// must hold o.mu.
func (o *Output) appendTaskLocked(st *taskState) {
	o.tasks = append(o.tasks, st)
	if st.collection() == nil {
		o.rootTasks = append(o.rootTasks, st)
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
	return !o.rec.HasLedgerSection(t.ID)
}
