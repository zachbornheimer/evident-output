package render

import (
	"slices"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// Rows are scarce (contract §13): a Task that finished with nothing to do
// and nothing to say adds a line and no information. Human output drops such
// a row when other content gives the reader something to look at.
//
// A row is zero-information when ALL of these hold:
//   - the Task is Done and resolved NoWork or AlreadySatisfied (it changed
//     nothing);
//   - it carries no information field (core.IsZeroInformationTask);
//   - no [changed]/[planned] section is named for it;
//   - the library did not invent it (a synthetic outcome row).
//
// Which zero-information rows may be hidden depends on where they sit:
//   - in a Group (independent children): NoWork and AlreadySatisfied;
//   - at the run's root: AlreadySatisfied only, because a root Task that
//     simply finishes is a landmark of the run (§13), and Evo cannot tell it
//     from a root Task that had nothing to do (both resolve NoWork);
//   - in a Sequence: never, because its rows state an order.
//
// A zero-information row is hidden only when:
//   - the run has no Failed, Blocked or Cancelled Task (a failing run keeps
//     every row so the reader sees what did and did not happen);
//   - some other content stays visible: a row that is not zero-information,
//     an effect section, a run Line, Warning or Fact. A run of nothing but
//     no-ops keeps its rows, since hiding them would leave an empty screen.
//
// Machine output never calls this; JSON and JSONL keep every Task.

// hideScope is how much of the zero-information rule applies where a Task sits.
type hideScope int

const (
	hideNothing hideScope = iota
	hideProvenNoOp
	hideAnyNoOp
)

func (scope hideScope) admits(t core.TaskSnapshot) bool {
	switch scope {
	case hideAnyNoOp:
		return core.IsZeroInformationTask(t)
	case hideProvenNoOp:
		return core.IsProvenNoOpTask(t)
	default:
		return false
	}
}

// zeroInformationScan accumulates what ZeroInformationTaskIDs learns while
// walking a snapshot's Tasks.
type zeroInformationScan struct {
	// ledgerSubjects is every [changed]/[planned] section subject, built
	// once per scan.
	ledgerSubjects map[string]bool
	candidates     map[string]bool
	visible        int
	stopped        bool
}

func (scan *zeroInformationScan) visit(t core.TaskSnapshot, scope hideScope) {
	switch t.State {
	case core.Failed, core.Blocked, core.Cancelled:
		scan.stopped = true
	}
	if scope.admits(t) && !scan.ledgerSubjects[t.Name] {
		scan.candidates[t.ID] = true
		return
	}
	scan.visible++
}

func (scan *zeroInformationScan) visitCollection(col core.TasksSnapshot, parent hideScope) {
	scope := hideAnyNoOp
	if col.Sequential || parent == hideNothing {
		scope = hideNothing
	}
	if col.Summary != "" {
		scan.visible++
	}
	for _, t := range col.Tasks {
		scan.visit(t, scope)
	}
	for _, child := range col.Collections {
		scan.visitCollection(child, scope)
	}
}

// ZeroInformationTaskIDs returns the IDs of the Tasks in s that human output
// must not render. It returns nil when nothing qualifies or when the run
// must show everything (see the rule above).
func ZeroInformationTaskIDs(s core.Snapshot) map[string]bool {
	scan := &zeroInformationScan{
		ledgerSubjects: ledgerSubjects(s),
		candidates:     make(map[string]bool, len(s.Tasks)+len(s.Collections)),
		visible:        len(s.Lines) + len(s.Warnings) + len(s.Facts) + len(s.Changes) + len(s.Plans),
	}
	for _, t := range s.Tasks {
		scan.visit(t, hideProvenNoOp)
	}
	for _, col := range s.Collections {
		scan.visitCollection(col, hideProvenNoOp)
	}
	if scan.stopped || scan.visible == 0 || len(scan.candidates) == 0 {
		return nil
	}
	return scan.candidates
}

// ledgerSubjects is the set of Task names s has a [changed] or [planned]
// section for. A qualified subject ("alpha › prune") counts for its Task
// name, so a same-named Task elsewhere stays visible rather than hidden.
func ledgerSubjects(s core.Snapshot) map[string]bool {
	subjects := make(map[string]bool, len(s.Changes)+len(s.Plans))
	for _, c := range s.Changes {
		subjects[core.SubjectTaskName(c.Subject)] = true
	}
	for _, p := range s.Plans {
		subjects[core.SubjectTaskName(p.Subject)] = true
	}
	return subjects
}

// WithoutTasks returns s with the Tasks in hidden removed from the root and
// from every collection. The input is not modified.
func WithoutTasks(s core.Snapshot, hidden map[string]bool) core.Snapshot {
	if len(hidden) == 0 {
		return s
	}
	s.Tasks = slices.DeleteFunc(slices.Clone(s.Tasks), func(t core.TaskSnapshot) bool { return hidden[t.ID] })
	s.Collections = collectionsWithoutTasks(s.Collections, hidden)
	return s
}

func collectionsWithoutTasks(cols []core.TasksSnapshot, hidden map[string]bool) []core.TasksSnapshot {
	out := make([]core.TasksSnapshot, len(cols))
	for i, col := range cols {
		col.Tasks = slices.DeleteFunc(slices.Clone(col.Tasks), func(t core.TaskSnapshot) bool { return hidden[t.ID] })
		col.Collections = collectionsWithoutTasks(col.Collections, hidden)
		out[i] = col
	}
	return out
}

// HumanProjection is s as a human reader should see it at this verbosity:
// hidden Facts dropped (SnapshotAtVerbosity), then zero-information Tasks
// removed. Machine projections take the snapshot as it is.
func HumanProjection(s core.Snapshot, verbose bool) core.Snapshot {
	s = SnapshotAtVerbosity(s, verbose)
	return WithoutTasks(s, ZeroInformationTaskIDs(s))
}
