package engine

import (
	"github.com/zachbornheimer/evident-output/internal/wire"
)

// wireProblemPayloadLocked builds the "evo.event" payload for a
// problem.recorded/warning.recorded line (spec §38), carrying every field
// the Problem carries — the same code/subject/count/unit/location/
// severity/evidence/fields/remedies machine truth a FormatJSON or
// FormatJSON-run consumer sees, not a narrower subset. It reuses
// wire.ToProblemDoc — the one owner of the core.Problem->wire projection,
// including Sensitive-Field redaction — instead of rebuilding that
// projection as a third hand-written map[string]any (the duplication
// ZYS-823's own review found across this function, wire.toProblemDocs and
// render.toJSONProblems). problemDocPayload then copies ProblemDoc's own
// fields into this event stream's payload shape.
func wireProblemPayloadLocked(p Problem) map[string]any {
	return problemDocPayload(wire.ToProblemDoc(p))
}

// problemDocPayload copies doc's non-zero fields into this event stream's
// payload shape (spec §38: a field the Problem never set is omitted, not
// present-but-empty). JSONL's own field is "summary", one name different
// from ProblemDoc's "message" (§36's "evo.run" field) — every other key
// carries doc's already-redacted, already-projected value straight
// through, so this is a direct copy rather than a second independent
// projection of core.Problem.
func problemDocPayload(doc wire.ProblemDoc) map[string]any {
	payload := map[string]any{"summary": doc.Message}
	if doc.Code != "" {
		payload["code"] = doc.Code
	}
	if doc.Subject != "" {
		payload["subject"] = doc.Subject
	}
	if doc.Detail != "" {
		payload["detail"] = doc.Detail
	}
	if doc.EvidenceTail != "" {
		payload["evidence_tail"] = doc.EvidenceTail
	}
	if doc.Count != 0 {
		payload["count"] = doc.Count
	}
	if doc.Unit != "" {
		payload["unit"] = doc.Unit
	}
	if doc.Severity != "" {
		payload["severity"] = doc.Severity
	}
	if doc.Location != nil {
		payload["location"] = doc.Location
	}
	if len(doc.Evidence) > 0 {
		payload["evidence"] = doc.Evidence
	}
	if len(doc.Fields) > 0 {
		payload["fields"] = doc.Fields
	}
	if len(doc.Remedies) > 0 {
		payload["remedies"] = doc.Remedies
	}
	return payload
}

// emitWireEventLocked appends one §38 "evo.event" JSONL line for eventType,
// scoped to entityID (empty for a run-level event) with payload as its
// domain fields. It is a no-op unless this Output is configured for
// FormatJSONL (spec §32.1: the event stream exists only in that Format) —
// every call site below fires unconditionally regardless of Format; this
// guard is the one place that decides whether the line is worth building.
//
// wireSeq is its own counter, independent of the legacy 0.3 journal's
// o.events sequence (appendEventLocked): the two streams describe distinct
// vocabularies (see internal/wire/event.go's §38 family constants vs. the
// legacy Event.Type strings appendEventLocked still carries unchanged), and
// §38 only requires seq strictly monotonic *within this stream*, not shared
// with the legacy one.
//
// A write failure is recorded rather than surfaced here (first one wins):
// this often runs deep inside the runtime's own lock, before any Task can
// meaningfully react. Finish folds wireEventErr into the misuse error it
// already returns, so "a later write fails; the Run then fails" without
// discarding the lines already written (see Finish's use of wireEventErr).
func (o *Output) emitWireEventLocked(eventType, entityID string, payload map[string]any) {
	if o.cfg.wireFormat != FormatJSONL {
		return
	}
	o.wireSeq++
	ev := Event{
		Type:      eventType,
		Sequence:  o.wireSeq,
		Timestamp: o.cfg.clock.Now(),
		OutputID:  o.outputID,
		EntityID:  entityID,
		Payload:   payload,
	}
	if err := writeWireEventLocked(o.cfg.wireStream, ev); err != nil && o.wireEventErr == nil {
		o.wireEventErr = err
	}
}

// emitCollectionDeclaredLocked emits collection.declared (spec §38) for a
// newly declared Group/Sequence container, at top level or nested — the one
// call site Group, Sequence, and declareChildContainerLocked all share.
func (o *Output) emitCollectionDeclaredLocked(st *tasksState, parentID string) {
	kind := wire.CollectionKindGroup
	if st.sequential {
		kind = wire.CollectionKindSequence
	}
	o.emitWireEventLocked(wire.EventCollectionDeclared, st.id, map[string]any{
		"name":      st.name,
		"kind":      kind,
		"parent_id": parentID,
	})
}

// wireRunOutcome maps a finished Conclusion's state to the §38/§35 outcome
// vocabulary (spec §35: "outcome: ok | blocked | failed | cancelled") — the
// same mapping wire.ToRunDocument uses for the final "evo.run" document,
// duplicated locally rather than imported since internal/wire's version is
// unexported (ToRunDocument's own outcomeFor).
func wireRunOutcome(state ConclusionState) string {
	switch state {
	case StateFailed:
		return wire.OutcomeFailed
	case StateBlocked:
		return wire.OutcomeBlocked
	case StateCancelled:
		return wire.OutcomeCancelled
	default:
		return wire.OutcomeOK
	}
}
