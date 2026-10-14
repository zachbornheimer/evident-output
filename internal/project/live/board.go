package live

import (
	"time"
	"unicode/utf8"

	"github.com/zachbornheimer/evident-output/internal/project"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// Board is what a live frame and a collection's verdict read of one run's
// declared Tasks and collections: a row per Task, keyed by record.TaskID,
// and a node per Group or Sequence, keyed by its id. Each node keeps the
// census and child index of what is below it current as the record
// changes, so a frame costs what the screen shows, not what the run holds.
//
// A Board is not safe for concurrent use: the run's lock guards it.
type Board struct {
	run    *record.Run
	source Source

	// tasks are every Task with a row, in declaration order; roots the
	// ones outside every collection; groups the collections at the root.
	tasks  []*row
	roots  []*row
	groups []*group

	rows  map[record.TaskID]*row
	nodes map[string]*group

	// armed is set once the run promised a paint before any entity
	// existed (Arm).
	armed bool

	// audit, when set, checks every frame a collection builds from its
	// child index against the frame a walk of every child builds. rev is
	// the record's revision before the frame was built.
	audit func(g *group, rows int, now time.Time, got record.TasksSnapshot, rev uint64)
}

// Source is what a Board reads of the run around it, under the run's lock.
// A Task's first paint and live tail belong to the Board; the snapshots a
// Source returns carry them because it asks the Board (FirstSeen, Tail).
type Source interface {
	// Pull follows the record: it calls Board.Follow for every Task written
	// since the last pull.
	Pull()
	// View is the Task's snapshot sharing its slices; Snapshot an owned copy.
	View(id record.TaskID) record.TaskSnapshot
	Snapshot(id record.TaskID) record.TaskSnapshot
	// Header is the collection's row without its children.
	Header(id string) record.TasksSnapshot
	// Emitted reports a root Task whose row is already in scrollback.
	Emitted(id record.TaskID) bool
	// HeldBack reports a resolved root Task whose row waits for Finish to
	// decide whether it shows at all.
	HeldBack(view *record.TaskSnapshot) bool
}

// row is one Task as the live region keeps it.
type row struct {
	id    record.TaskID
	name  string
	rec   *record.Task
	group *group

	// seen is the domain-clock time a frame first painted the Task — the
	// one elapsed-time anchor every row's heartbeat suffix reads (P5),
	// including a Pending Task, which never calls Phase or Progress.
	seen time.Time

	// followed is the state the census counts the Task under; a later
	// difference from the record's state is what Follow moves.
	followed record.State

	// filing is where the Task stands in its collection's child index.
	filing filing

	// tail is the bounded window of Writer lines a frame draws beneath
	// this row.
	tail tail
}

// group is one Group or Sequence as the live region keeps it.
type group struct {
	board  *Board
	id     string
	name   string
	parent *group

	// tasks and groups are the members declared directly, in order.
	tasks  []*row
	groups []*group

	// census counts every Task at or below the collection; kids what a
	// frame and the verdict read of its direct Tasks.
	census census
	kids   childIndex

	// namesake records that a child Task carries the collection's own
	// name, the only way it can render as its own Task (ownRow).
	namesake bool
}

// declaredState is the state a Task is declared in, and so the state the
// census first counts it under.
const declaredState = record.Pending

// NewBoard starts the live model of run, reading the run around it
// through source.
func NewBoard(run *record.Run, source Source) *Board {
	return &Board{
		run: run, source: source,
		rows:  make(map[record.TaskID]*row),
		nodes: make(map[string]*group),
	}
}

// DeclareGroup adds the collection id named name under the collection
// parent, or at the root when parent is "".
func (b *Board) DeclareGroup(id, name, parent string) {
	g := &group{board: b, id: id, name: name, parent: b.nodes[parent]}
	b.nodes[id] = g
	if g.parent == nil {
		b.groups = append(b.groups, g)
		return
	}
	g.parent.groups = append(g.parent.groups, g)
}

// DeclareTask adds the Task id named name, whose truth is rec, under the
// collection in, or at the root when in is "".
func (b *Board) DeclareTask(id record.TaskID, name string, rec *record.Task, in string) {
	r := &row{id: id, name: name, rec: rec, group: b.nodes[in], followed: declaredState}
	b.rows[id] = r
	b.tasks = append(b.tasks, r)
	g := r.group
	if g == nil {
		b.roots = append(b.roots, r)
		return
	}
	r.filing.pos = len(g.tasks)
	g.tasks = append(g.tasks, r)
	r.markFiling()
	g.namesake = g.namesake || name == g.name
	r.censusDeclared()
}

// Follow moves the Task id from the state the Board counts it under to the
// state the record holds now, and marks it for refiling. It reports the
// state it now counts, and false for a Task with no row.
func (b *Board) Follow(id record.TaskID) (record.State, bool) {
	r := b.rows[id]
	if r == nil {
		return "", false
	}
	r.markFiling()
	r.censusSync()
	return r.followed, true
}

// Changed marks the Task id for refiling: something a frame shows or a
// verdict counts of it changed outside the record.
func (b *Board) Changed(id record.TaskID) {
	if r := b.rows[id]; r != nil {
		r.markFiling()
	}
}

// Warned records the Task id's first warning.
func (b *Board) Warned(id record.TaskID) {
	if r := b.rows[id]; r != nil {
		r.censusesAbove(func(c *census) { c.warned++ })
	}
}

// FirstSeen is when a frame first painted the Task id, or the zero time.
func (b *Board) FirstSeen(id record.TaskID) time.Time {
	if r := b.rows[id]; r != nil {
		return r.seen
	}
	return time.Time{}
}

// Tail is the Task id's live tail as a snapshot carries it, sharing its
// lines.
func (b *Board) Tail(id record.TaskID) record.LiveTail {
	if r := b.rows[id]; r != nil {
		return r.tail.view()
	}
	return record.LiveTail{}
}

// PushTail records one completed Writer line of the Task id: retained is
// the Capture ring's line count, at the domain-clock time it arrived.
func (b *Board) PushTail(id record.TaskID, line string, retained int, at time.Time) {
	if r := b.rows[id]; r != nil {
		r.tail.push(line, retained, at)
	}
}

// States counts the collection id's direct Tasks by the states its
// verdict folds.
func (b *Board) States(id string) StateCounts {
	if g := b.nodes[id]; g != nil {
		return g.settled().states
	}
	return StateCounts{}
}

// Arm promises a paint before any entity exists, and reports whether this
// call made the promise.
func (b *Board) Arm() bool {
	if b.armed {
		return false
	}
	b.armed = true
	return true
}

// Rows is how many Tasks have a row.
func (b *Board) Rows() int { return len(b.tasks) }

// ArmedTitle reports the title-only row Arm promised: armed, and nothing
// declared yet. Active and Animating agree on it, or the header spinner is
// shown and then frozen.
func (b *Board) ArmedTitle() bool {
	return b.armed && len(b.tasks) == 0 && len(b.groups) == 0
}

// Active reports whether anything should be on screen: a Task Running,
// Pending or narrating a phase, determinate progress to show, or the armed
// title. Pending counts: a declared Task renders its named "○" row at once;
// VisibilityDelay withholds the spinner flash of near-instant work, not the
// fact that a Task now exists.
func (b *Board) Active() bool {
	for _, r := range b.tasks {
		truth := r.rec.Truth()
		state := truth.State
		if state == record.Running || state == record.Pending || truth.Phase != "" {
			return true
		}
		if kind := truth.Progress.Kind; kind == record.Determinate || kind == record.BytesKind {
			if state == record.Running || state.Succeeded() || state == record.Failed {
				return true
			}
		}
	}
	return b.ArmedTitle()
}

// UnpaintedRunning reports a Running Task no frame has painted yet.
func (b *Board) UnpaintedRunning() bool {
	for _, r := range b.tasks {
		if r.seen.IsZero() && r.rec.State() == record.Running {
			return true
		}
	}
	return false
}

// Animating reports whether a live row should keep ticking: the armed
// title, or a Pending or Running Task still rendered in the current frame
// — not a root Task already in scrollback (see Frame's matching filter).
// Counting only Running froze an all-Pending frame forever (evo-rec.md
// Problem 9).
func (b *Board) Animating() bool {
	if b.ArmedTitle() {
		return true
	}
	for _, g := range b.groups {
		if c := g.currentCensus(); c.running+c.pending > 0 {
			return true
		}
	}
	for _, r := range b.roots {
		if state := r.rec.State(); (state == record.Running || state == record.Pending) && !b.source.Emitted(r.id) {
			return true
		}
	}
	return false
}

// Frame is what a live frame of rows rows draws: the effect sections, and
// every collection and root Task as far as the frame could show it
// (LiveChildren), so building it snapshots the rows on screen rather than
// every Task in the run. A root Task already in scrollback, or held back
// for Finish, is left out: it would otherwise reappear on the next
// unrelated redraw and print twice.
func (b *Board) Frame(rows int, now time.Time) record.Snapshot {
	var s record.Snapshot
	s.Changes = b.run.ChangeSnapshots()
	s.Plans = b.run.PlanSnapshots()
	cols := b.collections(b.groups, rows, now)
	s.Collections = cols.Kept()
	s = project.WithRootCollectionTally(s, cols.Tally())
	root := NewLiveChildren("", rows)
	for _, r := range b.roots {
		r.stamp(now)
		view := b.source.View(r.id)
		if b.source.Emitted(r.id) || b.source.HeldBack(&view) {
			continue
		}
		if root.Admit(&view) {
			root.Keep(b.source.Snapshot(r.id))
		}
	}
	tasks := root.Collection(record.TasksSnapshot{})
	s.Tasks = tasks.Tasks
	if tally, ok := project.ChildTallyOf(tasks); ok {
		s = project.WithRootTally(s, tally)
	}
	return s
}

// collections projects groups for a frame of rows rows: the ones the frame
// can reach, the rest tallied from their censuses (LiveCollections).
func (b *Board) collections(groups []*group, rows int, now time.Time) *LiveCollections {
	projected := NewLiveCollections(rows)
	for _, g := range groups {
		if c := g.currentCensus(); projected.Admit(c.rank()) {
			projected.Keep(g.frame(rows, now))
			continue
		}
		g.stamp(now)
		projected.Omit(g.census.counts(), g.ownRow())
	}
	return projected
}

// frame is g as a frame of rows rows can show it: the children it could
// select, snapshotted, and a tally of the rest.
func (g *group) frame(rows int, now time.Time) record.TasksSnapshot {
	b := g.board
	rev := b.run.Revision()
	g.stampDirect(now)
	ts := b.source.Header(g.id)
	kept, roster := g.settled().project(g, rows)
	ts = b.collections(g.groups, rows, now).Into(ts)
	ts = roster.Project(ts, kept)
	if b.audit != nil {
		b.audit(g, rows, now, ts, rev)
	}
	return ts
}

// view is g's header and its Tasks' views, recursively, without the cost
// of a snapshot.
func (g *group) view() record.TasksSnapshot {
	ts := g.board.source.Header(g.id)
	for _, r := range g.tasks {
		ts.Tasks = append(ts.Tasks, g.board.source.View(r.id))
	}
	for _, child := range g.groups {
		ts.Collections = append(ts.Collections, child.view())
	}
	return ts
}

// ownRow is the name g's one row renders under when it renders as its own
// Task, or "". Only a Group holding a Task of its own name can, so every
// other Group answers without being walked.
func (g *group) ownRow() string {
	if !g.namesake {
		return ""
	}
	return OwnRowName(g.view())
}

// stamp anchors an unresolved Task's heartbeat clock to the moment a frame
// first paints it — never to declaration, so a Task declared up front but
// not yet visible does not appear pre-aged the instant it is finally shown
// (evo-rec.md Problem 9). A no-op once set.
func (r *row) stamp(now time.Time) {
	if r.unstampedIn(r.rec.State()) {
		r.seen = now
		r.markFiling()
		r.censusStamped()
	}
}

// stamp stamps every Task at or below g, as a frame that counted them does.
func (g *group) stamp(now time.Time) {
	if g.currentCensus().unstamped == 0 {
		return
	}
	g.stampDirect(now)
	for _, child := range g.groups {
		child.stamp(now)
	}
}

// unstampedIn reports whether r, in state, is a live row no frame painted.
func (r *row) unstampedIn(state record.State) bool {
	return (state == record.Running || state == record.Pending) && r.seen.IsZero()
}

// width is r's name width, in runes.
func (r *row) width() int { return utf8.RuneCountInString(r.name) }

// runningSince reports whether the Task id is Running, and when a frame
// first painted it.
func (b *Board) runningSince(id record.TaskID) (time.Time, bool) {
	r := b.rows[id]
	if r == nil || r.rec.State() != record.Running {
		return time.Time{}, false
	}
	return r.seen, true
}
