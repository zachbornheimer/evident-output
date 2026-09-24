package engine

import (
	"encoding/json"

	"github.com/zachbornheimer/evident-output/internal/wire"
)

// wireProblemPayloadLocked builds the "evo.event" payload for a
// problem.recorded/warning.recorded line (spec §38), carrying the same
// machine truth a FormatJSON/FormatJSON-run consumer sees for this
// Problem, not a narrower subset. It reuses wire.ToProblemDoc — the one
// owner of the core.Problem->wire projection, including Sensitive-Field
// redaction — instead of rebuilding that projection independently.
func wireProblemPayloadLocked(p Problem) map[string]any {
	return problemDocPayload(wire.ToProblemDoc(p))
}

// problemDocPayload builds this event stream's payload from doc's own JSON
// encoding (spec §38: a field the Problem never set is omitted, not
// present-but-empty — the same `omitempty` tags ProblemDoc already carries
// decide that here too). JSONL's own field is "summary", one name
// different from ProblemDoc's "message" (§36's "evo.run" field); every
// other key carries doc's already-redacted, already-projected value
// straight through, unrenamed.
//
// Marshaling doc itself (instead of copying its fields one at a time) is
// deliberate: the next field ToProblemDoc gains is in this payload for
// free, so JSONL cannot silently fall behind evo.run the way ZYS-823 found
// it had — see problemDocPayload_test.go's json-tag coverage check.
func problemDocPayload(doc wire.ProblemDoc) map[string]any {
	raw, err := json.Marshal(doc)
	if err != nil {
		// ProblemDoc's fields are all JSON-marshalable primitives/structs;
		// this is unreachable outside a future field breaking that
		// invariant, at which point a test should catch it before this
		// runs in production.
		return map[string]any{"summary": doc.Message}
	}
	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return map[string]any{"summary": doc.Message}
	}
	payload["summary"] = doc.Message
	delete(payload, "message")
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
