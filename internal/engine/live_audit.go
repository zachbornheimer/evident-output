package engine

import "time"

// liveIndexAudit, when set, checks every live frame a collection builds
// from its childIndex against the frame a walk of every child builds. The
// tests set it, so each live test of the engine also proves the index
// current at every mutation site.
var liveIndexAudit func(g *tasksState, rows int, now time.Time, got TasksSnapshot)
