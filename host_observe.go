package evo

import (
	"github.com/zachbornheimer/evident-output/v2/internal/change"
	"github.com/zachbornheimer/evident-output/v2/internal/freshness"
	"github.com/zachbornheimer/evident-output/v2/internal/graph"
	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

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
