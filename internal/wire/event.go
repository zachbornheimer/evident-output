package wire

import (
	"encoding/json"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// EventSchemaVersion is the "evo.event" JSONL line's schema_version
// (spec §38) — a distinct series from internal/render's legacy "0.3"
// EventJSON, never touched by this package.
const EventSchemaVersion = "1.0"

// EventObject is the "evo.event" envelope's object discriminator (spec §38).
const EventObject = "evo.event"

// Required event families (spec §38). Increment 1's runtime event model does
// not yet emit every family below (operation/basis/manifest families arrive
// with increment 2's File/manifest work) — the constants are defined now so
// call sites converge on one name instead of ad hoc strings, per the work
// order's "keep the type constants defined now."
const (
	EventRunStarted              = "run.started"
	EventCollectionDeclared      = "collection.declared"
	EventTaskDeclared            = "task.declared"
	EventTaskEligible            = "task.eligible"
	EventEvidenceEvaluated       = "evidence.evaluated"
	EventTaskStarted             = "task.started"
	EventDefinitionStarted       = "definition.started"
	EventOperationStarted        = "operation.started"
	EventOperationSkippedCurrent = "operation.skipped_current"
	EventTrackedResourceObserved = "tracked_resource.observed"
	EventBasisFingerprinted      = "basis.fingerprinted"
	EventVerificationObserved    = "verification.observed"
	EventFactRecorded            = "fact.recorded"
	EventWarningRecorded         = "warning.recorded"
	EventEffectPlanned           = "effect.planned"
	EventEffectCommitted         = "effect.committed"
	EventManifestTaskCommitted   = "manifest.task_committed"
	EventOperationFinished       = "operation.finished"
	EventDefinitionFinished      = "definition.finished"
	EventTaskFinished            = "task.finished"
	EventRunFinished             = "run.finished"
)

// EventDocument is one "evo.event" JSONL line (spec §38). seq is the
// ordering authority; timestamp (at) is informational only.
type EventDocument struct {
	Object        string         `json:"object"`
	SchemaVersion string         `json:"schema_version"`
	RunID         string         `json:"run_id,omitempty"`
	Seq           uint64         `json:"seq"`
	At            time.Time      `json:"at"`
	Type          string         `json:"type"`
	EntityID      string         `json:"entity_id,omitempty"`
	Payload       map[string]any `json:"payload,omitempty"`
}

// ToEventDocument builds the "evo.event" wire line from one runtime journal
// Event. The runtime Event.Type vocabulary (output.started, task.blocked,
// ...) predates the §38 family names above; increment 4 does not remap it —
// that migration is a runtime-model change (increment 2/3 territory), not a
// wire-encoder one. The wire shape (object/schema_version/run_id/seq/at/
// type/entity_id/payload) is final now regardless of which Type strings the
// runtime currently emits.
func ToEventDocument(e core.Event) EventDocument {
	return EventDocument{
		Object:        EventObject,
		SchemaVersion: EventSchemaVersion,
		RunID:         e.OutputID,
		Seq:           e.Sequence,
		At:            e.Timestamp,
		Type:          e.Type,
		EntityID:      e.EntityID,
		Payload:       e.Payload,
	}
}

// EncodeEvent encodes one journal event as a single "evo.event" JSON object
// (no trailing newline — JSONL callers append it themselves, one per line).
func EncodeEvent(e core.Event) ([]byte, error) {
	return json.Marshal(ToEventDocument(e))
}
