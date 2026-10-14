package evo

import (
	"container/heap"
	"context"
	"errors"
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

// --- folded from after.go ---

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
	if !o.graph.AddAfter(st.node, o.predecessorsOfLocked(preds)...) {
		o.recordMisuse(errInvalidConfig)
	}
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

// --- folded from builder.go ---

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
	if !o.graph.AddContainerAfter(col.node, o.predecessorsOfLocked(preds)...) {
		o.recordMisuse(errInvalidConfig)
	}
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
	switch {
	case col == nil:
		o.recordMisuse(g.rejected)
		o.mu.Unlock()
		return
	case o.ensureOpen() != nil:
		o.recordMisuse(errClosed)
		o.mu.Unlock()
		return
	}
	node := o.graph.AddGate(col.node, func() error { run(); return nil })
	if node == nil {
		o.recordMisuse(errInvalidConfig)
		o.mu.Unlock()
		return
	}
	gate := &taskState{id: node.ID, node: node, name: node.Name, followed: declaredState}
	gate.handle = &Task{out: o, id: gate.id}
	o.taskStates[gate.id] = gate
	o.graph.Submit(node, graph.Work{})
	o.followRecordLocked()
	o.mu.Unlock()
	o.graph.Kick()
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

// --- folded from container_members.go ---

// A container's members belong to the graph: it declared them, in order, and
// owns who contains whom. The engine keeps only each member's render state,
// in the Output's tables keyed by the member's id, and reads the membership
// through the node.

// taskCount is how many Tasks g declared directly.
func (g *tasksState) taskCount() int { return g.node.TaskCount() }

// taskAt is the render state of the Task g declared at position pos.
func (g *tasksState) taskAt(pos int) *taskState {
	return g.out.taskStates[g.node.TaskAt(pos).ID]
}

// taskStates are the render states of the Tasks g declared directly, in
// declaration order.
func (g *tasksState) taskStates() []*taskState {
	nodes := g.node.Tasks()
	states := make([]*taskState, len(nodes))
	for i, node := range nodes {
		states[i] = g.out.taskStates[node.ID]
	}
	return states
}

// childStates are the render states of the Groups and Sequences g declared
// directly, in declaration order.
func (g *tasksState) childStates() []*tasksState {
	nodes := g.node.Children()
	states := make([]*tasksState, len(nodes))
	for i, node := range nodes {
		states[i] = g.out.containerStates[node.ID]
	}
	return states
}

// parent is the container g is nested in, nil at the root.
func (g *tasksState) parent() *tasksState {
	if g.node.Parent == nil {
		return nil
	}
	return g.out.containerStates[g.node.Parent.ID]
}

// collection is the container st was declared under, nil for a root Task.
func (st *taskState) collection() *tasksState {
	if st.node.Parent == nil {
		return nil
	}
	return st.handle.out.containerStates[st.node.Parent.ID]
}

// --- folded from container_wait.go ---

// --- folded from declare.go ---

// declarationRefusal is the engine's error for a declaration the graph refused.
func declarationRefusal(err error) error {
	switch {
	case errors.Is(err, graph.ErrEntityLimit):
		return errLimitExceeded
	case errors.Is(err, graph.ErrClosed):
		return errClosed
	default:
		return fmt.Errorf("declaring a Task: %w", err)
	}
}

// Task declares a single root-level operation named name. Identity
// overrides go through the task key override. A repeated name is a duplicate
// sibling declaration (§3.1), never a get-or-create: 1.0 removed that idiom
// because letting two distinct declarations silently merge into one
// identity would make a false "already satisfied" possible once identity
// drives manifest reconciliation. The repeat records a Failed task with
// ProblemCodeDuplicateSiblingName (see failDuplicateSiblingLocked).
func (o *Run) Task(name string) *Task {
	clean := graph.DeclaredName(name)
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.graph.NameTaken(nil, graph.KindTask, clean) {
		return o.rejectedTask(o.failDuplicateSiblingLocked(nil, graph.KindTask, clean))
	}
	h := o.addTaskLocked(clean, nil)
	if h.rejected == nil {
		o.graph.ClaimName(nil, graph.KindTask, clean)
	}
	return h
}

func (o *Run) addTaskLocked(name string, col *tasksState) *Task {
	h := o.declareTaskLocked(name, col)
	if _, ok := o.taskStates[h.id]; ok {
		o.signalLiveLocked(true)
	}
	return h
}

// declareTaskLocked records a child without painting; addTaskLocked
// paints it.
//
// The task's §3.1 stable identity is kind + the declaring parent's key
// (parentKeyOf) + name, for every declaration, a refused duplicate's row
// included; the task key override replaces it later.
func (o *Run) declareTaskLocked(name string, col *tasksState) *Task {
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return o.rejectedTask(err)
	}
	if o.graph.DeclaredInCallback() {
		o.recordMisuse(errDeclaredInCallback)
		return o.rejectedTask(errDeclaredInCallback)
	}
	node, err := o.graph.AddTask(containerNode(col), name, record.TaskInit{
		State: declaredState, Progress: progress{Kind: indeterminate},
	})
	if err != nil {
		err = declarationRefusal(err)
		o.recordMisuse(err)
		return o.rejectedTask(err)
	}
	st := &taskState{
		id:       node.ID,
		node:     node,
		name:     node.Name,
		followed: declaredState,
		timing:   record.TaskTiming{Declared: o.cfg.clock.Now()},
	}
	h := &Task{out: o, id: st.id}
	st.handle = h
	o.appendTaskLocked(st)
	if col != nil {
		st.filing.pos = col.taskCount() - 1
		st.markFiling()
		col.hasNamesake = col.hasNamesake || name == col.name
		st.censusDeclared()
	}
	o.taskStates[st.id] = st
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: "task.declared", EntityID: st.id})
	o.emitWireEventLocked(wire.EventTaskDeclared, st.id, map[string]any{"name": name})
	return h
}

