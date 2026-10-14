package evo

import (
	"container/heap"
	"context"
	"fmt"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/project"
	"github.com/zachbornheimer/evident-output/internal/project/live"
	"github.com/zachbornheimer/evident-output/internal/project/wire"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// Task is a handle for one operation with phases or progress.
type Task struct {
	out *Run
	id  string
	// rejected is why the declaration was refused (see rejectedTask); nil
	// for a declared Task.
	rejected error
}

// succeed resolves the task Done with summary: the engine's synchronous
// success verb for library-owned rows (Confirm's gate) and engine tests.
// The public Task has no equivalent since 1.1: callers resolve through
// Define, and Summary carries their result.
func (t *Task) succeed(summary string) {
	t.finish(succeeded, project.Text(summary), nil)
}

// resolveWithProblem is Fail and Block: resolve as state with one Problem
// built from summary and options.
func (t *Task) resolveWithProblem(state entityState, summary string, options []ProblemOption) {
	p := applyProblemOptions(project.Text(summary), options)
	t.finish(state, project.Text(summary), []problem{p})
}

// block resolves the task as refused. A nil *Task is safe and resolves
// nothing.
func (t *Task) block(summary string, options ...ProblemOption) {
	t.resolveWithProblem(refused, summary, options)
}

// cancel resolves the task as cancelled. A nil *Task is safe and resolves
// nothing.
func (t *Task) cancel(reason string) {
	t.finish(cancelled, project.Text(reason), nil)
}

// finish is the caller-facing resolution path behind every terminal verb a
// program writes by hand; see graph.Graph.Resolve and graph.Graph.Admit.
// Once a task is submitted, a success the caller asks for is a proposal the
// callback's return ratifies; bad news resolves at once.
func (t *Task) finish(state entityState, summary string, problems []problem) *Task {
	if t == nil || t.out == nil {
		return t
	}
	o := t.out
	o.mu.Lock()
	st := o.taskStates[t.id]
	o.mu.Unlock()
	if st != nil {
		o.graph.Resolve(st.node, graph.Resolution{State: state, Summary: summary, Problems: problems, By: graph.ByCaller})
	}
	return t
}

// Predecessor is a handle After accepts: a *Task, *Group, *Sequence or
// *Computed[T]. It is sealed, so nothing else can be passed to After.
type Predecessor interface {
	predecessor()
}

func (*Task) predecessor()     {}
func (*Group) predecessor()    {}
func (*Sequence) predecessor() {}

// After declares predecessors: this Task starts only once every one of
// them succeeded, and never starts once one of them cannot.
func (t *Task) After(preds ...Predecessor) *Task {
	if t == nil || t.out == nil {
		return t
	}
	o := t.out
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[t.id]
	if st == nil {
		return t
	}
	o.graph.After(st.node, o.predecessorsOfLocked(preds)...)
	return t
}

// predecessorsOfLocked resolves After's arguments, leaving out the ones
// After ignores.
func (o *Run) predecessorsOfLocked(args []Predecessor) []graph.Predecessor {
	preds := make([]graph.Predecessor, 0, len(args))
	for _, arg := range args {
		if pred, ok := o.predecessorOfLocked(arg); ok {
			preds = append(preds, pred)
		}
	}
	return preds
}

// predecessorOfLocked resolves one After argument. ok is false for a nil
// handle, which After ignores. A handle this Output never declared yields
// the empty predecessor, which never succeeds.
func (o *Run) predecessorOfLocked(p Predecessor) (pred graph.Predecessor, ok bool) {
	switch x := p.(type) {
	case *Task:
		if x == nil || x.id == "" {
			return graph.Predecessor{}, false
		}
		if x.out == o {
			return graph.AfterTask(o.nodeOfTask(x.id)), true
		}
	case *Group:
		if x == nil || x.id == "" {
			return graph.Predecessor{}, false
		}
		if x.out == o {
			return graph.AfterContainer(o.nodeOfContainer(x.id)), true
		}
	case *Sequence:
		if x == nil || x.tasks == nil {
			return graph.Predecessor{}, false
		}
		return o.predecessorOfLocked(x.tasks)
	case interface{ producer() *Task }:
		if x == nil {
			return graph.Predecessor{}, false
		}
		return o.predecessorOfLocked(x.producer())
	default:
		return graph.Predecessor{}, false
	}
	return graph.Predecessor{}, true
}

// nodeOfTask is the graph's declaration of the Task named id, nil when this
// Output declared none.
func (o *Run) nodeOfTask(id string) *graph.Task {
	if st := o.taskStates[id]; st != nil {
		return st.node
	}
	return nil
}

// nodeOfContainer is the graph's declaration of the container named id, nil
// when this Output declared none.
func (o *Run) nodeOfContainer(id string) *graph.Container {
	if st := o.containerStates[id]; st != nil {
		return st.node
	}
	return nil
}

// After declares predecessors: this Group declares its children only once
// every one of them succeeded, and never does once one of them cannot. A
// predecessor may be a Task, a Computed producer, or a Group or Sequence.
// Call it before Define and before declaring any child; later is misuse.
func (g *Group) After(preds ...Predecessor) *Group {
	if g == nil || g.out == nil {
		return g
	}
	o := g.out
	o.mu.Lock()
	defer o.mu.Unlock()
	col := o.containerStates[g.id]
	if col == nil {
		return g
	}
	o.graph.ContainerAfter(col.node, o.predecessorsOfLocked(preds)...)
	return g
}

// After is Group.After for a Sequence.
func (s *Sequence) After(preds ...Predecessor) *Sequence {
	if s == nil || s.tasks == nil {
		return s
	}
	s.tasks.After(preds...)
	return s
}

// Define defers this Group's children: build runs once, when every After
// predecessor succeeded, and declares the children through the handle it
// receives. build is topology only. It returns nothing, takes no context,
// and must not Wait. Children it declares run under the Group's normal
// semantics. When a predecessor fails, build never runs and the Group
// settles NotStarted.
func (g *Group) define(build func(*Group)) {
	if build == nil {
		g.recordInvalid()
		return
	}
	g.defineBuilder(func() { build(g) })
}

// Define is Group.Define for a Sequence: build declares ordered steps.
func (s *Sequence) define(build func(*Sequence)) {
	if s == nil || s.tasks == nil {
		return
	}
	if build == nil {
		s.tasks.recordInvalid()
		return
	}
	s.tasks.defineBuilder(func() { build(s) })
}

func (g *Group) recordInvalid() {
	if g != nil && g.out != nil {
		g.out.recordMisuse(errInvalidConfig)
	}
}

func (g *Group) defineBuilder(run func()) {
	if g == nil || g.out == nil {
		return
	}
	o := g.out
	o.mu.Lock()
	col := o.containerStates[g.id]
	o.mu.Unlock()
	if col == nil {
		o.recordMisuse(g.rejected)
		return
	}
	o.graph.DefineBuilder(col.node, run)
}

// --- folded from child_counts.go ---

// childCounters counts a set of child Tasks by what a live tally reports
// of them, and takes a Task back out when it moves to another set.
type childCounters struct {
	total, done, running, pending, unfinished int
	// widths counts the Tasks at each name width, in runes, so the widest
	// survives a removal.
	widths   map[int]int
	maxWidth int
}

func (c *childCounters) add(f *filing) {
	c.move(f, 1)
	if c.widths == nil {
		c.widths = make(map[int]int)
	}
	c.widths[f.width]++
	c.maxWidth = max(c.maxWidth, f.width)
}

func (c *childCounters) remove(f *filing) {
	c.move(f, -1)
	if c.widths[f.width]--; c.widths[f.width] > 0 {
		return
	}
	delete(c.widths, f.width)
	if f.width < c.maxWidth {
		return
	}
	c.maxWidth = 0
	for w := range c.widths {
		c.maxWidth = max(c.maxWidth, w)
	}
}

// move counts f's Task by delta in every per-state total.
func (c *childCounters) move(f *filing, delta int) {
	c.total += delta
	switch f.state {
	case succeeded, record.Satisfied, excluded:
		c.done += delta
	case running:
		c.running += delta
	case pending:
		c.pending += delta
	}
	if !record.IsTerminalTask(f.state) {
		c.unfinished += delta
	}
}

// counts is c as a live tally, earliest being the earliest first paint
// among the counted Tasks.
func (c *childCounters) counts(earliest time.Time) project.ChildCounts {
	return project.ChildCounts{
		Total: c.total, Done: c.done,
		Running: c.running > 0, Pending: c.pending > 0, Unfinished: c.unfinished > 0,
		EarliestSeen: earliest, NameWidth: c.maxWidth,
	}
}

// stateCounts counts a collection's direct Tasks by the states the
// collection's verdict folds (verdictFold).
//
// slice 6: one of three counters of a collection's descendants by state, with
// the graph's tally and liveCensus; they fold into one count owned by project.
type stateCounts struct {
	running, failed, refused, cancelled, notStarted, unresolved, warned int
}

func (s *stateCounts) add(f *filing)    { s.move(f, 1) }
func (s *stateCounts) remove(f *filing) { s.move(f, -1) }

func (s *stateCounts) move(f *filing, delta int) {
	switch f.state {
	case running:
		s.running += delta
	case failed:
		s.failed += delta
	case refused:
		s.refused += delta
	case cancelled:
		s.cancelled += delta
	case notStarted:
		s.notStarted += delta
	case succeeded, record.Satisfied, excluded:
	default:
		s.unresolved += delta
	}
	if f.warned {
		s.warned += delta
	}
}

// fold is the verdict of the counted Tasks alone.
func (s stateCounts) fold() verdictFold {
	return verdictFold{
		running: s.running > 0, failed: s.failed > 0, refused: s.refused > 0,
		cancelled: s.cancelled > 0, unresolved: s.unresolved > 0,
	}
}

// seenEntry is one Task's first-paint stamp.
type seenEntry struct {
	at time.Time
	t  *taskState
}

// seenHeap is the first-paint stamps of a set of Tasks, earliest on top.
// A Task that left the set leaves its entry behind; earliest discards
// those when they surface.
type seenHeap []seenEntry

func (h seenHeap) Len() int           { return len(h) }
func (h seenHeap) Less(i, j int) bool { return h[i].at.Before(h[j].at) }
func (h seenHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *seenHeap) Push(x any)        { *h = append(*h, x.(seenEntry)) }
func (h *seenHeap) Pop() any {
	old := *h
	e := old[len(old)-1]
	*h = old[:len(old)-1]
	return e
}

// earliest is the earliest stamp of a Task still in the set, per member,
// or the zero time.
func (h *seenHeap) earliest(member func(*taskState) bool) time.Time {
	for h.Len() > 0 {
		top := (*h)[0]
		if member(top.t) && top.t.filing.seen.Equal(top.at) {
			return top.at
		}
		heap.Pop(h)
	}
	return time.Time{}
}

// --- folded from child_index.go ---

// childIndex keeps what a live frame needs of one collection's direct
// Tasks current as they change, so a frame reads the children it can show
// and the counts of the rest without walking them: a frame costs what the
// screen shows, however many Tasks the collection holds (E-091).
//
// A Task that changed is marked (taskState.markFiling) and refiled at the
// next read, so a burst of Progress calls on one Task refiles it once. The
// index files each child the way live.ClassifyChild does, so the frame it
// yields equals the one live.LiveChildren builds by walking every child.
type childIndex struct {
	dirty []*taskState

	// items are the disposition items and work every other child, by
	// declaration; byRank splits work by attention rank.
	items, work positionSet
	byRank      [live.AttentionRanks]positionSet

	all, workCounts childCounters
	states          stateCounts
	seenAll         seenHeap
	seenWork        seenHeap
	owns, peers     int

	// itemDispositions is the items' tally as of the items in [0,
	// itemsThrough]; itemsStale means a change it cannot append to.
	itemDispositions project.Dispositions
	itemsThrough     int
	itemsStale       bool

	// unstamped are Tasks filed while Running or Pending that no frame
	// has painted yet (taskState.stampLiveFirstSeen).
	unstamped []*taskState
}

// filing is how one child Task is filed in its collection's childIndex.
type filing struct {
	filed, dirty, listed bool
	// pos is the Task's declaration position among its collection's Tasks.
	pos  int
	kind live.ChildKind
	// state, width, seen, and warned are the Task fields the counts read.
	state  entityState
	width  int
	seen   time.Time
	warned bool
	// allSeenAt and workSeenAt are the stamps last pushed to the seen heaps.
	allSeenAt, workSeenAt time.Time
}

// markFiling records that t changed in a way its collection's frame or
// verdict reads, so the next read refiles it. Call it after any change to
// a taskState field the collection's frame shows or counts.
func (t *taskState) markFiling() {
	col := t.collection()
	if col == nil || t.filing.dirty {
		return
	}
	t.filing.dirty = true
	col.kids.dirty = append(col.kids.dirty, t)
}

// settled is g's index with every marked Task refiled.
func (g *tasksState) settled() *childIndex {
	g.out.pullRecordLocked()
	x := &g.kids
	for len(x.dirty) > 0 {
		dirty := x.dirty
		x.dirty = nil
		for _, t := range dirty {
			x.refile(g, t)
		}
	}
	return x
}

func (x *childIndex) refile(g *tasksState, t *taskState) {
	old := t.filing
	view := t.view()
	next := filing{
		filed: true, pos: old.pos, listed: old.listed,
		kind:  live.ClassifyChild(g.name, &view),
		state: t.node.Rec.State(), width: utf8.RuneCountInString(t.name),
		seen: t.liveFirstSeenAt, warned: t.node.Rec.WarningCount() > 0,
		allSeenAt: old.allSeenAt, workSeenAt: old.workSeenAt,
	}
	if old.filed {
		if next.sameFiling(old) && !next.kind.Item {
			t.filing = next
			return
		}
		x.unfile(t)
		next.workSeenAt = time.Time{}
	}
	t.filing = next
	x.file(g, t, &view)
}

func (f filing) sameFiling(o filing) bool {
	return f.kind == o.kind && f.state == o.state && f.width == o.width &&
		f.seen.Equal(o.seen) && f.warned == o.warned
}

func (x *childIndex) file(g *tasksState, t *taskState, view *record.TaskSnapshot) {
	f := &t.filing
	x.all.add(f)
	x.states.add(f)
	if f.kind.OwnTask {
		x.owns++
	}
	if !f.seen.IsZero() && !f.seen.Equal(f.allSeenAt) {
		f.allSeenAt = f.seen
		heap.Push(&x.seenAll, seenEntry{at: f.seen, t: t})
	}
	if f.kind.Item {
		x.items.add(f.pos)
		x.fileItem(f.pos, view)
	} else {
		x.work.add(f.pos)
		x.workCounts.add(f)
		if f.kind.Rank < live.AttentionRanks {
			x.byRank[f.kind.Rank].add(f.pos)
		}
		if f.kind.WorkPeer {
			x.peers++
		}
		if !f.seen.IsZero() && !f.seen.Equal(f.workSeenAt) {
			f.workSeenAt = f.seen
			heap.Push(&x.seenWork, seenEntry{at: f.seen, t: t})
		}
	}
	if t.unstampedIn(f.state) && !f.listed {
		f.listed = true
		x.unstamped = append(x.unstamped, t)
	}
}

func (x *childIndex) unfile(t *taskState) {
	f := &t.filing
	x.all.remove(f)
	x.states.remove(f)
	if f.kind.OwnTask {
		x.owns--
	}
	if f.kind.Item {
		x.items.remove(f.pos)
		x.itemsStale = true
		return
	}
	x.work.remove(f.pos)
	x.workCounts.remove(f)
	if f.kind.Rank < live.AttentionRanks {
		x.byRank[f.kind.Rank].remove(f.pos)
	}
	if f.kind.WorkPeer {
		x.peers--
	}
}

// fileItem adds the item at pos to the items' tally: appended when it
// follows every item already in it, since the tally is in declaration
// order, otherwise the tally is rebuilt at the next read.
func (x *childIndex) fileItem(pos int, view *record.TaskSnapshot) {
	if x.itemsStale || pos < x.itemsThrough {
		x.itemsStale = true
		return
	}
	x.itemDispositions.AddTask(view)
	x.itemsThrough = pos
}

func (x *childIndex) dispositions(g *tasksState) project.Dispositions {
	if x.itemsStale {
		x.itemDispositions, x.itemsThrough, x.itemsStale = project.Dispositions{}, -1, false
		x.items.each(func(pos int) {
			view := g.taskAt(pos).view()
			x.itemDispositions.AddTask(&view)
			x.itemsThrough = pos
		})
	}
	return x.itemDispositions.Clone()
}

// stampDirectTasks stamps the Tasks filed while no frame had painted
// them, as a frame that counted them does.
func (g *tasksState) stampDirectTasks(now time.Time) {
	x := g.settled()
	listed := x.unstamped
	x.unstamped = nil
	for _, t := range listed {
		t.filing.listed = false
		t.stampLiveFirstSeen(now)
	}
}

// project is the children a frame of rows rows could show, snapshotted in
// declaration order, and the roster of all of them.
func (x *childIndex) project(g *tasksState, rows int) ([]record.TaskSnapshot, live.ChildRoster) {
	budget := live.ChildRows(rows)
	pos := x.items.appendFirst(nil, budget+1)
	pos = x.work.appendFirst(pos, budget+1)
	for r := range x.byRank {
		pos = x.byRank[r].appendFirst(pos, budget)
	}
	slices.Sort(pos)
	pos = slices.Compact(pos)
	var kept []record.TaskSnapshot
	for _, p := range pos {
		kept = append(kept, g.taskAt(p).snapshot())
	}
	return kept, live.ChildRoster{
		All:    x.all.counts(x.seenAll.earliest(isFiled)),
		Work:   x.workCounts.counts(x.seenWork.earliest(isWork)),
		Items:  x.dispositions(g),
		Census: project.ChildCensus{Items: x.items.len(), OwnTask: x.owns > 0, WorkPeer: x.peers > 0},
	}
}

func isFiled(t *taskState) bool { return t.filing.filed }

func isWork(t *taskState) bool { return t.filing.filed && !t.filing.kind.Item }

// --- folded from container_wait.go ---

// Task declares a single root-level operation named name. A repeated name
// is a duplicate sibling declaration (§3.1), never a get-or-create: see
// graph.Graph.DeclareTask.
func (o *Run) Task(name string) *Task {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.taskHandle(o.graph.DeclareTask(nil, name))
}

// Group declares a collection of independent child tasks. Eligible children
// may overlap through the scheduler. A repeated name is a duplicate sibling
// declaration (§3.1), not a get-or-create.
func (o *Run) Group(name string) *Group {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.groupHandle(o.graph.DeclareContainer(nil, name, false))
}

// Sequence declares a self-managing, ordered container — the front door for
// a sequence of steps that must stop implying "still might run" once a
// member has already failed or been cancelled. A repeated name is a
// duplicate sibling declaration (§3.1), not a get-or-create.
func (o *Run) Sequence(name string) *Sequence {
	o.mu.Lock()
	defer o.mu.Unlock()
	return &Sequence{tasks: o.groupHandle(o.graph.DeclareContainer(nil, name, true))}
}

// taskHandle is the handle of the Task the graph declared, or a refused
// handle keeping err. Callers hold o.mu.
func (o *Run) taskHandle(node *graph.Task, err error) *Task {
	if err != nil {
		return o.rejectedTask(err)
	}
	return o.taskStates[node.ID].handle
}

// groupHandle is taskHandle for a Group or Sequence. Callers hold o.mu.
func (o *Run) groupHandle(node *graph.Container, err error) *Group {
	if err != nil {
		return o.rejectedGroup(err)
	}
	return o.containerStates[node.ID].handle
}

// --- folded from group.go ---

// Container is a node that can declare work beneath it: the run, a Group,
// or a Sequence. A Task is never a Container.
//
// Shared topology builders take a Container:
//
//	func Build(parent evo.Container)
type Container interface {
	Task(name string) *Task
	Group(name string) *Group
	Sequence(name string) *Sequence
}

var (
	_ Container = (*Run)(nil)
	_ Container = (*Group)(nil)
	_ Container = (*Sequence)(nil)
)

// Group is the front door for independent child work. Eligible
// children may overlap through the scheduler. The type cannot be named
// Group — that name is the constructor. Nest via Group/Sequence.
type Group struct {
	out *Run
	id  string
	// rejected is why the declaration was refused (see rejectedGroup); nil
	// for a declared Group or Sequence.
	rejected error
}

// Task declares a child task. A repeated name is a duplicate sibling
// declaration, not a get-or-create (§3.1, Output.Task's doc comment): it
// fails the new call with ProblemCodeDuplicateSiblingName rather than
// returning the earlier handle. Callers that reference a Task again later
// (for example in After) must keep the first handle, typically in a typed
// variable, instead of re-declaring by name.
func (g *Group) Task(name string) *Task {
	if g == nil || g.out == nil {
		return &Task{}
	}
	return g.out.declareChildTask(g, name)
}

// declareChildTask declares a Task named name in the container g. A Task
// declared under a refused container is refused for the same reason.
func (o *Run) declareChildTask(g *Group, name string) *Task {
	o.mu.Lock()
	defer o.mu.Unlock()
	col := o.containerStates[g.id]
	if col == nil {
		return o.rejectedTask(g.rejected)
	}
	return o.taskHandle(o.graph.DeclareTask(col.node, name))
}

// Sequence declares (or returns) an ordered child container nested here.
func (g *Group) Sequence(name string) *Sequence {
	child := g.declareChild(name, true)
	return &Sequence{tasks: child}
}

// Group declares (or returns) an independent child container nested here.
func (g *Group) Group(name string) *Group {
	return g.declareChild(name, false)
}

func (g *Group) declareChild(name string, sequential bool) *Group {
	if g == nil || g.out == nil {
		return &Group{}
	}
	g.out.mu.Lock()
	defer g.out.mu.Unlock()
	parent := g.out.containerStates[g.id]
	if parent == nil {
		return g.out.rejectedGroup(g.rejected)
	}
	return g.out.groupHandle(g.out.graph.DeclareContainer(parent.node, name, sequential))
}

// Summary sets a success-oriented collection summary.
func (g *Group) Summary(text string) *Group {
	if g == nil || g.out == nil {
		return g
	}
	g.out.mu.Lock()
	defer g.out.mu.Unlock()
	col := g.out.containerStates[g.id]
	if col == nil {
		return g
	}
	if err := g.out.ensureOpen(); err != nil {
		g.out.recordMisuse(err)
		return g
	}
	col.node.Summarize(text)
	g.out.bumpLocked()
	g.out.appendEventLocked(recEvent{Type: "tasks.summary_set", EntityID: g.id})
	return g
}

// --- folded from names.go ---

// --- folded from sequence.go ---

// Sequence is the front door for ordered child work: the same
// scheduler as Group with implicit predecessor edges in declaration order.
// A failed/blocked/cancelled child makes later siblings NotStarted.
type Sequence struct {
	tasks *Group
}

// Task declares (or, for a repeated name, returns) a child task in
// declaration order.
func (g *Sequence) Task(name string) *Task {
	if g == nil || g.tasks == nil {
		return &Task{}
	}
	return g.tasks.out.declareChildTask(g.tasks, name)
}

// Summary sets a success-oriented sequence summary.
func (g *Sequence) Summary(text string) *Sequence {
	if g == nil || g.tasks == nil {
		return g
	}
	g.tasks.Summary(text)
	return g
}

// Sequence declares (or returns) an ordered child container nested here.
func (g *Sequence) Sequence(name string) *Sequence {
	if g == nil || g.tasks == nil {
		return &Sequence{}
	}
	return g.tasks.Sequence(name)
}

// Group declares (or returns) an independent child container nested here.
func (g *Sequence) Group(name string) *Group {
	if g == nil || g.tasks == nil {
		return &Group{}
	}
	return g.tasks.Group(name)
}

// --- folded from task_annotate.go ---

// Doing sets the active current-step live text and starts the task if
// pending — replaces the previous text, promotes the task to Running, and
// becomes a durable line per step off-TTY. text is a printf format when args
// are present (fmt.Sprintf semantics). Chained right after Task, it sets
// the first step at declaration.
func (t *Task) Doing(text string, args ...any) *Task {
	if len(args) > 0 {
		text = fmt.Sprintf(text, args...)
	}
	return t.annotate(func(st *taskState) { t.out.setPhaseLocked(st, text) })
}

// resolvedByInterrupt reports whether this state was reached by the
// interrupt sweep rather than by the caller. Narrating such a row is not
// misuse: cancellation resolves it underneath whoever was reporting on it,
// and a worker already inside its per-item step had no way to prevent the
// one straggling update that follows. Blaming the caller for the interrupt's
// own timing put a misuse warning at the top of every interrupted ledger.
func resolvedByInterrupt(state entityState) bool {
	return state == cancelled || state == notStarted
}

// annotate is the one guard every non-terminal annotation verb (Doing,
// Progress, Bytes, Summary, Problem, Fact) shares: under o.mu,
// it applies apply to the task's state only while the task is open. On a
// closed Output it records that misuse; on a terminal row it records
// ErrAlreadyResolved, unless the interrupt sweep resolved the row (see
// resolvedByInterrupt). It returns t so each verb can chain.
func (t *Task) annotate(apply func(st *taskState)) *Task {
	return t.withTask(func(st *taskState) {
		if err := t.out.ensureOpen(); err != nil {
			t.out.recordMisuse(err)
			return
		}
		if record.IsTerminalTask(st.node.Rec.State()) {
			if !resolvedByInterrupt(st.node.Rec.State()) {
				t.out.recordMisuseFor(st.name, errAlreadyResolved)
			}
			return
		}
		apply(st)
	})
}

// withTask is the one lock-and-lookup every Task verb that
// edits its row goes through: under o.mu it hands apply the task's state,
// and does nothing for a nil or zero Task or a task this Output does
// not know. That is where "a nil *Task is safe" is kept, for every
// verb at once. It returns t so each verb can chain.
func (t *Task) withTask(apply func(st *taskState)) *Task {
	if t == nil || t.out == nil {
		return t
	}
	t.out.mu.Lock()
	defer t.out.mu.Unlock()
	if st := t.out.taskStates[t.id]; st != nil {
		apply(st)
		st.markFiling()
	}
	return t
}

// setLiveOnlyPhase updates the task's phase text through setLiveOnlyPhaseLocked
// — the shared entry point for every phase source that is NOT the caller's
// own narrated beat: Writer's per-line mirror of a talkative child's raw
// output. Off-TTY, an explicit Task.Doing call
// still forces its own durable row (the P10 contract: the one line the
// caller asked to see); this path never does — a child's full output already
// has one durable home, the evidence ring (and its failure-path DetailTail),
// so a row per mirrored line would just repeat it (release-gate round 9
// finding 4).
func (t *Task) setLiveOnlyPhase(text string) {
	t.annotate(func(st *taskState) { t.out.setLiveOnlyPhaseLocked(st, text) })
}

// setPhaseLocked is Doing's locked body. Callers must already hold o.mu and
// have checked ensureOpen/isTerminalTask.
func (o *Run) setPhaseLocked(st *taskState, text string) {
	if st.node.Doing(text, o.cfg.clock.Now()) {
		o.announceRunningLocked(st)
	}
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: "task.phase_changed", EntityID: st.id})
	o.signalLiveLocked(true)
	o.emitTaskRunningProgressiveLocked(st, triggerPhase)
}

