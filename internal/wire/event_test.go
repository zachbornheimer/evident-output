package wire

import (
	"os"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/wireschema"
)

func TestToEventDocument_ValidatesAgainstSchema(t *testing.T) {
	schema, err := os.ReadFile("../../schema/event.v2.json")
	if err != nil {
		t.Fatalf("read schema/event.v2.json: %v", err)
	}
	e := core.Event{
		SchemaVersion: core.EventSchemaVersion,
		Sequence:      17,
		Timestamp:     time.Date(2026, 9, 15, 20, 0, 1, 241000000, time.UTC),
		Type:          EventTaskFinished,
		OutputID:      "out_1",
		EntityID:      "task_1",
		Payload:       map[string]any{"phase": "after_definition"},
	}
	doc, err := EncodeEvent(e)
	if err != nil {
		t.Fatalf("EncodeEvent: %v", err)
	}
	if err := wireschema.Validate(schema, doc); err != nil {
		t.Fatalf("evo.event document does not conform to schema/event.v2.json:\n%v\n\ndocument:\n%s", err, doc)
	}
}

func TestToEventDocument_SeqIsOrderingAuthority(t *testing.T) {
	e := core.Event{Sequence: 42, Type: EventTaskStarted, OutputID: "out_1"}
	doc := ToEventDocument(e)
	if doc.Seq != 42 {
		t.Fatalf("Seq = %d, want 42", doc.Seq)
	}
	if doc.Object != EventObject {
		t.Fatalf("Object = %q, want %q", doc.Object, EventObject)
	}
	if doc.SchemaVersion != EventSchemaVersion {
		t.Fatalf("SchemaVersion = %q, want %q", doc.SchemaVersion, EventSchemaVersion)
	}
}

// TestEventFamilies_AreDistinctFromLegacyEventTypes documents that the §38
// family constants are a separate vocabulary from the runtime's current
// Event.Type strings (output.started, task.blocked, ...) — increment 4 does
// not remap the runtime, only defines where the eventual family names live.
func TestEventFamilies_AreDistinctFromLegacyEventTypes(t *testing.T) {
	families := []string{
		EventRunStarted, EventCollectionDeclared, EventTaskDeclared, EventTaskEligible,
		EventEvidenceEvaluated, EventTaskStarted, EventDefinitionStarted, EventOperationStarted,
		EventOperationSkippedCurrent, EventTrackedResourceObserved, EventBasisFingerprinted,
		EventVerificationObserved, EventFactRecorded, EventWarningRecorded, EventEffectPlanned,
		EventEffectCommitted, EventManifestTaskCommitted, EventOperationFinished,
		EventDefinitionFinished, EventTaskFinished, EventRunFinished,
	}
	seen := make(map[string]bool, len(families))
	for _, f := range families {
		if f == "" {
			t.Fatalf("empty event family constant")
		}
		if seen[f] {
			t.Fatalf("duplicate event family constant %q", f)
		}
		seen[f] = true
	}
	if len(seen) != 21 {
		t.Fatalf("got %d distinct §38 event families, want 21", len(seen))
	}
}
