package evo

import (
	"github.com/zachbornheimer/evident-output/v2/internal/freshness"
	"github.com/zachbornheimer/evident-output/v2/internal/graph"
	txt "github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// TaskDeclared registers a just-declared Task's row, live index, census place, events, and a forced paint.
func (e runHost) TaskDeclared(t *graph.Task) {
	o := e.out
	st := &taskState{id: t.ID, node: t, name: t.Name}
	st.handle = &Task{out: o, id: st.id, state: st}
	o.taskStates[st.id] = st
	o.appendTaskLocked(st, false)
}

// GateDeclared registers the gate of a container builder: no row, no collection, followed like any Task.
func (e runHost) GateDeclared(t *graph.Task) {
	o := e.out
	gate := &taskState{id: t.ID, node: t, name: t.Name}
	gate.handle = &Task{out: o, id: gate.id}
	o.taskStates[gate.id] = gate
	o.show.DeclareGate(t.TaskID(), t.Rec, t.GateFor().ID)
}

func (e runHost) ContainerDeclared(c *graph.Container) {
	o := e.out
	st := &tasksState{id: c.ID, out: o, node: c, name: c.Name}
	parentID := ""
	if c.Parent == nil {
		o.collections = append(o.collections, st)
	} else {
		parentID = c.Parent.ID
	}
	o.containerStates[st.id] = st
	st.handle = &Group{out: o, id: st.id}
	o.show.DeclareGroup(txt.DeclaredGroup{
		ID: c.ID, Name: c.Name, Declaration: c.Declaration, Sequential: c.Sequential, Rec: c.Rec, Node: c,
	}, parentID)
}

// Started is the render work a claimed Task owes (eligibility event, Running, snapshot version, forced
// paint). It reports false when an interrupt settled the Task first, so the scheduler skips its callback.
func (e runHost) Started(t *graph.Task) bool {
	if t.IsGate() {
		return true
	}
	o := e.out
	o.mu.Lock()
	defer o.mu.Unlock()
	o.followRecordLocked()
	st := o.stateOf(t)
	if st == nil || record.IsTerminalTask(t.Rec.State()) {
		// An interrupt settled the Task between its claim and this step: never started.
		return false
	}
	o.emitWireEventLocked(wire.EventTaskEligible, st.id, wire.EmptyPayload{}.EventPayload())
	if t.Rec.State() == pending {
		o.promoteRunningLocked(st)
	} else {
		o.announceRunningLocked(st)
	}
	o.bumpLocked()
	o.show.Live.Signal(true)
	return true
}

func (e runHost) Resolving(t *graph.Task) { e.out.show.Live.HoldRunning(record.TaskID(t.ID)) }

// AlreadyResolved records a second terminal verb on t as misuse, keeping the rejected summary.
func (e runHost) AlreadyResolved(t *graph.Task, summary string) {
	e.out.recordAlreadyResolvedLocked(t.Name, summary)
}

// Disowned disowns every Effect in flight for t: an Effect whose callback resolved its own Task as
// anything but Done and returned nil handled the failure itself, so its work must not reach the ledger.
func (e runHost) Disowned(t *graph.Task) {
	if id := t.TaskID(); e.out.changes.EffectInFlight(id) {
		e.out.changes.DenyEffect(id)
	}
}

// Settled writes the settled Task where readers find it. Only a Done Task commits its manifest record.
func (e runHost) Settled(t *graph.Task) {
	st := e.out.fileSettledLocked(t)
	if st != nil && t.Rec.State().Succeeded() {
		e.out.commitManifestLocked(st)
	}
}

// Overturned writes the Task's second settle, Failed after Done, where readers find it, and takes back the
// manifest record its first settle committed: the next Run must not skip work this Run found to conflict.
func (e runHost) Overturned(t *graph.Task) {
	if st := e.out.fileSettledLocked(t); st != nil {
		e.out.freshness.Retract(t, freshness.ModeFor(e.out.cfg.dryRun))
	}
}

// fileSettledLocked writes the settled Task where readers find it and returns its state, nil when it has none.
// A Group or Sequence child repaints the live ledger and, in plain text, writes with its collection once that settles;
// a standalone Task commits its rows to scrollback at once so later Printf/Confirm output never lands above finished
// work. Callers hold r.mu.
func (r *Run) fileSettledLocked(t *graph.Task) *taskState {
	r.followRecordLocked()
	st := r.stateOf(t)
	if st == nil {
		return nil
	}
	r.show.Stream.Commit(record.TaskID(st.id))
	if st.collection() != nil {
		r.show.Live.Signal(true)
	} else {
		r.show.Stream.CommitEffects(st.id)
	}
	return st
}

// commitManifestLocked persists st's operations once it settled Done and narrates the commit. Callers hold r.mu.
func (r *Run) commitManifestLocked(st *taskState) {
	commit := r.freshness.Commit(r.graph.Context(), st.node, freshness.ModeFor(r.cfg.dryRun))
	if commit.Err != nil {
		r.warnManifestUnsavedLocked(commit.Err)
		return
	}
	if commit.Written {
		r.emitWireEventLocked(wire.EventManifestTaskCommitted, st.id,
			wire.ManifestTaskCommittedPayload{Operations: commit.Operations}.EventPayload())
	}
}

// RunEnded registers the synthetic Task carrying the run's own ending, already settled.
func (e runHost) RunEnded(t *graph.Task) {
	o := e.out
	o.appendTaskLocked(&taskState{id: t.ID, node: t, name: t.Name}, true)
}