// setLiveOnlyPhaseLocked is the shared locked body behind every phase
// source that is not the caller's own narrated beat (see setLiveOnlyPhase):
// the same state transition as setPhaseLocked (promotion, activity clock,
// live redraw signal), but it never forces its own durable line in plain
// mode. A talkative child's mirrored output line (Writer) already has one
// durable home, the evidence ring, so it gets no plain-mode row per line
// (release-gate round 9 finding 4).
func (o *Run) setLiveOnlyPhaseLocked(st *taskState, text string) {
	// Identical live-only phase is a no-op: Writer leftover/repeated lines
	// must not bump, emit task.phase_changed, or force a live paint.
	changed, started := st.node.Mirror(text, o.cfg.clock.Now())
	if !changed {
		return
	}
	if started {
		o.announceRunningLocked(st)
	}
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: "task.phase_changed", EntityID: st.id})
	o.signalLiveLocked(true)
}

// recordWarning is the warning-severity Problem path: st.warnings,
// task.warned, EventWarningRecorded, censusWarned. Public Warn was removed in 1.1.
func (t *Task) recordWarning(p problem) *Task {
	return t.annotate(func(st *taskState) {
		if st.node.Warn(p) {
			st.censusWarned()
		}
		t.out.bumpLocked()
		t.out.appendEventLocked(recEvent{Type: "task.warned", EntityID: t.id})
		t.out.emitWireEventLocked(wire.EventWarningRecorded, t.id, wire.ToProblemDoc(p).EventPayload())
		t.out.signalLiveLocked(true)
	})
}

