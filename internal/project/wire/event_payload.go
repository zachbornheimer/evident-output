package wire

import (
	"encoding/base64"
	"unicode/utf8"
)

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
	return ProblemRecordedPayload{
		Summary: d.Message, Code: d.Code, Subject: d.Subject, Detail: d.Detail,
		EvidenceTail: d.EvidenceTail, Count: d.Count, Unit: d.Unit,
		Location: d.Location, Remedies: d.Remedies,
	}.EventPayload()
}

// EventPayload is d as a verification.observed payload.
func (d VerificationDoc) EventPayload() map[string]any {
	p := map[string]any{"name": d.Name, "status": d.Status}
	if len(d.Facts) > 0 {
		p["facts"] = d.Facts
	}
	return p
}

// DefinitionFinishedPayload is a definition.finished payload: Failed is whether the callback ended in
// failure. A callback that ended in an Exclude or Refuse decision did not fail.
type DefinitionFinishedPayload struct {
	Failed bool `json:"failed"`
}

// EventPayload is p as the event's payload map.
func (p DefinitionFinishedPayload) EventPayload() map[string]any {
	return map[string]any{"failed": p.Failed}
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

// Payload keys of a result.written event: the bytes written, as text when they are valid UTF-8 and as
// base64 otherwise.
const (
	PayloadKeyText   = "text"
	PayloadKeyBase64 = "base64"
)

// TextOrBase64 splits payload into the text that carries it exactly, or its base64 when the bytes are not
// valid UTF-8; exactly one result is non-empty, unless payload is empty.
func TextOrBase64(payload string) (text, encoded string) {
	if utf8.ValidString(payload) {
		return payload, ""
	}
	return "", base64.StdEncoding.EncodeToString([]byte(payload))
}
