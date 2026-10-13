package evo

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/zachbornheimer/evident-output/internal/graph"
	"github.com/zachbornheimer/evident-output/internal/process"
	"github.com/zachbornheimer/evident-output/internal/project"
	"github.com/zachbornheimer/evident-output/internal/project/live"
	"github.com/zachbornheimer/evident-output/internal/project/wire"
	"github.com/zachbornheimer/evident-output/internal/record"
)

// TaskHandle is a handle for one operation with phases or progress.
type TaskHandle struct {
	out *Output
	id  string
	// rejected is why the declaration was refused (see rejectedTask); nil
	// for a declared Task.
	rejected error
	// facade holds this handle's public wrapper (see FacadeSlot).
	facade FacadeSlot
}

// Context reports the cancellation signal this task's work runs under — the
// run's own (see Output.Context), so a Define or Effect callback
// doing I/O selects on it and stops when the run is interrupted.
func (t *TaskHandle) Context() context.Context {
	if t == nil {
		return context.Background()
	}
	return t.out.Context()
}

// Snapshot returns the task snapshot.
func (t *TaskHandle) Snapshot() TaskSnapshot {
	if t == nil || t.out == nil {
		return TaskSnapshot{State: Pending, Progress: Progress{Kind: Indeterminate}}
	}
	t.out.mu.Lock()
	defer t.out.mu.Unlock()
	st := t.out.taskStates[t.id]
	if st == nil {
		return TaskSnapshot{ID: t.id, State: Pending, Progress: Progress{Kind: Indeterminate}}
	}
	return st.snapshot()
}

// Basis declares freshness inputs for this Task. Calls accumulate. Basis is
// Task configuration frozen at Define: a call after Define records
// ErrBasisAfterDefine and is ignored.
func (t *TaskHandle) Basis(inputs ...Fingerprint) *TaskHandle {
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
	if !st.neverDefined() {
		o.recordMisuseFor(st.name, ErrBasisAfterDefine)
		return t
	}
	sources := make([]BasisSource, 0, len(inputs))
	for _, in := range inputs {
		if in != nil {
			sources = append(sources, FingerprintBasis(in))
		}
	}
	o.taskFreshness.AddInputs(st.TaskID(), sources...)
	return t
}

// --- folded from after.go ---

// After declares predecessors: this Task starts only once every one of
// them succeeded, and never starts once one of them cannot.
func (t *TaskHandle) After(preds ...any) *TaskHandle {
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
		o.recordMisuse(ErrInvalidConfig)
	}
	return t
}