// Summary sets one sanitized line of result text for the task's terminal
// row: last call wins, empty clears it. It never resolves the task (Define
// or the Evo-native operation outcome does), and it is not live activity
// (Doing/Progress/Bytes own the Running row). It is the same field
// TaskSnapshot.Summary and JSON/JSONL "summary" project. Calling it after
// the task resolved is misuse unless the interrupt sweep resolved it (see
// annotate). Group.Summary is the same shape one level up.
func (t *Task) Summary(text string) *Task {
	return t.annotate(func(st *taskState) {
		st.node.Summarize(text)
		t.out.bumpLocked()
		t.out.appendEventLocked(recEvent{Type: "task.summary_set", EntityID: t.id})
		t.out.signalLiveLocked(true)
	})
}

// Problem attaches one structured diagnostic to the Task. It is an
// annotation: it never ends the Task or changes its outcome, which settles
// from the Define callback's return alone and carries the Problem in every
// output mode. A fatal finding is returned through Fail. SeverityWarning
// uses the warning projection. An invalid severity is rejected with a
// context-bearing misuse error and is not recorded.
func (t *Task) Problem(summary string, opts ...ProblemOption) *Task {
	p := applyProblemOptions(project.Text(summary), opts)
	sev, err := record.ClassifiedProblemSeverity(p)
	if err != nil {
		return t.annotate(func(st *taskState) { t.out.recordMisuse(err) })
	}
	if sev == severityWarning {
		return t.recordWarning(p)
	}
	return t.recordBlockingProblem(p)
}

