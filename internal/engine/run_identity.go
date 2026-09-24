package engine

import "crypto/rand"

// runIDPrefix marks a random run identity on the wire ("run_id", spec
// §16/§35).
const runIDPrefix = "run_"

// unembeddedRunID is the identity every 1.1 run carried, and the one a run
// that neither pins Config.RunID nor sets Config.Embedded still carries:
// a 1.1 golden test that pinned "out_1" stays byte-stable (DEC-CANCEL-005).
const unembeddedRunID = "out_1"

// runIDSeqSlot is the id sequence value the run itself consumed through
// 1.1, when "out_1" was drawn from the same sequence as every other id.
// Starting past it keeps every Task, Group, and message id on the wire
// where consumers found it (the first Task stays "task_2") whatever the
// run's identity is.
const runIDSeqSlot = 1

// issueRunID is the run's identity: the one Config.RunID pinned, a fresh
// random one for an Embedded run, or the 1.1 constant otherwise. Embedded
// runs are random, not a per-process counter: concurrent requests served
// by one process (spec §53, one Output per HTTP request) must never share
// a run_id a machine consumer correlates on.
func (c *config) issueRunID() string {
	switch {
	case c.runID != "":
		return c.runID
	case c.embedded:
		return runIDPrefix + rand.Text()
	default:
		return unembeddedRunID
	}
}