// predecessorsOfLocked resolves After's arguments, leaving out the ones
// After ignores.
func (o *Output) predecessorsOfLocked(args []any) []graph.Predecessor {
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
func (o *Output) predecessorOfLocked(p any) (pred graph.Predecessor, ok bool) {
	switch x := p.(type) {
	case *TaskHandle:
		if x == nil || x.id == "" {
			return graph.Predecessor{}, false
		}
		if x.out == o {
			return graph.AfterTask(o.nodeOfTask(x.id)), true
		}
	case *GroupHandle:
		if x == nil || x.id == "" {
			return graph.Predecessor{}, false
		}
		if x.out == o {
			return graph.AfterContainer(o.nodeOfContainer(x.id)), true
		}
	case *SequenceHandle:
		if x == nil || x.tasks == nil {
			return graph.Predecessor{}, false
		}
		return o.predecessorOfLocked(x.tasks)
	case interface{ Producer() *TaskHandle }:
		if x == nil {
			return graph.Predecessor{}, false
		}
		return o.predecessorOfLocked(x.Producer())
	default:
		return graph.Predecessor{}, false
	}
	return graph.Predecessor{}, true
}

// nodeOfTask is the graph's declaration of the Task named id, nil when this
// Output declared none.
func (o *Output) nodeOfTask(id string) *graph.Task {
	if st := o.taskStates[id]; st != nil {
		return st.node
	}
	return nil
}

// nodeOfContainer is the graph's declaration of the container named id, nil
// when this Output declared none.
func (o *Output) nodeOfContainer(id string) *graph.Container {
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
func (g *GroupHandle) After(preds ...any) *GroupHandle {
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
		o.recordMisuse(ErrInvalidConfig)
	}
	return g
}

// After is GroupHandle.After for a Sequence.
func (s *SequenceHandle) After(preds ...any) *SequenceHandle {
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
func (g *GroupHandle) Define(build func(*GroupHandle)) {
	if build == nil {
		g.recordInvalid()
		return
	}
	g.defineBuilder(func() { build(g) })
}

// Define is GroupHandle.Define for a Sequence: build declares ordered steps.
func (s *SequenceHandle) Define(build func(*SequenceHandle)) {
	if s == nil || s.tasks == nil {
		return
	}
	if build == nil {
		s.tasks.recordInvalid()
		return
	}
	s.tasks.defineBuilder(func() { build(s) })
}

func (g *GroupHandle) recordInvalid() {
	if g != nil && g.out != nil {
		g.out.recordMisuse(ErrInvalidConfig)
	}
}

func (g *GroupHandle) defineBuilder(run func()) {
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
		o.recordMisuse(ErrClosed)
		o.mu.Unlock()
		return
	}
	node := o.graph.AddGate(col.node, func() error { run(); return nil })
	if node == nil {
		o.recordMisuse(ErrInvalidConfig)
		o.mu.Unlock()
		return
	}
	gate := &taskState{id: node.ID, node: node, name: node.Name, followed: declaredState}
	gate.handle = &TaskHandle{out: o, id: gate.id}
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
	case Done, Skipped:
		c.done += delta
	case Running:
		c.running += delta
	case Pending:
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
	running, failed, blocked, cancelled, notStarted, unresolved, warned int
}

func (s *stateCounts) add(f *filing)    { s.move(f, 1) }
func (s *stateCounts) remove(f *filing) { s.move(f, -1) }

func (s *stateCounts) move(f *filing, delta int) {
	switch f.state {
	case Running:
		s.running += delta
	case Failed:
		s.failed += delta
	case Blocked:
		s.blocked += delta
	case Cancelled:
		s.cancelled += delta
	case NotStarted:
		s.notStarted += delta
	case Done, Skipped:
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
		running: s.running > 0, failed: s.failed > 0, blocked: s.blocked > 0,
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
	state  EntityState
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

// Wait blocks until every task this Group's children (and their nested
// children, recursively) declared has settled, and returns their aggregate
// outcome. It does not itself serialize eligible
// siblings: every descendant Task was already submitted to the scheduler by
// its own Define call, so Wait only parks on outcomes the scheduler is
// already free to produce concurrently — the same non-serializing guarantee
// TaskHandle.Wait already gives a single Task.
//
// A caller never snapshots/counts failed children to know whether the
// container succeeded: Wait alone is the ordinary control-flow answer, and
// it returns nil only when every descendant actually ran and succeeded.
// Per-child detail remains available through Snapshot.
func (g *GroupHandle) Wait() error {
	if g == nil || g.out == nil {
		return nil
	}
	if g.rejected != nil {
		return rejectedWaitOutcome(g.rejected)
	}
	node := g.out.graph.Container(g.id)
	if node == nil {
		return nil
	}
	outcome := g.out.graph.WaitContainer(node)
	g.out.followRecordAfterWait()
	return outcome
}

// Wait is Sequence's counterpart to GroupHandle.Wait: the ordered container
// settles once every declared step (and any nested Group/Sequence) has
// settled, honoring the same NotStarted-suppression and error-join rules.
func (s *SequenceHandle) Wait() error {
	if s == nil || s.tasks == nil {
		return nil
	}
	return s.tasks.Wait()
}

// --- folded from declare.go ---

// declarationRefusal is the engine's error for a declaration the graph refused.
func declarationRefusal(err error) error {
	switch {
	case errors.Is(err, graph.ErrEntityLimit):
		return ErrLimitExceeded
	case errors.Is(err, graph.ErrClosed):
		return ErrClosed
	default:
		return fmt.Errorf("declaring a Task: %w", err)
	}
}

// Task declares a single root-level operation named name. Identity
// overrides go through TaskHandle.Key. A repeated name is a duplicate
// sibling declaration (§3.1), never a get-or-create: 1.0 removed that idiom
// because letting two distinct declarations silently merge into one
// identity would make a false "already satisfied" possible once identity
// drives manifest reconciliation. The repeat records a Failed task with
// ProblemCodeDuplicateSiblingName (see failDuplicateSiblingLocked).
func (o *Output) Task(name string) *TaskHandle {
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

func (o *Output) addTaskLocked(name string, col *tasksState) *TaskHandle {
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
// included; TaskHandle.Key replaces it later.
func (o *Output) declareTaskLocked(name string, col *tasksState) *TaskHandle {
	if err := o.ensureOpen(); err != nil {
		o.recordMisuse(err)
		return o.rejectedTask(err)
	}
	if o.graph.DeclaredInCallback() {
		o.recordMisuse(ErrDeclaredInCallback)
		return o.rejectedTask(ErrDeclaredInCallback)
	}
	node, err := o.graph.AddTask(containerNode(col), name, record.TaskInit{
		State: declaredState, Progress: Progress{Kind: Indeterminate}, Resolution: ResolutionNoWork,
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
	}
	h := &TaskHandle{out: o, id: st.id}
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
	o.appendEventLocked(Event{Type: "task.declared", EntityID: st.id})
	o.emitWireEventLocked(wire.EventTaskDeclared, st.id, map[string]any{"name": name})
	return h
}

// Group declares a collection of independent child tasks. Eligible children
// may overlap through the scheduler. A repeated name is a duplicate sibling
// declaration (§3.1), not a get-or-create — see failDuplicateSiblingLocked.
func (o *Output) Group(name string) *GroupHandle {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.declareContainerLocked(nil, name, false)
}

// Sequence declares a self-managing, ordered container — the front door for
// a sequence of steps that must stop implying "still might run" once a
// member has already failed or been cancelled. A repeated name is a
// duplicate sibling declaration (§3.1), not a get-or-create — see
// failDuplicateSiblingLocked.
func (o *Output) Sequence(name string) *SequenceHandle {
	o.mu.Lock()
	defer o.mu.Unlock()
	return &SequenceHandle{tasks: o.declareContainerLocked(nil, name, true)}
}

// declareContainerLocked declares a Group (sequential false) or Sequence
// under parent, or at the root when parent is nil — the one path every
// level shares, so one rule decides duplicates everywhere (§3.1). A nested
// container's identity is its parent's key plus its name, and it starts
// after the step before it when parent is a Sequence. A refusal comes back
// as a rejected handle. Callers must already hold o.mu.
func (o *Output) declareContainerLocked(parent *tasksState, name string, sequential bool) *GroupHandle {
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
		o.recordMisuse(ErrDeclaredInCallback)
		return o.rejectedGroup(ErrDeclaredInCallback)
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
	h := &GroupHandle{out: o, id: st.id}
	st.handle = h
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "tasks.declared", EntityID: st.id})
	o.emitCollectionDeclaredLocked(st, parentID)
	return h
}

// declareGroupTask declares a child task by name in the container g — the
// identity behind Group.Task/Sequence.Task. A repeated name is a duplicate
// sibling declaration (§3.1), not a get-or-create. A task declared under a
// refused container is refused for the same reason.
func (o *Output) declareGroupTask(g *GroupHandle, name string) *TaskHandle {
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
	Task(name string) *TaskHandle
	Group(name string) *GroupHandle
	Sequence(name string) *SequenceHandle
}

var (
	_ Container = (*Output)(nil)
	_ Container = (*GroupHandle)(nil)
	_ Container = (*SequenceHandle)(nil)
)

// GroupHandle is the front door for independent child work. Eligible
// children may overlap through the scheduler. The type cannot be named
// Group — that name is the constructor. Nest via Group/Sequence.
type GroupHandle struct {
	out *Output
	id  string
	// rejected is why the declaration was refused (see rejectedGroup); nil
	// for a declared Group or Sequence.
	rejected error
	// facade holds this handle's public wrapper (see FacadeSlot).
	facade FacadeSlot
}

// Task declares a child task. A repeated name is a duplicate sibling
// declaration, not a get-or-create (§3.1, Output.Task's doc comment): it
// fails the new call with ProblemCodeDuplicateSiblingName rather than
// returning the earlier handle. Callers that reference a Task again later
// (for example in After) must keep the first handle, typically in a typed
// variable, instead of re-declaring by name.
func (g *GroupHandle) Task(name string) *TaskHandle {
	if g == nil || g.out == nil {
		return &TaskHandle{}
	}
	return g.out.declareGroupTask(g, name)
}

// Sequence declares (or returns) an ordered child container nested here.
func (g *GroupHandle) Sequence(name string) *SequenceHandle {
	child := g.declareChild(name, true)
	return &SequenceHandle{tasks: child}
}

// Group declares (or returns) an independent child container nested here.
func (g *GroupHandle) Group(name string) *GroupHandle {
	return g.declareChild(name, false)
}

func (g *GroupHandle) declareChild(name string, sequential bool) *GroupHandle {
	if g == nil || g.out == nil {
		return &GroupHandle{}
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
func (g *GroupHandle) Summary(text string) *GroupHandle {
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
	g.out.appendEventLocked(Event{Type: "tasks.summary_set", EntityID: g.id})
	return g
}

// Snapshot returns the collection snapshot with derived state.
func (g *GroupHandle) Snapshot() TasksSnapshot {
	if g == nil || g.out == nil {
		return TasksSnapshot{State: Empty}
	}
	g.out.mu.Lock()
	defer g.out.mu.Unlock()
	col := g.out.containerStates[g.id]
	if col == nil {
		return TasksSnapshot{ID: g.id, State: Empty}
	}
	return col.snapshot()
}

// --- folded from names.go ---

// DefaultVisibleNames is how many names TruncateNames keeps before summarizing.
const DefaultVisibleNames = project.DefaultVisibleNames

// TruncateNames joins names for a skip/kept-style summary.
// Empty names yields "". visible <= 0 uses DefaultVisibleNames.
// When more names remain than visible, appends the overflow glyph for
// profile (evo-rec.md's tightened vocabulary: "… +N more", ASCII "... +N
// more") instead of a bare ", +N" that carries no glyph at all.
// profile is variadic so the simplest call — TruncateNames(names, visible) —
// stays correct: an omitted profile renders the Unicode overflow glyph; a
// caller that has already resolved a GlyphProfile passes it explicitly.
func TruncateNames(names []string, visible int) string {
	return project.TruncateNames(names, visible)
}

// --- folded from sequence.go ---

// SequenceHandle is the front door for ordered child work: the same
// scheduler as Group with implicit predecessor edges in declaration order.
// A failed/blocked/cancelled child makes later siblings NotStarted.
type SequenceHandle struct {
	tasks *GroupHandle
	// facade holds this handle's public wrapper (see FacadeSlot).
	facade FacadeSlot
}

// Task declares (or, for a repeated name, returns) a child task in
// declaration order.
func (g *SequenceHandle) Task(name string) *TaskHandle {
	if g == nil || g.tasks == nil {
		return &TaskHandle{}
	}
	return g.tasks.out.declareGroupTask(g.tasks, name)
}

// Summary sets a success-oriented sequence summary.
func (g *SequenceHandle) Summary(text string) *SequenceHandle {
	if g == nil || g.tasks == nil {
		return g
	}
	g.tasks.Summary(text)
	return g
}

// Snapshot returns the sequence snapshot with derived state.
func (g *SequenceHandle) Snapshot() TasksSnapshot {
	if g == nil || g.tasks == nil {
		return TasksSnapshot{State: Empty}
	}
	return g.tasks.Snapshot()
}

// Sequence declares (or returns) an ordered child container nested here.
func (g *SequenceHandle) Sequence(name string) *SequenceHandle {
	if g == nil || g.tasks == nil {
		return &SequenceHandle{}
	}
	return g.tasks.Sequence(name)
}

// Group declares (or returns) an independent child container nested here.
func (g *SequenceHandle) Group(name string) *GroupHandle {
	if g == nil || g.tasks == nil {
		return &GroupHandle{}
	}
	return g.tasks.Group(name)
}

// --- folded from task_annotate.go ---

// Doing sets the active current-step live text and starts the task if
// pending — replaces the previous text, promotes the task to Running, and
// becomes a durable line per step off-TTY. text is a printf format when args
// are present (fmt.Sprintf semantics). Chained right after Task, it sets
// the first step at declaration.
func (t *TaskHandle) Doing(text string, args ...any) *TaskHandle {
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
func resolvedByInterrupt(state EntityState) bool {
	return state == Cancelled || state == NotStarted
}

// annotate is the one guard every non-terminal annotation verb (Doing,
// Progress, Bytes, Summary, Problem, Fact) shares: under o.mu,
// it applies apply to the task's state only while the task is open. On a
// closed Output it records that misuse; on a terminal row it records
// ErrAlreadyResolved, unless the interrupt sweep resolved the row (see
// resolvedByInterrupt). It returns t so each verb can chain.
func (t *TaskHandle) annotate(apply func(st *taskState)) *TaskHandle {
	return t.withTask(func(st *taskState) {
		if err := t.out.ensureOpen(); err != nil {
			t.out.recordMisuse(err)
			return
		}
		if record.IsTerminalTask(st.node.Rec.State()) {
			if !resolvedByInterrupt(st.node.Rec.State()) {
				t.out.recordMisuseFor(st.name, ErrAlreadyResolved)
			}
			return
		}
		apply(st)
	})
}

// withTask is the one lock-and-lookup every TaskHandle verb that
// edits its row goes through: under o.mu it hands apply the task's state,
// and does nothing for a nil or zero TaskHandle or a task this Output does
// not know. That is where "a nil *TaskHandle is safe" is kept, for every
// verb at once. It returns t so each verb can chain.
func (t *TaskHandle) withTask(apply func(st *taskState)) *TaskHandle {
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
// output. Off-TTY, an explicit TaskHandle.Doing call
// still forces its own durable row (the P10 contract: the one line the
// caller asked to see); this path never does — a child's full output already
// has one durable home, the evidence ring (and its failure-path DetailTail),
// so a row per mirrored line would just repeat it (release-gate round 9
// finding 4).
func (t *TaskHandle) setLiveOnlyPhase(text string) {
	t.annotate(func(st *taskState) { t.out.setLiveOnlyPhaseLocked(st, text) })
}

// setPhaseLocked is Doing's locked body. Callers must already hold o.mu and
// have checked ensureOpen/isTerminalTask.
func (o *Output) setPhaseLocked(st *taskState, text string) {
	st.node.Rec.SetPhase(text)
	st.node.Rec.MarkActivity(o.cfg.clock.Now())
	if st.node.Rec.State() == Pending {
		o.promoteRunningLocked(st)
		st.node.Rec.EnsureIndeterminateProgress()
	}
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "task.phase_changed", EntityID: st.id})
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
func (o *Output) setLiveOnlyPhaseLocked(st *taskState, text string) {
	// Identical live-only phase is a no-op: Writer leftover/repeated lines
	// must not bump, emit task.phase_changed, or force a live paint.
	if !st.node.Rec.SetPhaseIfChanged(text) {
		return
	}
	st.node.Rec.MarkActivity(o.cfg.clock.Now())
	if st.node.Rec.State() == Pending {
		o.promoteRunningLocked(st)
		st.node.Rec.EnsureIndeterminateProgress()
	}
	o.bumpLocked()
	o.appendEventLocked(Event{Type: "task.phase_changed", EntityID: st.id})
	o.signalLiveLocked(true)
}

// recordWarning is the warning-severity Problem path: st.warnings,
// task.warned, EventWarningRecorded, censusWarned. Public Warn was removed in 1.1.
func (t *TaskHandle) recordWarning(p Problem) *TaskHandle {
	return t.annotate(func(st *taskState) {
		if st.node.Rec.AppendWarning(p) == 1 {
			st.censusWarned()
		}
		t.out.bumpLocked()
		t.out.appendEventLocked(Event{Type: "task.warned", EntityID: t.id})
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
// annotate). GroupHandle.Summary is the same shape one level up.
func (t *TaskHandle) Summary(text string) *TaskHandle {
	return t.annotate(func(st *taskState) {
		st.node.Rec.SetSummary(text)
		t.out.bumpLocked()
		t.out.appendEventLocked(Event{Type: "task.summary_set", EntityID: t.id})
		t.out.signalLiveLocked(true)
	})
}

// Problem appends one structured diagnostic without resolving the Task.
// Severity defaults to SeverityError: a Task holding an error Problem can
// never settle success-class (see Task.HonestOutcome). SeverityWarning uses the
// previous warning projection and never fails the Task. An invalid severity
// is rejected with a context-bearing misuse error and is not recorded.
func (t *TaskHandle) Problem(summary string, opts ...ProblemOption) *TaskHandle {
	p := record.ApplyProblemOptions(project.Text(summary), opts)
	sev, err := record.ClassifiedProblemSeverity(p)
	if err != nil {
		return t.annotate(func(st *taskState) { t.out.recordMisuse(err) })
	}
	if sev == SeverityWarning {
		return t.recordWarning(p)
	}
	return t.recordBlockingProblem(p)
}

func (t *TaskHandle) recordBlockingProblem(p Problem) *TaskHandle {
	return t.annotate(func(st *taskState) {
		st.node.Rec.AppendProblems(p)
		t.out.bumpLocked()
		t.out.appendEventLocked(Event{Type: "task.problem_recorded", EntityID: t.id})
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
func (t *TaskHandle) Fact(name, value string) *TaskHandle {
	f := record.SanitizeFact(FactRecord{Name: project.Text(name), Value: project.Text(value)})
	return t.annotate(func(st *taskState) {
		st.node.Rec.AppendFact(f)
		t.out.bumpLocked()
		t.out.emitWireEventLocked(wire.EventFactRecorded, t.id, wire.ToFactDoc(f).EventPayload())
		t.out.signalLiveLocked(true)
	})
}

// --- folded from task_commit.go ---

// resolveProblems folds extra into st's Problems as it settles into state.
// A Failed or Blocked row's Problems gain the capture tail the Task already
// gathered, so the detail a caller collected through evidence()/PhaseWriter()
// needs no opt-in (beginner-2). The capture is read only when some Problem
// lacks its own detail, which also avoids re-entering the redactor lock this
// resolution already holds for a pending tail.
func (st *taskState) resolveProblems(state EntityState, extra []Problem) {
	tail := ""
	if st.node.Rec.NeedsEvidenceTail(state, extra) && st.evidence != nil && !st.evidence.Empty() {
		tail = st.evidence.detailText()
	}
	st.node.Rec.ResolveProblems(state, extra, tail)
}

// commitSettledLocked writes a settled Task where its readers find it. A
// Group or Sequence child repaints the live ledger; a standalone Task
// commits its row and its own File/Exec ledger rows to scrollback at once,
// so later Printf/Confirm output can never land above finished work. A
// Done Task commits its manifest record; Failed, Blocked, and Cancelled
// never do (spec §8.2/§11.3).
func (o *Output) commitSettledLocked(st *taskState) {
	if st.collection() != nil {
		o.signalLiveLocked(true)
	} else {
		o.commitResolvedTaskLocked(st.id)
		o.commitNamedEffectsLocked(st.id)
	}
	if st.node.Rec.State() != Done {
		return
	}
	o.commitManifestTaskLocked(o.graph.Context(), st.id)
}

// --- folded from task_next.go ---

// nextSelf is a ProblemOption that attaches a command action re-running the
// caller's own binary with args — a self-referencing remedy ("rerun with
// --apply") that doesn't restate which binary to run (I6). Uses the same
// identity source as Confirm's PolicyFlag / I2's Fail fallback: Config.Title
// when set, else the binary's own basename. Use NextCommand instead when the
// remedy is a different (foreign) tool.
func (o *Output) nextSelf(args ...string) ProblemOption {
	return NextCommand(o.policySourceName(), args...)
}

// --- folded from task_progress.go ---

// Progress sets absolute completed/total count progress.
// Counts use int (collection lengths, indices). For byte quantities use Bytes.
// Prefer absolute Progress over Advance so retries cannot double-count.
func (t *TaskHandle) Progress(completed, total int) *TaskHandle {
	return t.setProgress(int64(completed), int64(total), Determinate)
}

// Bytes sets absolute byte progress (units and rate formatting).
func (t *TaskHandle) Bytes(completed, total int64) *TaskHandle {
	return t.setProgress(completed, total, BytesKind)
}

func (t *TaskHandle) setProgress(completed, total int64, kind ProgressKind) *TaskHandle {
	return t.annotate(func(st *taskState) { t.applyProgressLocked(st, completed, total, kind) })
}

// applyProgressLocked reports whether the update was applied — false means a
// guard (invalid values, regression, sealed-total mismatch) rejected it and
// recorded misuse instead, so a paired live update is not applied for a progress change that never happened
// (e.g. Phase) that would otherwise describe a progress change that never
// happened.
func (t *TaskHandle) applyProgressLocked(st *taskState, completed, total int64, kind ProgressKind) bool {
	switch st.node.Rec.ApplyProgress(completed, total, kind) {
	case record.ProgressInvalid:
		t.out.recordMisuse(ErrInvalidProgress)
		return false
	case record.ProgressRegressed:
		t.out.recordMisuse(ErrProgressRegression)
		return false
	}
	st.node.Rec.MarkActivity(t.out.cfg.clock.Now())
	if st.node.Rec.State() == Pending {
		t.out.promoteRunningLocked(st)
	}
	t.out.bumpLocked()
	t.out.appendEventLocked(Event{Type: "task.progress_changed", EntityID: t.id})
	// Progress is high-frequency: coalesce unless first frame.
	t.out.signalLiveLocked(false)
	t.out.emitTaskRunningProgressiveLocked(st, triggerProgress)
	return true
}

// --- folded from task_resolve.go ---

// succeed resolves the task Done with summary: the engine's synchronous
// success verb for library-owned rows (Confirm's gate) and engine tests.
// The public TaskHandle has no equivalent since 1.1: callers resolve
// through Define, and Summary carries their result project.
func (t *TaskHandle) succeed(summary string) {
	t.finish(Done, project.Text(summary), nil)
}

// Fail resolves the task as failed. This is a statement, not a fluent
// chain — Fail returns nothing, so a bare `task.Fail("summary")` is
// errcheck-clean. A nil *TaskHandle is safe and resolves nothing. Inside
// Define, return the error as well so Wait sees the callback's own result.
func (t *TaskHandle) Fail(summary string, options ...ProblemOption) {
	t.resolveWithProblem(Failed, summary, options)
}

// resolveWithProblem is Fail and Block: resolve as state with one Problem
// built from summary and options.
func (t *TaskHandle) resolveWithProblem(state EntityState, summary string, options []ProblemOption) {
	p := record.ApplyProblemOptions(project.Text(summary), options)
	t.finish(state, project.Text(summary), []Problem{p})
}

// Block resolves the task as blocked. This is a statement, not a fluent
// chain — Block returns nothing, so a bare `task.Block("summary")` is
// errcheck-clean. A nil *TaskHandle is safe and resolves nothing. Inside
// Define, Block then return nil: Wait still reports failure from the
// Blocked row.
func (t *TaskHandle) Block(summary string, options ...ProblemOption) {
	t.resolveWithProblem(Blocked, summary, options)
}

// Cancel resolves the task as cancelled. A nil *TaskHandle is safe and
// resolves nothing.
func (t *TaskHandle) Cancel(reason string) {
	t.finish(Cancelled, project.Text(reason), nil)
}

// skip resolves the task as skipped (the engine body behind Skipped).
// reason is a printf format when args are present (fmt.Sprintf semantics).
func (t *TaskHandle) skip(reason string, args ...any) *TaskHandle {
	if len(args) > 0 {
		reason = fmt.Sprintf(reason, args...)
	}
	return t.finish(Skipped, project.Text(reason), nil)
}

// failScheduled resolves the task Failed on the scheduler's authority,
// carrying the callback's error text as the row summary — Fail's shape,
// without Fail's caller-side submitted-task guard.
func (t *TaskHandle) failScheduled(summary string) {
	p := record.ApplyProblemOptions(project.Text(summary), nil)
	t.resolveScheduled(Failed, project.Text(summary), []Problem{p})
}

// doneScheduled resolves the task Done on the scheduler's authority — the
// one path that may declare a submitted task successful (see finish).
func (t *TaskHandle) doneScheduled() {
	t.resolveScheduled(Done, "", nil)
}

// finish is the caller-facing resolution path — every terminal verb
// (Fail/Block/Cancel/Skipped) a program writes by hand.
//
// Once a task is submitted, the scheduler owns the verdict on its work: only
// the callback's own return value says whether the work succeeded. So a
// hand-written Skipped on a submitted task is a *proposal*, not a
// resolution — it is held until the callback returns and then either
// ratified (the work did succeed; the caller's own summary is what renders,
// which is how a callback declares "✓ branches  8 deleted") or rejected as
// ErrAlreadyResolved misuse, with the observed failure taking the row (P2:
// an engine-side success resolution can no longer launder an error into a
// green row). Nothing ratifies its own completion.
//
// Bad news needs no ratification: Fail/Block/Cancel state an outcome the
// caller already knows and can only make the row worse, so they resolve
// immediately — that is how a callback reports its own failure (P13, where
// the scheduler must then not resolve it a second time) and how an interrupt
// cancels a running row.
func (t *TaskHandle) finish(state EntityState, summary string, problems []Problem) *TaskHandle {
	return t.resolve(state, summary, problems, graph.ByCaller)
}

// resolveScheduled is the scheduler's own resolution path, called only from
// executeWork once the callback has returned. It is the sole authority that
// may declare a submitted task successful.
func (t *TaskHandle) resolveScheduled(state EntityState, summary string, problems []Problem) {
	t.resolve(state, summary, problems, graph.ByScheduler)
}

// deniesItsOwnEffect reports whether this resolution is an evo.Effect
// callback disowning the work it was given: an Effect creating "module"
// whose fn calls Skipped or Fail and then returns nil rendered both `! skipped 1
// (install failed)` and `[changed] broken  created 1 module` — the ledger
// counting the package the installer had just rejected. A nil return after
// the row said "skipped" means "I handled it", not "I did it".
//
// Only the callback's own verdict counts. A later Fail from the program
// (an Effect in Define, then `task.Fail(...)`) and an interrupt that
// cancels a running Effect both describe work that really happened,
// and both still owe the reader `! already mutated: …`. The separator is
// the resolving goroutine's own stack: callbackDepth is non-zero only
// inside a task callback, which is precisely "the row resolved itself".
func (o *Output) deniesItsOwnEffect(st *taskState, state EntityState, authority graph.Authority) bool {
	if !o.changes.EffectInFlight(record.TaskID(st.id)) || st.node.Phase() != graph.PhaseRunning || authority != graph.ByCaller || state == Done {
		return false
	}
	return graph.CallbackDepth() > 0
}

// resolve is the one path behind every terminal verb. The graph decides what
// the resolution does to the Task (see graph.Graph.Admit); this writes what
// the reader sees of it: the Summary, the Problems and the committed row.

func (t *TaskHandle) resolve(state EntityState, summary string, problems []Problem, authority graph.Authority) *TaskHandle {
	if t == nil || t.out == nil {
		return t
	}
	t.out.holdRunningPaint(t.id)
	t.out.mu.Lock()
	defer t.out.mu.Unlock()
	st := t.out.taskStates[t.id]
	if st == nil {
		return t
	}
	if err := t.out.ensureOpen(); err != nil {
		t.out.recordMisuse(err)
		return t
	}
	admission := t.out.graph.Admit(st.node, graph.Resolution{State: state, Summary: summary, Problems: problems, By: authority})
	if admission.Verdict == graph.AlreadyResolved {
		t.out.recordAlreadyResolvedLocked(st.name, summary)
		return t
	}
	if t.out.deniesItsOwnEffect(st, state, authority) {
		t.out.changes.DenyEffect(record.TaskID(st.id))
	}
	if admission.Verdict == graph.Proposed {
		return t
	}
	if summary != "" {
		st.node.Rec.SetSummary(summary)
	}
	st.resolveProblems(admission.State, problems)
	t.out.graph.Conclude(st.node, admission)
	t.out.followRecordLocked()
	t.out.emitWireEventLocked(wire.EventTaskFinished, t.id, taskFinishedPayload(st))
	t.out.commitSettledLocked(st)
	return t
}

// taskFinishedPayload is the task.finished event's payload: the settled
// state, why it settled, and its Summary when it has one (ZYS-971).
func taskFinishedPayload(st *taskState) map[string]any {
	payload := map[string]any{
		"state":      string(st.node.Rec.State()),
		"resolution": string(st.node.Rec.Resolution()),
	}
	if st.node.Rec.Summary() != "" {
		payload["summary"] = st.node.Rec.Summary()
	}
	return payload
}

// --- folded from task_run.go ---

// Run is the ordinary way to shell out from a Task: it executes cmd as this
// task's subprocess, wiring cmd.Stdout/cmd.Stderr through the same evidence +
// PhaseWriter plumbing PhaseWriter uses directly. Each line becomes the
// task's live Phase, and every byte is retained (redacted, bounded) in the
// task's evidence ring so DetailTail has proof after Fail — reach for
// evidence directly only when the caller isn't running a process.Cmd. If
// cmd.Stdout/cmd.Stderr already point somewhere (a caller wiring its own log
// file, say), Run tees into it rather than replacing it.
//
// If the task has no Phase yet, Run sets one from cmd's basename
// (filepath.Base(cmd.Path) or cmd.Args[0]) so a live view shows what's
// running before the child ever writes a line.
//
// Run adds no context handling of its own and leaves cmd's standard input
// unset.
//
// Run returns the subprocess error verbatim and never resolves the task —
// the caller's Define callback turns the result into the outcome:
//
//	cmd := process.NewCmd("go", "build", "./...")
//	if err := task.run(cmd); err != nil {
//	    return fmt.Errorf("build failed: %w", err)
//	}
//	return nil
func (t *TaskHandle) run(cmd *process.Cmd) error {
	if t != nil && t.out != nil {
		t.ensurePhase(commandPhaseName(cmd))
		pw := &phaseWriter{task: t, evidence: t.Capture()}
		cmd.SetStdout(teeSubprocessWriter(cmd.Stdout(), pw))
		cmd.SetStderr(teeSubprocessWriter(cmd.Stderr(), pw))
	}
	return cmd.Run()
}

// shellWrapperBasenames names common shell/interpreter wrappers whose own
// basename is a meaningless phase — the actual work lives in a -c/-Command
// script argument, not the wrapper's own name. FP-004 ("no placeholder
// phase") applies to Run itself: exec.Command("sh", "-c", "go build ./...")
// must not publish "sh" as the live phase (beginner-11).
var shellWrapperBasenames = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "ksh": true, "dash": true,
	"cmd": true, "cmd.exe": true, "powershell": true, "powershell.exe": true, "pwsh": true,
}

// commandPhaseName is the human phase text for a not-yet-run *exec.Cmd: the
// basename of the resolved path when set, else the first argv element —
// exec.Command("go", "build") and exec.Command("/usr/bin/go", "build") both
// read "go", not a full path. A shell wrapper's own basename is skipped in
// favor of its -c/-Command script's first word; when neither is available,
// commandPhaseName returns "" and ensurePhase defers to first output rather
// than publish a placeholder.
func commandPhaseName(cmd *process.Cmd) string {
	name := ""
	switch {
	case cmd.Path() != "":
		name = filepath.Base(cmd.Path())
	case len(cmd.Args()) > 0:
		name = filepath.Base(cmd.Args()[0])
	}
	if name == "" || !shellWrapperBasenames[name] {
		return name
	}
	if script := shellScriptCommandName(cmd.Args()); script != "" {
		return script
	}
	return ""
}

// shellScriptCommandName returns the basename of the first word of a shell
// -c (or Windows -Command) script argument, so "sh -c 'go build ./...'"
// reads "go", not "sh" — but only when the script is a single simple command.
// A compound script (newline, ;, &&, or |) has no one "first word" that
// honestly describes the whole script — "cd /tmp && make" naming an initial
// phase of "cd" would be a placeholder as misleading as the wrapper's own
// basename FP-004 already guards against (release-gate round 5 finding 7).
// commandPhaseName returns "" instead, and Run's ensurePhase defers to the
// child's first output line.
func shellScriptCommandName(args []string) string {
	for i, a := range args {
		if (a == "-c" || a == "-Command") && i+1 < len(args) {
			script := args[i+1]
			if !isSingleSimpleCommand(script) {
				return ""
			}
			fields := strings.Fields(script)
			if len(fields) > 0 {
				return filepath.Base(fields[0])
			}
		}
	}
	return ""
}

// compoundShellOperators are substrings that mark a shell script as more
// than one simple command — the presence of any of these means no single
// "first word" honestly names the whole script.
var compoundShellOperators = []string{"\n", ";", "&&", "|"}

// isSingleSimpleCommand reports whether script contains none of
// compoundShellOperators.
func isSingleSimpleCommand(script string) bool {
	for _, op := range compoundShellOperators {
		if strings.Contains(script, op) {
			return false
		}
	}
	return true
}

// ensurePhase sets phase only when the task has none yet, so Run never
// overwrites a phase the caller already declared before calling it.
func (t *TaskHandle) ensurePhase(phase string) {
	if phase == "" || t.Snapshot().Phase != "" {
		return
	}
	t.Doing(phase)
}

// teeSubprocessWriter adds extra to existing without discarding it — an
// already-wired cmd.Stdout/cmd.Stderr keeps receiving output, and Run's own
// capture/phase plumbing observes the same bytes.
func teeSubprocessWriter(existing io.Writer, extra io.Writer) io.Writer {
	if existing == nil {
		return extra
	}
	return io.MultiWriter(existing, extra)
}

// --- folded from task_state.go ---

// key is the Task's §3.1 stable key, empty for a Task the graph never declared.
func (st *taskState) key() string { return st.node.Key() }

// TaskID is the Task's id in the run record; with ManifestKey it makes
// taskState the freshness.TaskView freshness judges the Task through.
func (st *taskState) TaskID() record.TaskID { return record.TaskID(st.id) }

// ManifestKey is the Task's stable key in the manifest.
func (st *taskState) ManifestKey() string { return st.key() }

// neverDefined reports whether st is still waiting on its caller: declared,
// never Defined, and not resolved by a verb either.
func (st *taskState) neverDefined() bool { return st.node.NeverDefined() }

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
	handle *TaskHandle

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
	followed EntityState
	// settleAnswered is set once the render work a settle owes was queued.
	settleAnswered bool
	// runningAnnounced is set once the render work becoming Running owes was
	// done, so task.started fires once however the row reached Running.
	runningAnnounced bool

	// ownsWindow is set on a Confirm gate while its Suspend window is open
	// (guarded by Output.mu): the window shows this Task's rows, so they are
	// not held back like a sibling's.
	ownsWindow bool

	// filing is where this Task stands in its collection's childIndex.
	filing filing

	// heartbeat is the §40 plain-mode durable heartbeat's state.
	heartbeat plainHeartbeat

	// capture is the get-or-create sink shared by Task.Capture and PhaseWriter
	// so child-process evidence recorded via either path lands in one ring and
	// DetailTail sees it after Fail.
	evidence *evidence
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
	out *Output
	// node is the graph's declaration of this Group/Sequence and owns its
	// §3.1 stable key.
	node   *graph.Container
	name   string
	handle *GroupHandle

	// kids is what a live frame and the verdict read of tasks.
	kids childIndex

	// runningSteps holds the children promoteRunningLocked moved to
	// Running that may still be Running (pruned on each promotion), so the
	// "one Running child" check never rescans every step.
	runningSteps []*taskState

	// path caches containerPath: a container's place in the tree is fixed at
	// declaration, and every child's ledger section shares the one slice.
	path record.ContainerPath
	// census counts its descendant Tasks for the live frame (liveCensus).
	census liveCensus
	// hasNamesake records that a child Task carries this container's own
	// name, the only way it can render as its own Task (liveOwnRow).
	hasNamesake bool
}

// containerPath is the chain of containers enclosing st, nearest first;
// nil for a root Task.
func (st *taskState) containerPath() record.ContainerPath {
	if st.collection() == nil {
		return nil
	}
	return st.collection().containerPath()
}

// containerPath is c then every container above it.
func (c *tasksState) containerPath() record.ContainerPath {
	if c.path == nil {
		c.path = record.ContainerPath{{ID: c.id, Name: c.name}}
		if c.parent() != nil {
			c.path = append(c.path, c.parent().containerPath()...)
		}
	}
	return c.path
}

// --- folded from task_taxonomy.go ---

// dispositionVerb names which accumulation act a Reason's usage constraints
// are checked against — TaskHandle.Skipped, or the keep records Kept used
// before it was removed in 1.1.
type dispositionVerb string

const (
	dispositionSkip dispositionVerb = "skip"
	dispositionKeep dispositionVerb = "keep"
)

// wireName is v's record kind on the machine wire.
func (v dispositionVerb) wireName() string {
	if v == dispositionSkip {
		return wire.DispositionSkipped
	}
	return wire.DispositionKept
}

// Skipped accumulates a (reason, name) skip record on the task, with an
// optional trailing errs for evidence of why. It returns nothing —
// accumulating a record is an act, not a value to chain — is usable before
// the task resolves, and does not itself resolve the task. The taxonomy line
// ("- skipped N (a reasonA, b reasonB)") is derived from every accumulated
// record at render time, so no caller can hand-build (and thereby miscount)
// the summary; the aggregation key is untouched by errs. Any errs render as
// one bounded evidence line under the count row (first cause + "(+N more)"),
// full list under Verbose.
func (t *TaskHandle) Skipped(reason TaxonomyReason) {
	t.recordTaxonomy(reason, "", dispositionSkip, nil)
	t.finish(Skipped, "", nil)
}

func (t *TaskHandle) recordTaxonomy(reason TaxonomyReason, name string, verb dispositionVerb, errs []error) {
	t.withTask(func(st *taskState) { t.recordTaxonomyLocked(st, reason, name, verb, errs) })
}

func (t *TaskHandle) recordTaxonomyLocked(st *taskState, reason TaxonomyReason, name string, verb dispositionVerb, errs []error) {
	if err := t.out.ensureOpen(); err != nil {
		t.out.recordMisuse(err)
		return
	}
	if record.IsTerminalTask(st.node.Rec.State()) {
		t.out.recordMisuseFor(st.name, ErrAlreadyResolved)
		return
	}
	t.out.enforceReasonConstraintLocked(reason, st.name, verb)
	if name == "" {
		name = st.name
	}
	rec := TaxonomyRecord{Reason: reason.Name(), Name: project.Text(name), Causes: causesFromErrors(errs)}
	switch verb {
	case dispositionSkip:
		st.node.Rec.AppendSkipped(rec)
	case dispositionKeep:
		st.node.Rec.AppendKept(rec)
	}
	t.out.bumpLocked()
	t.out.appendEventLocked(Event{Type: "task." + string(verb) + "_recorded", EntityID: t.id})
	t.out.emitWireEventLocked(wire.EventDispositionRecorded, t.id, wire.ToDispositionDoc(verb.wireName(), rec).EventPayload())
}

// causesFromErrors renders each non-nil err's text, sanitized like every
// other human-visible taxonomy field, for TaxonomyRecord.Causes.
func causesFromErrors(errs []error) []string {
	if len(errs) == 0 {
		return nil
	}
	var out []string
	for _, err := range errs {
		if err == nil {
			continue
		}
		out = append(out, project.Text(err.Error()))
	}
	return out
}

// enforceReasonConstraintLocked records misuse when reason's declared
// constraints (ForSkip, OnTask) don't match how it is being used here.
// Strict panics via recordMisuse; production still counts the record —
// a constraint violation degrades to "counted anyway", never a dropped truth.
func (o *Output) enforceReasonConstraintLocked(reason TaxonomyReason, taskName string, verb dispositionVerb) {
	if reason.SkipOnly() && verb != dispositionSkip {
		o.recordMisuse(ErrReasonSkipOnly)
	}
	if reason.OnlyOnTask() != "" && reason.OnlyOnTask() != taskName {
		o.recordMisuse(ErrReasonWrongTask)
	}
}

// --- folded from wait.go ---

// Wait blocks until the task is terminal and returns the error its callback
// returned (nil on Done, Skipped, or a dry run that never invoked it).
//
// A task that was already resolved before it was defined never runs its
// callback (the misuse is recorded at the Define call), and Wait returns nil
// immediately rather than blocking on work that will never happen (P15).
//
// A task that never ran — an abandoned queue, a predecessor that failed —
// returns ErrNotStarted rather than nil, and a cancelled one returns its
// cancellation: Wait never reports success for work that did not happen.
//
// Wait called while the calling goroutine holds a resource claim (inside an
// Effect, File, or Basis hold) returns ErrNestedResourceAcquisition without
// waiting: the awaited work could need that claim, and neither could move.
//
// A waiter has stopped doing work, so the concurrency ceiling must not be
// the reason the task it waits on cannot start (P16). Wait therefore runs
// that task, and whatever is holding it back, on its own goroutine when the
// scheduler has not picked them up (see graph.Graph.RunWaited): nested
// Define+Wait completes even at MaxConcurrency 1, because the waiting
// callback's slot carries the work it is waiting for instead of idling.
//
// MaxConcurrency bounds every executing callback. A plain caller (one
// outside any callback) holds no slot, so it runs work only in a free slot
// and otherwise waits for the pool. A goroutine a callback started holds
// none either: if that callback blocks on it while every slot is held,
// neither can move, and Wait returns ErrWaitDeadlock naming the shape
// instead of hanging. Declare that work as Tasks under a Group, or Wait
// inside the callback itself (API-041).
func (t *TaskHandle) Wait() error {
	if t == nil || t.out == nil {
		return nil
	}
	if t.rejected != nil {
		return rejectedWaitOutcome(t.rejected)
	}
	node := t.out.graph.Task(t.id)
	if node == nil {
		return nil
	}
	outcome := t.out.graph.Wait(node)
	t.out.followRecordAfterWait()
	return outcome
}

// followRecordAfterWait is what a Wait owes its caller once the graph let it
// go: the render state has answered every settle the Wait saw. A Task's done
// channel closes inside the graph's critical section, before the resolution
// that settled it has painted and filed the row. Taking o.mu here waits out
// that resolution (it holds o.mu from settle to commit) and follows the
// record for a settle made outside one, so a caller that reads the frame
// right after Wait sees the settled row.
func (o *Output) followRecordAfterWait() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.followRecordLocked()
}
