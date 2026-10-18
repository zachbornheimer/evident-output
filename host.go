package evo

import (
	"github.com/zachbornheimer/evident-output/v2/internal/change"
	"github.com/zachbornheimer/evident-output/v2/internal/fs"
	"github.com/zachbornheimer/evident-output/v2/internal/graph"
	txt "github.com/zachbornheimer/evident-output/v2/internal/project"
	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

// runHost is the Run as graph, freshness and change borrow it: it implements graph.Host,
// freshness.Narration and change.Engine.
type runHost struct{ out *Run }

const resourceWaitingPrefix = "waiting for "

func (r *Run) newChangeHost() change.Host {
	return change.NewHost(change.HostParts{Engine: runHost{out: r}, Record: r.rec, Graph: r.graph, Freshness: r.freshness})
}

func (e runHost) Lock() { e.out.mu.Lock() }

func (e runHost) Unlock() { e.out.mu.Unlock() }

// DryRun reports whether the Run plans instead of making changes; fixed at construction, so lock-free.
func (e runHost) DryRun() bool { return e.out.cfg.dryRun }

func (e runHost) FS() fs.FS {
	e.out.mu.Lock()
	defer e.out.mu.Unlock()
	return e.out.cfg.fileFS
}

// LedgerTense is the tense id's rows take, and the shared guard behind every change a Task records.
func (e runHost) LedgerTense(id record.TaskID) (record.LedgerTense, error) {
	o := e.out
	st := o.taskStates[string(id)]
	if st == nil {
		return 0, errClosed
	}
	if err := e.CheckOpen(); err != nil {
		return 0, err
	}
	if record.IsTerminalTask(st.node.Rec.State()) {
		o.recordMisuseFor(st.name, errAlreadyResolved)
		return 0, errAlreadyResolved
	}
	return record.TenseFor(o.cfg.dryRun), nil
}

func (e runHost) CheckOpen() error {
	err := e.out.ensureOpen()
	if err != nil {
		e.out.recordMisuse(err)
	}
	return err
}

func (e runHost) RecordMisuse(err error) { e.out.recordMisuse(err) }

// NoteManifestWarning surfaces a safe cache-miss warning (spec §11.3) as a run-scoped Fact, never an error.
func (e runHost) NoteManifestWarning(warning error) { e.out.Fact("manifest", warning.Error()) }

// ShowWaiting makes id's row read "waiting for <label>" and returns the restore function.
func (e runHost) ShowWaiting(id record.TaskID, label string) (restore func()) {
	o, taskID := e.out, string(id)
	text := txt.Text(resourceWaitingPrefix + label)
	o.mu.Lock()
	defer o.mu.Unlock()
	st := o.taskStates[taskID]
	if st == nil || record.IsTerminalTask(st.node.Rec.State()) {
		return nil
	}
	prior := st.node.Rec.Phase()
	o.setLiveOnlyPhaseLocked(st, text)
	return func() {
		o.mu.Lock()
		defer o.mu.Unlock()
		st := o.taskStates[taskID]
		if st == nil || record.IsTerminalTask(st.node.Rec.State()) || st.node.Rec.Phase() != text {
			return
		}
		o.setLiveOnlyPhaseLocked(st, prior)
	}
}

func (e runHost) SectionOpened(_ record.TaskID, section string, tense record.LedgerTense) {
	e.out.bumpLocked()
	e.out.appendEventLocked(recEvent{Type: tense.DeclaredEvent(), EntityID: section})
}

// EntryRecorded journals a recorded ledger row and narrates it as the effect line.
func (e runHost) EntryRecorded(task record.TaskID, row change.LedgerRow) {
	o := e.out
	if row.Added {
		o.bumpLocked()
		o.appendEventLocked(recEvent{Type: row.Tense.RecordedEvent(), EntityID: row.Section})
	}
	event := wire.EventEffectPlanned
	if row.Tense == record.TenseChanged {
		event = wire.EventEffectCommitted
	}
	o.emitWireEventLocked(event, string(task), effectPayload(row).EventPayload())
}

// effectPayload is row as the effect line: the quantity the ledger recorded, so the JSONL stream agrees
// with the human rows and the final document.
func effectPayload(row change.LedgerRow) wire.EffectPayload {
	payload := wire.EffectPayload{Verb: row.Verb, Object: row.Entry.Object()}
	if quantity, counted := row.Entry.Quantity(); counted {
		recorded := int64(quantity)
		payload.Quantity = &recorded
	}
	return payload
}

// VerificationRecorded files the Task's row again and narrates each detail as evo.run projects it.
func (e runHost) VerificationRecorded(task record.TaskID, details []record.VerificationDetail) {
	o := e.out
	o.show.Board.Changed(task)
	for _, d := range details {
		o.emitWireEventLocked(wire.EventVerificationObserved, string(task), wire.ToVerificationDoc(d).EventPayload())
	}
}

// taskNameLocked is the name of the Task id, empty for an id the Run holds no Task for. Callers hold r.mu.
func (r *Run) taskNameLocked(id record.TaskID) string {
	if st := r.taskStates[string(id)]; st != nil {
		return st.name
	}
	return ""
}

// stateOf is the render state of the graph's Task t. Callers hold r.mu.
func (r *Run) stateOf(t *graph.Task) *taskState { return r.taskStates[t.ID] }

func (runHost) Ending(err error) (record.State, record.Problem) { return endingOf(err) }

func (e runHost) Changed(*graph.Task) { e.out.followRecordLocked() }

// EndingName names the Task carrying the run's own ending: the subject, else the executable.
func (e runHost) EndingName() string { return e.out.endingName() }