func (t *Task) recordBlockingProblem(p problem) *Task {
	return t.annotate(func(st *taskState) {
		st.node.Report(p)
		t.out.bumpLocked()
		t.out.appendEventLocked(recEvent{Type: "task.problem_recorded", EntityID: t.id})
		t.out.emitWireEventLocked(wire.EventProblemRecorded, t.id, wire.ToProblemDoc(p).EventPayload())
		t.out.signalLiveLocked(true)
	})
}

// Fact accumulates a discovered name/value annotation on the task — info
// severity, Problem(SeverityWarning)'s non-terminal sibling (user-13-problems.md Problem 8:
// "Tasks are work. Facts are information."). Renders as a dim "name  value"
// line, inline when it is the task's only annotation, nested otherwise.
// Like Problem, it returns the Task for chaining and never resolves it — call
// it any number of times before the task's terminal verb.
func (t *Task) Fact(name, value string) *Task {
	f := record.SanitizeFact(factRecord{Name: project.Text(name), Value: project.Text(value)})
	return t.annotate(func(st *taskState) {
		st.node.Note(f)
		t.out.bumpLocked()
		t.out.emitWireEventLocked(wire.EventFactRecorded, t.id, wire.ToFactDoc(f).EventPayload())
		t.out.signalLiveLocked(true)
	})
}

