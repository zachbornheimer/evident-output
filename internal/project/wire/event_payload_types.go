package wire

// The payload types below describe every evo.event payload the runtime writes, one per §38 event family.
// schema.go binds each to its event type, so schema/event.v2.json is generated from these types and a
// payload key the schema does not declare fails the strict tests. Each EventPayload builds the payload
// map directly, omitting an unset optional field exactly as the type's own omitempty tag omits it.

// EmptyPayload is the payload of an event that carries none: task.eligible, task.started, definition.started.
type EmptyPayload struct{}

// EventPayload is p as the event's payload map: none.
func (EmptyPayload) EventPayload() map[string]any { return nil }

// RunStartedPayload is a run.started payload: the run's subject, absent when it has none.
type RunStartedPayload struct {
	Name string `json:"name,omitempty"`
}

// EventPayload is p as the event's payload map.
func (p RunStartedPayload) EventPayload() map[string]any {
	payload := map[string]any{}
	putNonZero(payload, "name", p.Name)
	return payload
}

// TaskDeclaredPayload is a task.declared payload.
type TaskDeclaredPayload struct {
	Name string `json:"name"`
}

// EventPayload is p as the event's payload map.
func (p TaskDeclaredPayload) EventPayload() map[string]any {
	return map[string]any{"name": p.Name}
}

// CollectionDeclaredPayload is a collection.declared payload. ParentID is empty for a root collection.
type CollectionDeclaredPayload struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	ParentID string `json:"parent_id"`
}

// EventPayload is p as the event's payload map.
func (p CollectionDeclaredPayload) EventPayload() map[string]any {
	return map[string]any{"name": p.Name, "kind": p.Kind, "parent_id": p.ParentID}
}

// Evidence phase names of an evidence.evaluated payload.
const (
	EvidencePhaseBeforeDefinition = "before_definition"
	EvidencePhaseAfterDefinition  = "after_definition"
)

// EvidenceEvaluatedPayload is an evidence.evaluated payload: one Verify observation.
type EvidenceEvaluatedPayload struct {
	Phase     string `json:"phase"`
	Evaluated bool   `json:"evaluated"`
	Satisfied bool   `json:"satisfied"`
	Source    string `json:"source"`
}

// EventPayload is p as the event's payload map.
func (p EvidenceEvaluatedPayload) EventPayload() map[string]any {
	return map[string]any{"phase": p.Phase, "evaluated": p.Evaluated, "satisfied": p.Satisfied, "source": p.Source}
}

// ObservedSubject is what an operation, tracked_resource or basis event is about: a File or exec-output
// by Path, an Exec by Executable, named by Kind.
type ObservedSubject struct {
	Kind       string `json:"kind"`
	Path       string `json:"path,omitempty"`
	Executable string `json:"executable,omitempty"`
}

// fields is the subject's payload keys.
func (s ObservedSubject) fields() map[string]any {
	payload := map[string]any{"kind": s.Kind}
	putNonZero(payload, "path", s.Path)
	putNonZero(payload, "executable", s.Executable)
	return payload
}

// OperationObservedPayload is an operation.started or operation.skipped_current payload; Reason is why the
// operation ran or was current, absent when none was judged.
type OperationObservedPayload struct {
	ObservedSubject
	Reason string `json:"reason,omitempty"`
}

// EventPayload is p as the event's payload map.
func (p OperationObservedPayload) EventPayload() map[string]any {
	payload := p.fields()
	putNonZero(payload, "reason", p.Reason)
	return payload
}

// OperationFinishedPayload is an operation.finished payload: whether the operation changed anything.
type OperationFinishedPayload struct {
	ObservedSubject
	Changed bool `json:"changed"`
}

// EventPayload is p as the event's payload map.
func (p OperationFinishedPayload) EventPayload() map[string]any {
	payload := p.fields()
	payload["changed"] = p.Changed
	return payload
}

// TrackedResourceObservedPayload is a tracked_resource.observed payload: whether the resource exists.
type TrackedResourceObservedPayload struct {
	ObservedSubject
	Exists bool `json:"exists"`
}

// EventPayload is p as the event's payload map.
func (p TrackedResourceObservedPayload) EventPayload() map[string]any {
	payload := p.fields()
	payload["exists"] = p.Exists
	return payload
}

// BasisFingerprintedPayload is a basis.fingerprinted payload: Count inputs were fingerprinted. A File's
// line names no Kind. The line that judges a Task's whole basis names no subject and carries Current.
type BasisFingerprintedPayload struct {
	Kind       string `json:"kind,omitempty"`
	Path       string `json:"path,omitempty"`
	Executable string `json:"executable,omitempty"`
	Count      int    `json:"count"`
	Current    *bool  `json:"current,omitempty"`
}

