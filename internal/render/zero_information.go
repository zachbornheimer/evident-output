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
//   - it carries no Summary, Problem, Warning, Fact, disposition record,
//     Action or verification detail, and no in-flight Progress or Phase;
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

// IsZeroInformationTask reports whether t alone, ignoring its ledger and its
// neighbours, would add nothing to a human reader (see the rule above).
func IsZeroInformationTask(t core.TaskSnapshot) bool {
	if t.State != core.Done || t.Synthetic() {
		return false
	}
	if t.Resolution != core.ResolutionNoWork && t.Resolution != core.ResolutionAlreadySatisfied {
		return false
	}
	return t.Summary == "" && t.Phase == "" && t.Progress.Total == 0 && t.Progress.Completed == 0 &&
		len(t.Problems) == 0 && len(t.Warnings) == 0 && len(t.Facts) == 0 &&
		len(t.Skipped) == 0 && len(t.Kept) == 0 && len(t.Actions) == 0 &&
		len(t.Verification) == 0
}

// IsProvenNoOpRootTask reports whether t is a zero-information row that may
// be hidden at the run's root: one whose no-op was proven by Verify.
func IsProvenNoOpRootTask(t core.TaskSnapshot) bool {
	return IsZeroInformationTask(t) && t.Resolution == core.ResolutionAlreadySatisfied
}

func (scope hideScope) admits(t core.TaskSnapshot) bool {
	switch scope {
	case hideAnyNoOp:
		return IsZeroInformationTask(t)
	case hideProvenNoOp:
		return IsProvenNoOpRootTask(t)
	default:
		return false
	}
}

// zeroInformationScan accumulates what ZeroInformationTaskIDs learns while
// walking a snapshot's Tasks.
type zeroInformationScan struct {
	snapshot   core.Snapshot
	candidates map[string]bool
	visible    int
	stopped    bool
}

func (scan *zeroInformationScan) visit(t core.TaskSnapshot, scope hideScope) {
	switch t.State {
	case core.Failed, core.Blocked, core.Cancelled:
		scan.stopped = true
	}
	if scope.admits(t) && !hasLedgerSection(scan.snapshot, t.Name) {
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
		snapshot:   s,
		candidates: make(map[string]bool, len(s.Tasks)+len(s.Collections)),
		visible:    len(s.Lines) + len(s.Warnings) + len(s.Facts) + len(s.Changes) + len(s.Plans),
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

func hasLedgerSection(s core.Snapshot, subject string) bool {
	return slices.ContainsFunc(s.Changes, func(c core.ChangesSnapshot) bool { return c.Subject == subject }) ||
		slices.ContainsFunc(s.Plans, func(p core.PlanSnapshot) bool { return p.Subject == subject })
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