// --- folded from task_next.go ---

// --- folded from task_progress.go ---

// Progress sets absolute completed/total count progress.
// Counts use int (collection lengths, indices). For byte quantities use Bytes.
// Prefer absolute Progress over Advance so retries cannot double-count.
func (t *Task) Progress(completed, total int) *Task {
	return t.setProgress(int64(completed), int64(total), determinate)
}

func (t *Task) setProgress(completed, total int64, kind progressKind) *Task {
	return t.annotate(func(st *taskState) { t.applyProgressLocked(st, completed, total, kind) })
}

// applyProgressLocked reports whether the update was applied — false means a
// guard (invalid values, regression, sealed-total mismatch) rejected it and
// recorded misuse instead, so a paired live update is not applied for a progress change that never happened
// (e.g. Phase) that would otherwise describe a progress change that never
// happened.
func (t *Task) applyProgressLocked(st *taskState, completed, total int64, kind progressKind) bool {
	verdict, started := st.node.Progress(completed, total, kind, t.out.cfg.clock.Now())
	switch verdict {
	case record.ProgressInvalid:
		t.out.recordMisuse(errInvalidProgress)
		return false
	case record.ProgressRegressed:
		t.out.recordMisuse(errProgressRegression)
		return false
	}
	if started {
		t.out.announceRunningLocked(st)
	}
	t.out.bumpLocked()
	t.out.appendEventLocked(recEvent{Type: "task.progress_changed", EntityID: t.id})
	// Progress is high-frequency: coalesce unless first frame.
	t.out.signalLiveLocked(false)
	t.out.emitTaskRunningProgressiveLocked(st, triggerProgress)
	return true
}

