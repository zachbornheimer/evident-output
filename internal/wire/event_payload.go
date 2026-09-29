package wire

// Each EventPayload below is its doc's §38 "evo.event" payload: the same
// fields evo.run carries for that doc, built directly (no JSON round-trip
// that could fail), with an unset optional field omitted exactly as the
// doc's own omitempty tag omits it. event_payload_test.go proves each one
// encodes to the doc's own JSON, so a field added to a doc without its
// payload key fails there.

// problemEventSummaryKey is JSONL's name for ProblemDoc.Message (§38);
// evo.run calls the same field "message" (§36).
const problemEventSummaryKey = "summary"

// EventPayload is d as a problem.recorded/warning.recorded payload.
func (d ProblemDoc) EventPayload() map[string]any {
	p := map[string]any{problemEventSummaryKey: d.Message}
	putNonZero(p, "code", d.Code)
	putNonZero(p, "subject", d.Subject)
	putNonZero(p, "detail", d.Detail)
	putNonZero(p, "evidence_tail", d.EvidenceTail)
	putNonZero(p, "count", d.Count)
	putNonZero(p, "unit", d.Unit)
	putNonZero(p, "location", d.Location)
	if len(d.Remedies) > 0 {
		p["remedies"] = d.Remedies
	}
	return p
}

// EventPayload is d as a verification.observed payload.
func (d VerificationDoc) EventPayload() map[string]any {
	p := map[string]any{"name": d.Name, "status": d.Status}
	if len(d.Facts) > 0 {
		p["facts"] = d.Facts
	}
	return p
}

// EventPayload is d as a fact.recorded payload.
func (d FactDoc) EventPayload() map[string]any {
	return map[string]any{"name": d.Name, "value": d.Value}
}

// putNonZero sets p[key] only when v is set, mirroring omitempty.
func putNonZero[T comparable](p map[string]any, key string, v T) {
	var zero T
	if v != zero {
		p[key] = v
	}
}