// Group declares a collection of independent child tasks. Eligible children
// may overlap through the scheduler. A repeated name is a duplicate sibling
// declaration (§3.1), not a get-or-create — see failDuplicateSiblingLocked.
func (o *Run) Group(name string) *Group {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.declareContainerLocked(nil, name, false)
}

// Sequence declares a self-managing, ordered container — the front door for
// a sequence of steps that must stop implying "still might run" once a
// member has already failed or been cancelled. A repeated name is a
// duplicate sibling declaration (§3.1), not a get-or-create — see
// failDuplicateSiblingLocked.
func (o *Run) Sequence(name string) *Sequence {
	o.mu.Lock()
	defer o.mu.Unlock()
	return &Sequence{tasks: o.declareContainerLocked(nil, name, true)}
}

// declareContainerLocked declares a Group (sequential false) or Sequence
// under parent, or at the root when parent is nil — the one path every
// level shares, so one rule decides duplicates everywhere (§3.1). A nested
// container's identity is its parent's key plus its name, and it starts
// after the step before it when parent is a Sequence. A refusal comes back
// as a rejected handle. Callers must already hold o.mu.
func (o *Run) declareContainerLocked(parent *tasksState, name string, sequential bool) *Group {
	clean := graph.DeclaredName(name)
	kind := graph.ContainerKind(sequential)
	if o.graph.NameTaken(containerNode(parent), kind, clean) {
		return o.rejectedGroup(o.failDuplicateSiblingLocked(parent, kind, clean))
	}
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return o.rejectedGroup(err)
	}
	if o.graph.DeclaredInCallback() {
		o.recordMisuse(errDeclaredInCallback)
		return o.rejectedGroup(errDeclaredInCallback)
	}
	node := o.graph.AddContainer(containerNode(parent), clean, sequential)
	st := &tasksState{
		id:   node.ID,
		out:  o,
		node: node,
		name: node.Name,
	}
	parentID := ""
	if parent == nil {
		o.collections = append(o.collections, st)
	} else {
		parentID = parent.id
	}
	o.containerStates[st.id] = st
	o.graph.ClaimName(containerNode(parent), kind, clean)
	h := &Group{out: o, id: st.id}
	st.handle = h
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: "tasks.declared", EntityID: st.id})
	o.emitCollectionDeclaredLocked(st, parentID)
	return h
}