// EventPayload is p as the event's payload map.
func (p BasisFingerprintedPayload) EventPayload() map[string]any {
	payload := map[string]any{"count": p.Count}
	putNonZero(payload, "kind", p.Kind)
	putNonZero(payload, "path", p.Path)
	putNonZero(payload, "executable", p.Executable)
	if p.Current != nil {
		payload["current"] = *p.Current
	}
	return payload
}

// EffectPayload is an effect.planned or effect.committed payload; Quantity is absent for an uncounted effect.
type EffectPayload struct {
	Verb     string `json:"verb"`
	Object   string `json:"object"`
	Quantity *int64 `json:"quantity,omitempty"`
}

// EventPayload is p as the event's payload map.
func (p EffectPayload) EventPayload() map[string]any {
	payload := map[string]any{"verb": p.Verb, "object": p.Object}
	if p.Quantity != nil {
		payload["quantity"] = *p.Quantity
	}
	return payload
}

// ManifestTaskCommittedPayload is a manifest.task_committed payload: how many operations the commit recorded.
type ManifestTaskCommittedPayload struct {
	Operations int `json:"operations"`
}

// EventPayload is p as the event's payload map.
func (p ManifestTaskCommittedPayload) EventPayload() map[string]any {
	return map[string]any{"operations": p.Operations}
}

// TaskFinishedPayload is a task.finished payload: the Outcome the Task settled with, absent for a Task that
// never started exactly as in evo.run, and its Summary when it has one.
type TaskFinishedPayload struct {
	Outcome string `json:"outcome,omitempty"`
	Summary string `json:"summary,omitempty"`
}

// EventPayload is p as the event's payload map.
func (p TaskFinishedPayload) EventPayload() map[string]any {
	payload := map[string]any{}
	putNonZero(payload, "outcome", p.Outcome)
	putNonZero(payload, "summary", p.Summary)
	return payload
}

// RunFinishedPayload is a run.finished payload; Explanation and Summary are absent when the run gave none.
type RunFinishedPayload struct {
	Outcome     string `json:"outcome"`
	ExitCode    int    `json:"exit_code"`
	Explanation string `json:"explanation,omitempty"`
	Summary     string `json:"summary,omitempty"`
}

// EventPayload is p as the event's payload map.
func (p RunFinishedPayload) EventPayload() map[string]any {
	payload := map[string]any{"outcome": p.Outcome, "exit_code": p.ExitCode}
	putNonZero(payload, "explanation", p.Explanation)
	putNonZero(payload, "summary", p.Summary)
	return payload
}

// ProblemRecordedPayload is a problem.recorded or warning.recorded payload: a ProblemDoc with its message
// under "summary" (§38). ProblemDoc.EventPayload builds it.
type ProblemRecordedPayload struct {
	Summary      string       `json:"summary"`
	Code         string       `json:"code,omitempty"`
	Subject      string       `json:"subject,omitempty"`
	Detail       string       `json:"detail,omitempty"`
	EvidenceTail string       `json:"evidence_tail,omitempty"`
	Count        int64        `json:"count,omitempty"`
	Unit         string       `json:"unit,omitempty"`
	Location     *LocationDoc `json:"location,omitempty"`
	Remedies     []ActionDoc  `json:"remedies,omitempty"`
}

// EventPayload is p as the event's payload map.
func (p ProblemRecordedPayload) EventPayload() map[string]any {
	payload := map[string]any{problemEventSummaryKey: p.Summary}
	putNonZero(payload, "code", p.Code)
	putNonZero(payload, "subject", p.Subject)
	putNonZero(payload, "detail", p.Detail)
	putNonZero(payload, "evidence_tail", p.EvidenceTail)
	putNonZero(payload, "count", p.Count)
	putNonZero(payload, "unit", p.Unit)
	putNonZero(payload, "location", p.Location)
	if len(p.Remedies) > 0 {
		payload["remedies"] = p.Remedies
	}
	return payload
}

// ResultWrittenPayload is a result.written payload: the bytes written as Text, or as Base64 when they are
// not valid UTF-8. An empty write carries an empty Text.
type ResultWrittenPayload struct {
	Text   string `json:"text,omitempty"`
	Base64 string `json:"base64,omitempty"`
}

// NewResultWrittenPayload is b as a result.written payload.
func NewResultWrittenPayload(b []byte) ResultWrittenPayload {
	text, encoded := TextOrBase64(string(b))
	return ResultWrittenPayload{Text: text, Base64: encoded}
}

// EventPayload is p as the event's payload map.
func (p ResultWrittenPayload) EventPayload() map[string]any {
	if p.Base64 != "" {
		return map[string]any{PayloadKeyBase64: p.Base64}
	}
	return map[string]any{PayloadKeyText: p.Text}
}
