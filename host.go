package evo

import (
	"github.com/zachbornheimer/evident-output/v2/internal/change"
	"github.com/zachbornheimer/evident-output/v2/internal/freshness"
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

func (o *Run) newChangeHost() change.Host {
	return change.NewHost(change.HostParts{Engine: runHost{out: o}, Record: o.rec, Graph: o.graph, Freshness: o.freshness})
}

func (e runHost) Lock()   { e.out.mu.Lock() }
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

// taskNameLocked is the name of the Task id, empty for an id the Run holds no Task for. Callers hold o.mu.
func (o *Run) taskNameLocked(id record.TaskID) string {
	if st := o.taskStates[string(id)]; st != nil {
		return st.name
	}
	return ""
}

// stateOf is the render state of the graph's Task t. Callers hold o.mu.
func (o *Run) stateOf(t *graph.Task) *taskState { return o.taskStates[t.ID] }

func (runHost) Ending(err error) (record.State, record.Problem) { return endingOf(err) }

func (e runHost) Changed(*graph.Task) { e.out.followRecordLocked() }

// EndingName names the Task carrying the run's own ending: the subject, else the executable.
func (e runHost) EndingName() string { return e.out.endingName() }

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

// Settled writes the settled Task where readers find it. Only a Done Task commits its manifest record
// (spec §8.2/§11.3).
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
// work. Callers hold o.mu.
func (o *Run) fileSettledLocked(t *graph.Task) *taskState {
	o.followRecordLocked()
	st := o.stateOf(t)
	if st == nil {
		return nil
	}
	o.show.Stream.Commit(record.TaskID(st.id))
	if st.collection() != nil {
		o.show.Live.Signal(true)
	} else {
		o.show.Stream.CommitEffects(st.id)
	}
	return st
}

// commitManifestLocked persists st's operations once it settled Done and narrates the commit. Callers hold o.mu.
func (o *Run) commitManifestLocked(st *taskState) {
	commit := o.freshness.Commit(o.graph.Context(), st.node, freshness.ModeFor(o.cfg.dryRun))
	if commit.Err != nil {
		o.warnManifestUnsavedLocked(commit.Err)
		return
	}
	if commit.Written {
		o.emitWireEventLocked(wire.EventManifestTaskCommitted, st.id,
			wire.ManifestTaskCommittedPayload{Operations: commit.Operations}.EventPayload())
	}
}

// RunEnded registers the synthetic Task carrying the run's own ending, already settled.
func (e runHost) RunEnded(t *graph.Task) {
	o := e.out
	o.appendTaskLocked(&taskState{id: t.ID, node: t, name: t.Name}, true)
}

// narrate streams one event about a Task under the Run's lock; a run that streams no events skips the lock.
func (e runHost) narrate(event, entityID string, payload func() map[string]any) {
	o := e.out
	if !o.show.Events.On() {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.emitWireEventLocked(event, entityID, payload())
}

func (e runHost) DefinitionStarted(t *graph.Task) {
	e.narrate(wire.EventDefinitionStarted, t.ID, wire.EmptyPayload{}.EventPayload)
}

func (e runHost) DefinitionFinished(t *graph.Task, failed bool) {
	e.narrate(wire.EventDefinitionFinished, t.ID, func() map[string]any {
		return wire.DefinitionFinishedPayload{Failed: failed}.EventPayload()
	})
}

func (e runHost) EvidenceEvaluated(t *graph.Task, phase graph.EvidencePhase, evidence record.EvidencePhase) {
	e.narrate(wire.EventEvidenceEvaluated, t.ID, func() map[string]any {
		name := wire.EvidencePhaseBeforeDefinition
		if phase == graph.EvidenceAfter {
			name = wire.EvidencePhaseAfterDefinition
		}
		return wire.EvidenceEvaluatedPayload{
			Phase: name, Evaluated: evidence.Evaluated, Satisfied: evidence.Satisfied, Source: evidence.Source,
		}.EventPayload()
	})
}

func (e runHost) BasisJudged(id record.TaskID, judgement freshness.BasisJudgement) {
	e.narrate(wire.EventBasisFingerprinted, string(id), func() map[string]any {
		current := judgement.Current
		return wire.BasisFingerprintedPayload{Count: len(judgement.Observed), Current: &current}.EventPayload()
	})
}

// VerifierObserved narrates one Verify check's outcome (schema/event.v2.json), named for the Task it verifies.
func (e runHost) VerifierObserved(id record.TaskID, outcome freshness.VerifierOutcome) {
	e.narrate(wire.EventVerificationObserved, string(id), func() map[string]any {
		status := wire.VerificationUnsatisfied
		switch outcome {
		case freshness.VerifierErrored:
			status = wire.VerificationError
		case freshness.VerifierSatisfied:
			status = wire.VerificationSatisfied
		}
		return wire.VerificationDoc{Name: e.out.taskNameLocked(id), Status: status}.EventPayload()
	})
}

// Observe narrates one step of a tracked operation as an "evo.event" line; the wire vocabulary stays at this edge.
func (e runHost) Observe(ob change.Observation) {
	e.out.emitWireEventLocked(observationEvent(ob.Kind), string(ob.Task), observationPayload(ob))
}

func observationEvent(kind change.ObservationKind) string {
	switch kind {
	case change.OperationStarted:
		return wire.EventOperationStarted
	case change.OperationSkippedCurrent:
		return wire.EventOperationSkippedCurrent
	case change.OperationFinished:
		return wire.EventOperationFinished
	case change.ResourceObserved:
		return wire.EventTrackedResourceObserved
	default:
		return wire.EventBasisFingerprinted
	}
}

// observationPayload is the event payload for ob: the subject's kind and name under its kind's
// historical key, then the step's fields. A File's basis.fingerprinted line never names its kind.
func observationPayload(ob change.Observation) map[string]any {
	subject := wire.ObservedSubject{Kind: wire.PayloadKindFile, Path: ob.Subject.Name}
	switch ob.Subject.Kind {
	case change.SubjectExec:
		subject = wire.ObservedSubject{Kind: wire.PayloadKindExec, Executable: ob.Subject.Name}
	case change.SubjectExecOutput:
		subject.Kind = wire.PayloadKindExecOutput
	}
	switch ob.Kind {
	case change.OperationStarted, change.OperationSkippedCurrent:
		return wire.OperationObservedPayload{ObservedSubject: subject, Reason: ob.Reason}.EventPayload()
	case change.OperationFinished:
		return wire.OperationFinishedPayload{ObservedSubject: subject, Changed: ob.Changed}.EventPayload()
	case change.ResourceObserved:
		return wire.TrackedResourceObservedPayload{ObservedSubject: subject, Exists: ob.Exists}.EventPayload()
	}
	basis := wire.BasisFingerprintedPayload{Kind: subject.Kind, Path: subject.Path, Executable: subject.Executable, Count: ob.Count}
	if ob.Subject.Kind == change.SubjectFile {
		basis.Kind = ""
	}
	return basis.EventPayload()
}
