// This file owns Plan: what a planned Run has left unapplied, so work that
// reads it is judged uncertain rather than current.

package freshness

import "path/filepath"

// Plan is how a Run judges an operation's freshness: whether it only plans
// its work and, when it does, which paths the operations it planned earlier
// would change. The zero Plan is a Run that applies its work.
type Plan struct{ table *TaskTable }

// Plans reports whether the Run only plans its work.
func (p Plan) Plans() bool { return p.table != nil }

// Changes reports whether an operation planned earlier in this Run would
// change a path basis observed. The bytes observed now are then not the
// bytes the work would read, so it is uncertain and never current
// (CONTRACT.md §3).
func (p Plan) Changes(basis []BasisRecord) bool {
	return p.table != nil && p.table.plansChange(basis)
}

// plannedPaths are the paths a planned Run's operations would change, each
// with every directory above it, so a Basis on a directory sees a change
// inside it.
type plannedPaths map[string]struct{}

func (s plannedPaths) add(path string) {
	for {
		if _, seen := s[path]; seen {
			return
		}
		s[path] = struct{}{}
		parent := filepath.Dir(path)
		if parent == path {
			return
		}
		path = parent
	}
}

func (s plannedPaths) changesAny(basis []BasisRecord) bool {
	for _, b := range basis {
		if b.Kind != string(KindFSPath) {
			continue
		}
		if _, changed := s[b.Key]; changed {
			return true
		}
	}
	return false
}