// declareGroupTask declares a child task by name in the container g — the
// identity behind Group.Task/Sequence.Task. A repeated name is a duplicate
// sibling declaration (§3.1), not a get-or-create. A task declared under a
// refused container is refused for the same reason.
func (o *Run) declareGroupTask(g *Group, name string) *Task {
	clean := graph.DeclaredName(name)
	o.mu.Lock()
	defer o.mu.Unlock()
	col := o.containerStates[g.id]
	if col == nil {
		return o.rejectedTask(g.rejected)
	}
	if o.graph.NameTaken(col.node, graph.KindTask, clean) {
		return o.rejectedTask(o.failDuplicateSiblingLocked(col, graph.KindTask, clean))
	}
	h := o.addTaskLocked(clean, col)
	if h.rejected == nil {
		o.graph.ClaimName(col.node, graph.KindTask, clean)
	}
	return h
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
	return g.out.declareGroupTask(g, name)
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
	return g.out.declareContainerLocked(parent, name, sequential)
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
	col.node.Rec.SetSummary(text)
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
	return g.tasks.out.declareGroupTask(g.tasks, name)
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
	st.node.Rec.SetPhase(text)
	st.node.Rec.MarkActivity(o.cfg.clock.Now())
	if st.node.Rec.State() == pending {
		o.promoteRunningLocked(st)
		st.node.Rec.EnsureIndeterminateProgress()
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
	if !st.node.Rec.SetPhaseIfChanged(text) {
		return
	}
	st.node.Rec.MarkActivity(o.cfg.clock.Now())
	if st.node.Rec.State() == pending {
		o.promoteRunningLocked(st)
		st.node.Rec.EnsureIndeterminateProgress()
	}
	o.bumpLocked()
	o.appendEventLocked(recEvent{Type: "task.phase_changed", EntityID: st.id})
	o.signalLiveLocked(true)
}

// recordWarning is the warning-severity Problem path: st.warnings,
// task.warned, EventWarningRecorded, censusWarned. Public Warn was removed in 1.1.
func (t *Task) recordWarning(p problem) *Task {
	return t.annotate(func(st *taskState) {
		if st.node.Rec.AppendWarning(p) == 1 {
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
		st.node.Rec.SetSummary(text)
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
		st.node.Rec.AppendProblems(p)
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
		st.node.Rec.AppendFact(f)
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
	switch st.node.Rec.ApplyProgress(completed, total, kind) {
	case record.ProgressInvalid:
		t.out.recordMisuse(errInvalidProgress)
		return false
	case record.ProgressRegressed:
		t.out.recordMisuse(errProgressRegression)
		return false
	}
	st.node.Rec.MarkActivity(t.out.cfg.clock.Now())
	if st.node.Rec.State() == pending {
		t.out.promoteRunningLocked(st)
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

// --- folded from task_state.go ---

// key is the Task's §3.1 stable key, empty for a Task the graph never declared.
func (st *taskState) key() string { return st.node.Key() }

// TaskID is the Task's id in the run record.
func (st *taskState) TaskID() record.TaskID { return record.TaskID(st.id) }

// isGate reports whether st is a container builder's gate, no row and in no
// collection.
func (st *taskState) isGate() bool { return st.node.IsGate() }

// key is the container's §3.1 stable key.
func (g *tasksState) key() string { return g.node.Key() }

type taskState struct {
	id string
	// node is the graph's declaration of this Task. It owns the Task's stable
	// key and everything the scheduler knows: its phase, predecessors and
	// the work Define submitted.
	node   *graph.Task
	name   string
	handle *Task

	// liveFirstSeenAt is the domain-clock time this task was first actually
	// painted in the live region (see taskState.stampLiveFirstSeen in live.go) —
	// the one elapsed-time anchor every row's heartbeat suffix reads (P5),
	// including a Pending task, which never calls Phase/Progress.
	liveFirstSeenAt time.Time

	// followed is the state the render state last accounted for: the live
	// census counts the Task under it, and a later difference from the
	// record's state is what followRecordLocked reacts to. A Task starts
	// followed as declaredState, so a Task settled before the engine ever
	// saw it still gets its first reaction.
	followed entityState
	// settleAnswered is set once the render work a settle owes was queued.
	settleAnswered bool
	// runningAnnounced is set once the render work becoming Running owes was
	// done, so task.started fires once however the row reached Running.
	runningAnnounced bool
	// timing is when the Task was declared, began running and settled.
	timing record.TaskTiming

	// ownsWindow is set on a Confirm gate while its Suspend window is open
	// (guarded by Output.mu): the window shows this Task's rows, so they are
	// not held back like a sibling's.
	ownsWindow bool

	// filing is where this Task stands in its collection's childIndex.
	filing filing

	// heartbeat is the §40 plain-mode durable heartbeat's state.
	heartbeat plainHeartbeat

	// tail is the bounded window of Writer lines a live frame draws beneath
	// this row; evidence above stays the full record.
	tail liveTail

	// Emission bookkeeping so terminal standalone tasks stream in plain mode
	// on resolve (P2).
	coreEmitted bool

	// synthetic marks a task the library invented to carry an output-level
	// outcome (Output.Fail/Cancel's synthetic "command" task) rather than
	// one the caller declared. shouldSuppressRepeatedCondition (I2) must
	// never drop the standalone conclusion band for one of these: it is the
	// only place the run's outcome is ever stated, unlike a caller-declared
	// Task whose own row already says the same thing.
	synthetic bool

	// plainStream is what plain progressive streaming already emitted for
	// this still-Running standalone task.
	plainStream plainStreamMark
}

type tasksState struct {
	id string
	// out is the Output this container belongs to: a read of its live index
	// first follows the record (see Output.followRecordLocked).
	out *Run
	// node is the graph's declaration of this Group/Sequence and owns its
	// §3.1 stable key.
	node   *graph.Container
	name   string
	handle *Group

	// kids is what a live frame and the verdict read of tasks.
	kids childIndex

	// runningSteps holds the children promoteRunningLocked moved to
	// Running that may still be Running (pruned on each promotion), so the
	// "one Running child" check never rescans every step.
	runningSteps []*taskState

	// census counts its descendant Tasks for the live frame (liveCensus).
	census liveCensus
	// hasNamesake records that a child Task carries this container's own
	// name, the only way it can render as its own Task (liveOwnRow).
	hasNamesake bool
}

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
