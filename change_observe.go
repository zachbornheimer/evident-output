package evo

import (
	"github.com/zachbornheimer/evident-output/internal/change"
	"github.com/zachbornheimer/evident-output/internal/project/wire"
)

// Observe narrates one step of a tracked operation as the §38 "evo.event"
// line it has always been: change speaks of subjects and steps, the wire
// vocabulary stays here.
func (h changeHost) Observe(ob change.Observation) {
	h.out.mu.Lock()
	defer h.out.mu.Unlock()
	h.out.emitWireEventLocked(observationEvent(ob.Kind), string(ob.Task), observationPayload(ob))
}

// observationEvent is the §38 event type for a step of a tracked operation.
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

// observationPayload is the §38 payload for ob: the subject's kind and name
// under the key its kind has always used, then the fields its step carries.
// A File's basis.fingerprinted line has never named its kind, and still does
// not.
func observationPayload(ob change.Observation) map[string]any {
	kind, nameKey := wire.PayloadKindFile, "path"
	switch ob.Subject.Kind {
	case change.SubjectExec:
		kind, nameKey = wire.PayloadKindExec, "executable"
	case change.SubjectExecOutput:
		kind = wire.PayloadKindExecOutput
	}
	payload := map[string]any{nameKey: ob.Subject.Name}
	if ob.Kind != change.BasisFingerprinted || ob.Subject.Kind != change.SubjectFile {
		payload["kind"] = kind
	}
	switch ob.Kind {
	case change.OperationStarted, change.OperationSkippedCurrent:
		if ob.Reason != "" {
			payload["reason"] = ob.Reason
		}
	case change.OperationFinished:
		payload["changed"] = ob.Changed
	case change.ResourceObserved:
		payload["exists"] = ob.Exists
	case change.BasisFingerprinted:
		payload["count"] = ob.Count
	}
	return payload
}
