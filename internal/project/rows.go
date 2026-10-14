package project

import (
	"slices"
	"time"

	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// Rows is the shared row model: every declared Task and collection as each
// projection shows it — identity, the record it reads, its timing and where
// it sits — in declaration order, keyed by record.TaskID and by the
// collection's id. Snapshots, views and collection headers are built here,
// so the live frame, the plain stream and the wire read one model.
//
// Rows is not safe for concurrent use: the run's lock guards it.
type Rows struct {
	lens Lens

	// tasks are every Task with a row, in declaration order; roots the ones
	// outside every collection; groups the collections at the root.
	tasks  []*DeclaredTask
	roots  []*DeclaredTask
	groups []*DeclaredGroup

	byTask  map[record.TaskID]*DeclaredTask
	byGroup map[string]*DeclaredGroup
}

// Lens is what other owners know of a row: the live board its first paint,
// tail and verdict counts, freshness what its operations tracked.
type Lens interface {
	FirstSeen(id record.TaskID) time.Time
	Tail(id record.TaskID) record.LiveTail
	States(group string) StateCounts
	Tracked(id record.TaskID) record.TaskOperations
}

// DeclaredTask is one Task as every projection shows it.
type DeclaredTask struct {
	ID record.TaskID
	// Node is the declaration that owns the Task's stable key, which a key
	// override may change after the row exists.
	Node        Node
	Name        string
	Declaration int
	Rec         *record.Task
	// Synthetic marks the Task the library invented to carry the run's
	// own ending: its conclusion band is never suppressed as a repeat.
	Synthetic bool
	// Timing is when the Task was declared, began running and settled.
	Timing record.TaskTiming
	// Group is the collection the Task sits in, nil at the root.
	Group *DeclaredGroup
}

// DeclaredGroup is one Group or Sequence as every projection shows it.
type DeclaredGroup struct {
	ID, Name    string
	Declaration int
	Sequential  bool
	Rec         *record.Container
	// Node is the collection's declaration: its stable key, and whether its
	// builder failed or never started.
	Node   GroupNode
	Parent *DeclaredGroup
	// Tasks and Groups are the members declared directly, in order.
	Tasks  []*DeclaredTask
	Groups []*DeclaredGroup
}

// Node is a declaration as a row reads it: its §3.1 stable key.
type Node interface{ Key() string }

// GroupNode is a collection's declaration as its row and verdict read it.
type GroupNode interface {
	Node
	HasBuilder() bool
	BuilderFailed() bool
	BuilderNotStarted() bool
	// MembershipClosed reports that nothing more is expected to be
	// declared into the collection (graph.Container.MembershipClosed).
	MembershipClosed() bool
}

// StateCounts counts a collection's direct Tasks by the states its verdict
// folds.
type StateCounts struct {
	Running, Failed, Refused, Cancelled, NotStarted, Unresolved, Warned int
}

// NewRows starts an empty row model reading other owners through lens.
func NewRows(lens Lens) *Rows {
	return &Rows{lens: lens, byTask: make(map[record.TaskID]*DeclaredTask), byGroup: make(map[string]*DeclaredGroup)}
}

// DeclareGroup adds g under the collection parent, or at the root when
// parent is "", and returns its row.
func (r *Rows) DeclareGroup(g DeclaredGroup, parent string) *DeclaredGroup {
	row := &g
	row.Parent = r.byGroup[parent]
	r.byGroup[row.ID] = row
	if row.Parent == nil {
		r.groups = append(r.groups, row)
	} else {
		row.Parent.Groups = append(row.Parent.Groups, row)
	}
	return row
}

// DeclareTask adds t under the collection in, or at the root when in is "",
// and returns its row.
func (r *Rows) DeclareTask(t DeclaredTask, in string) *DeclaredTask {
	row := &t
	row.Group = r.byGroup[in]
	r.byTask[row.ID] = row
	r.tasks = append(r.tasks, row)
	if row.Group == nil {
		r.roots = append(r.roots, row)
	} else {
		row.Group.Tasks = append(row.Group.Tasks, row)
	}
	return row
}

// Task is the row of the Task id, or nil.
func (r *Rows) Task(id record.TaskID) *DeclaredTask { return r.byTask[id] }

// Group is the row of the collection id, or nil.
func (r *Rows) Group(id string) *DeclaredGroup { return r.byGroup[id] }

// Count is how many Tasks have a row.
func (r *Rows) Count() int { return len(r.tasks) }

// View is the Task id's snapshot sharing its record's slices: read it under
// the run's lock and never let it escape. It costs no allocation, so a live
// frame can classify every Task by it and copy only the ones it shows.
func (r *Rows) View(id record.TaskID) record.TaskSnapshot {
	t := r.byTask[id]
	truth := t.Rec.Truth()
	group := ""
	if t.Group != nil {
		group = t.Group.ID
	}
	base := record.TaskSnapshot{
		ID: string(t.ID), Key: t.Node.Key(), Name: t.Name,
		State: truth.State, Phase: truth.Phase, ActivityAt: truth.ActivityAt, Progress: truth.Progress,
		Summary: truth.Summary, Problems: truth.Problems, Warnings: truth.Warnings, Facts: truth.Facts,
		Verification: truth.Verification, Actions: truth.Actions, Skipped: truth.Skipped, Kept: truth.Kept,
		Collection: group, Declaration: t.Declaration, DefineRan: truth.DefineRan, Evidence: truth.VerifyEvidence,
		Timing: t.Timing,
	}
	return record.WithLiveTail(record.NewTaskSnapshot(base, r.lens.FirstSeen(id), t.Synthetic), r.lens.Tail(id))
}

// Snapshot is the Task id's snapshot, owning every slice it holds.
func (r *Rows) Snapshot(id record.TaskID) record.TaskSnapshot {
	s := r.View(id)
	s.Problems = record.CloneProblems(s.Problems)
	s.Warnings = record.CloneProblems(s.Warnings)
	s.Facts = record.CloneFacts(s.Facts)
	s.Verification = record.CloneVerificationDetails(s.Verification)
	s.Actions = record.CloneActions(s.Actions)
	s.Skipped = cloneTaxonomy(s.Skipped)
	s.Kept = cloneTaxonomy(s.Kept)
	if !r.byTask[id].Synthetic {
		s.Tracked = r.lens.Tracked(id)
	}
	tail := record.LiveTailOf(s)
	tail.Lines = slices.Clone(tail.Lines)
	return record.WithLiveTail(s, tail)
}

func cloneTaxonomy(in []record.TaxonomyRecord) []record.TaxonomyRecord {
	if len(in) == 0 {
		return nil
	}
	out := slices.Clone(in)
	for i := range out {
		out[i].Causes = slices.Clone(out[i].Causes)
	}
	return out
}

// Header is the collection id's row without its members.
func (r *Rows) Header(id string) record.TasksSnapshot {
	g := r.byGroup[id]
	state := r.verdict(g)
	return record.TasksSnapshot{
		ID: g.ID, Key: g.Node.Key(), Name: g.Name, State: state,
		Summary: r.summary(g, state), Declaration: g.Declaration, Sequential: g.Sequential, Built: g.Node.HasBuilder(),
	}
}

// Collection is the collection id's header and every member's snapshot,
// recursively.
func (r *Rows) Collection(id string) record.TasksSnapshot {
	g := r.byGroup[id]
	ts := r.Header(id)
	ts.Tasks = slices.Grow(ts.Tasks, len(g.Tasks))
	for _, t := range g.Tasks {
		ts.Tasks = append(ts.Tasks, r.Snapshot(t.ID))
	}
	ts.Collections = slices.Grow(ts.Collections, len(g.Groups))
	for _, child := range g.Groups {
		ts.Collections = append(ts.Collections, r.Collection(child.ID))
	}
	return ts
}

// Into fills s with every root Task and collection, snapshotted.
func (r *Rows) Into(s record.Snapshot) record.Snapshot {
	for _, g := range r.groups {
		s.Collections = append(s.Collections, r.Collection(g.ID))
	}
	for _, t := range r.roots {
		s.Tasks = append(s.Tasks, r.Snapshot(t.ID))
	}
	return s
}

// Actions are every Task's next steps, each once, in declaration order: a
// Task's own, then each of its Problems' and warnings', so a remedy attached
// to one of several Problems reaches the run's Next steps too (ZYS-848).
func (r *Rows) Actions() []record.Action {
	seen := map[string]bool{}
	var out []record.Action
	add := func(list []record.Action) {
		for _, a := range list {
			if k := record.ActionKey(a); !seen[k] {
				seen[k] = true
				out = append(out, a)
			}
		}
	}
	for _, t := range r.tasks {
		truth := t.Rec.Truth()
		add(truth.Actions)
		for _, p := range truth.Problems {
			add(p.Actions)
		}
		for _, w := range truth.Warnings {
			add(w.Actions)
		}
	}
	return record.CloneActions(out)
}