// taskFinishedPayload is the task.finished event's payload: the Outcome
// the Task settled with, and its Summary when it has one (ZYS-971).
func taskFinishedPayload(st *taskState) map[string]any {
	payload := map[string]any{
		"outcome": string(st.node.Rec.State().Outcome()),
	}
	if st.node.Rec.Summary() != "" {
		payload["summary"] = st.node.Rec.Summary()
	}
	return payload
}

// --- folded from task_run.go ---

// --- folded from task_taxonomy.go ---

// --- folded from wait.go ---

type DefineOption func(*defineConfig)

type defineConfig struct{ cleanStopSignals []os.Signal }

func CleanStop(signals ...os.Signal) DefineOption {
	return func(c *defineConfig) { c.cleanStopSignals = append(c.cleanStopSignals, signals...) }
}

func (g *Group) Build(fn func(*Group)) { g.define(fn) }

func (s *Sequence) Build(fn func(*Sequence)) { s.define(fn) }

// cleanStopTask is one running Task that declared CleanStop: the signals
// that end it Succeeded, and the cancel of its own context.
type cleanStopTask struct {
	signals []os.Signal
	cancel  context.CancelFunc
	stopped atomic.Bool
}

func (c *cleanStopTask) names(sig os.Signal) bool {
	return slices.Contains(c.signals, sig)
}

