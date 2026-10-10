package engine

import "time"

// liveIndexAudit, when set, checks every live frame a collection builds
// from its childIndex against the frame a walk of every child builds. The
// tests set it, so each live test of the engine also proves the index
// current at every mutation site.
//
// rev is the record's revision before the frame was built: an audit that
// finds the record written since compares two different runs of it, and
// skips.
var liveIndexAudit func(g *tasksState, rows int, now time.Time, got TasksSnapshot, rev uint64)
