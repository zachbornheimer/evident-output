package evo

import (
	"github.com/zachbornheimer/evident-output/v2/internal/freshness"
	"github.com/zachbornheimer/evident-output/v2/internal/graph"
	txt "github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

func (r *Run) manifestConfig() freshness.ManifestConfig {
	return freshness.ManifestConfig{AppID: r.cfg.appID, StateDir: r.cfg.stateDir, Workspace: r.changes.Workspace()}
}

// promoteRunningLocked moves a Pending task to Running as the scheduler starts its callback, the only way a Task
// becomes Running (CONTRACT.md section 4). Narration it carried while it waited shows from here.
func (r *Run) promoteRunningLocked(st *taskState) {
	st.node.MarkRunning()
	r.announceRunningLocked(st)
}

// announceRunningLocked is the render state a Task owes for becoming Running, answered once per Task.
// A Sequence records misuse when a sibling is already Running; a Group's children are not policed.
func (r *Run) announceRunningLocked(st *taskState) {
	if !r.show.Started(st.node.TaskID()) {
		return
	}
	if seq := st.node.Parent; seq != nil && seq.Sequential && seq.StepStarted(st.node) {
		r.recordMisuse(errConcurrentRunning)
	}
}

func (r *Run) bumpLocked() { r.show.Bump() }

// appendEventLocked journals e in the order things happened: the record is followed first, so a settle a graph
// goroutine made on its own is journaled before an event this section writes after it.
func (r *Run) appendEventLocked(e recEvent) {
	r.followRecordLocked()
	e.Timestamp = r.cfg.clock.Now()
	e.SchemaVersion = eventSchemaVersion
	if e.OutputID == "" {
		e.OutputID = r.outputID
	}
	r.graph.Root().Journal(e, r.cfg.maxEvents)
}

// followRecordLocked brings every projection up to date with the record. Callers hold r.mu.
func (r *Run) followRecordLocked() { r.show.Follow() }

type taskState struct {
	id string
	// node is the graph's declaration of this Task; it owns the stable key and scheduling.
	node   *graph.Task
	name   string
	handle *Task
	// refused marks a Task whose declaration was refused as misuse (Compute on a Task with a Verify, Define(nil)). It never
	// runs, and the misuse line already says why, so Finish settles it without a "forgot to resolve" row.
	refused bool
}

type tasksState struct {
	id string
	// out is the Run this container belongs to; reading its live index first follows the record.
	out *Run
	// node is the graph's declaration of this Group/Sequence (§3.1 stable key).
	node   *graph.Container
	name   string
	handle *Group
}

// taskStates are the render states of the Tasks g declared directly, in declaration order.
func (g *tasksState) taskStates() []*taskState {
	nodes := g.node.Tasks()
	states := make([]*taskState, len(nodes))
	for i, node := range nodes {
		states[i] = g.out.taskStates[node.ID]
	}
	return states
}

// collection is the container st was declared under, nil for a root Task.
func (st *taskState) collection() *tasksState {
	if st.node.Parent == nil {
		return nil
	}
	return st.handle.out.containerStates[st.node.Parent.ID]
}

// appendTaskLocked gives st a row in every projection and announces it; synthetic marks the run's own ending Task.
func (r *Run) appendTaskLocked(st *taskState, synthetic bool) {
	in := ""
	if col := st.collection(); col != nil {
		in = col.id
	}
	row := txt.DeclaredTask{ID: st.node.TaskID(), Node: st.node, Name: st.name, Declaration: st.node.Declaration, Rec: st.node.Rec, Synthetic: synthetic}
	if !synthetic {
		row.Timing.Declared = r.cfg.clock.Now()
	}
	r.tasks = append(r.tasks, st)
	if in == "" {
		r.rootTasks = append(r.rootTasks, st)
	}
	r.show.DeclareTask(row, in)
}

// arm marks the live surface ready to paint before any entity is declared (first-paint rule). Idempotent.
func (r *Run) arm() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.show.Arm()
}

type presentHost struct{ o *Run }

func (h presentHost) Ended() bool        { return h.o.closed || h.o.finished }
func (h presentHost) Width() int         { return h.o.cfg.width }
func (h presentHost) Resize(width int)   { h.o.cfg.width = width }
func (h presentHost) Journal(e recEvent) { h.o.appendEventLocked(e) }
func (h presentHost) Tracked(id record.TaskID) record.TaskOperations {
	return h.o.freshness.Tasks().Tracked(id)
}

// snapshotLocked is the run as every projection shows it now. Callers hold r.mu.
func (r *Run) snapshotLocked() snapshot {
	return r.show.Snapshot(snapshot{
		OutputID:  r.outputID,
		Subject:   r.cfg.subject,
		Summary:   r.rec.RunSummary(),
		Lines:     r.rec.Lines(),
		Timestamp: r.cfg.clock.Now(),
		DryRun:    r.cfg.dryRun,
		Warnings:  r.rec.RunWarnings(),
		Facts:     r.rec.RunFacts(),
	})
}

// emitWireEventLocked writes one "evo.event" line; a run that streams no events writes nothing.
func (r *Run) emitWireEventLocked(eventType, entityID string, payload map[string]any) {
	r.show.Events.Emit(eventType, entityID, payload)
}