// cleanStopRegistry holds the CleanStop Tasks that are running now. Its zero
// value is ready to use.
type cleanStopRegistry struct {
	mu      sync.Mutex
	running map[*cleanStopTask]struct{}
}

func (r *cleanStopRegistry) add(c *cleanStopTask) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.running == nil {
		r.running = map[*cleanStopTask]struct{}{}
	}
	r.running[c] = struct{}{}
}

func (r *cleanStopRegistry) remove(c *cleanStopTask) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.running, c)
}

// stop cancels every running Task that names sig and reports whether any
// did; when none does, the signal belongs to the run.
func (r *cleanStopRegistry) stop(sig os.Signal) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	handled := false
	for c := range r.running {
		if c.names(sig) {
			c.stopped.Store(true)
			c.cancel()
			handled = true
		}
	}
	return handled
}

// wrap runs fn under a context a named signal can cancel, and reports a
// callback stopped that way as success.
func (r *cleanStopRegistry) wrap(signals []os.Signal, fn func(context.Context) error) func(context.Context) error {
	if len(signals) == 0 {
		return fn
	}
	return func(ctx context.Context) error {
		taskCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		task := &cleanStopTask{signals: signals, cancel: cancel}
		r.add(task)
		defer r.remove(task)
		err := fn(taskCtx)
		if task.stopped.Load() {
			return nil
		}
		return err
	}
}
