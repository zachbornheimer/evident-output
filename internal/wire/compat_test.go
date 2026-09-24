package wire

import (
	"encoding/json"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/wireschema"
)

// schemaDir is the read-only fs.FS this file's compat tests read
// schema/run.v2.json through, so the read is an explicit injectable
// boundary rather than a bare os.ReadFile call.
var schemaDir = os.DirFS("../../schema")

// This file is ZYS-823 gap 4: an older-shape "evo.run" document (written
// before a field existed) must still decode into today's types with no
// loss on the fields it does carry, and a field this package added since
// must be additive — an old consumer type that has never heard of it still
// decodes today's document with no error and no data loss on the fields it
// knows about.
//
// "resource" maps to TrackedResourceDoc and "Patch" maps to VerificationDoc
// (spec §27: Patch reduces to File, and File's own machine truth — per-
// attribute contents/permissions outcome, with the Facts that explain a
// failed one — is VerificationDoc, not a second patch-specific wire type;
// there is no separate Patch JSON type in this package). "Problem" maps to
// ProblemDoc. fixtureResourcePath is an obviously-fake path (never touches
// disk — these tests only exercise json.Marshal/Unmarshal on in-memory
// structs).
const fixtureResourcePath = "fixture://managed/x"

func TestSchemaCompat_ProblemDocOlderShapeDecodesWithNoLoss(t *testing.T) {
	older := []byte(`{"code":"A1","message":"finding","subject":"file.go","detail":"why","count":2,"unit":"line"}`)
	var got ProblemDoc
	if err := json.Unmarshal(older, &got); err != nil {
		t.Fatalf("decode older ProblemDoc shape: %v", err)
	}
	want := ProblemDoc{Code: "A1", Message: "finding", Subject: "file.go", Detail: "why", Count: 2, Unit: "line"}
	if got != want {
		t.Fatalf("older-shape decode = %+v, want %+v (no loss on known fields)", got, want)
	}
	if got.EvidenceTail != "" {
		t.Fatalf("EvidenceTail = %q, want zero value for a fixture that never set it", got.EvidenceTail)
	}
}

func TestSchemaCompat_ProblemDocEvidenceTailIsAdditive(t *testing.T) {
	current := ProblemDoc{Code: "A1", Message: "finding", Detail: "why", EvidenceTail: "tail text"}
	encoded, err := json.Marshal(current)
	if err != nil {
		t.Fatalf("encode current ProblemDoc: %v", err)
	}
	type oldConsumerProblemDoc struct {
		Code    string `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
		Detail  string `json:"detail,omitempty"`
	}
	var old oldConsumerProblemDoc
	if err := json.Unmarshal(encoded, &old); err != nil {
		t.Fatalf("old consumer must decode today's document ignoring evidence_tail: %v", err)
	}
	if old.Code != current.Code || old.Message != current.Message || old.Detail != current.Detail {
		t.Fatalf("old consumer decode = %+v, want fields it knows about preserved", old)
	}
}

func TestSchemaCompat_TrackedResourceDocOlderShapeDecodesWithNoLoss(t *testing.T) {
	older := []byte(`{"kind":"file","path":"` + fixtureResourcePath + `","fingerprint":"sha256:abc"}`)
	var got TrackedResourceDoc
	if err := json.Unmarshal(older, &got); err != nil {
		t.Fatalf("decode older TrackedResourceDoc shape: %v", err)
	}
	want := TrackedResourceDoc{Kind: "file", Path: fixtureResourcePath, Fingerprint: "sha256:abc"}
	if got != want {
		t.Fatalf("older-shape decode = %+v, want %+v", got, want)
	}
}

func TestSchemaCompat_TrackedResourceDocModeIsAdditive(t *testing.T) {
	current := TrackedResourceDoc{Kind: "file", Path: fixtureResourcePath, Mode: "0644"}
	encoded, err := json.Marshal(current)
	if err != nil {
		t.Fatalf("encode current TrackedResourceDoc: %v", err)
	}
	type oldConsumerResourceDoc struct {
		Kind string `json:"kind"`
		Path string `json:"path"`
	}
	var old oldConsumerResourceDoc
	if err := json.Unmarshal(encoded, &old); err != nil {
		t.Fatalf("old consumer must decode today's document ignoring mode: %v", err)
	}
	if old.Kind != current.Kind || old.Path != current.Path {
		t.Fatalf("old consumer decode = %+v, want fields it knows about preserved", old)
	}
}

func TestSchemaCompat_VerificationDocOlderShapeDecodesWithNoLoss(t *testing.T) {
	older := []byte(`{"name":"contents","status":"satisfied"}`)
	var got VerificationDoc
	if err := json.Unmarshal(older, &got); err != nil {
		t.Fatalf("decode older VerificationDoc shape: %v", err)
	}
	want := VerificationDoc{Name: "contents", Status: "satisfied"}
	if got.Name != want.Name || got.Status != want.Status || len(got.Facts) != 0 {
		t.Fatalf("older-shape decode = %+v, want %+v (no loss, no invented Facts)", got, want)
	}
}

func TestSchemaCompat_VerificationDocFactsIsAdditive(t *testing.T) {
	current := VerificationDoc{
		Name: "permissions", Status: "error",
		Facts: []FactDoc{{Name: "error", Value: "operation not permitted"}},
	}
	encoded, err := json.Marshal(current)
	if err != nil {
		t.Fatalf("encode current VerificationDoc: %v", err)
	}
	type oldConsumerVerificationDoc struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	var old oldConsumerVerificationDoc
	if err := json.Unmarshal(encoded, &old); err != nil {
		t.Fatalf("old consumer must decode today's document ignoring facts: %v", err)
	}
	if old.Name != current.Name || old.Status != current.Status {
		t.Fatalf("old consumer decode = %+v, want fields it knows about preserved", old)
	}
}

func TestSchemaCompat_EffectDocOlderShapeDecodesWithNoLoss(t *testing.T) {
	older := []byte(`{"subject":"build","status":"changed","verb":"created","object":"file"}`)
	var got EffectDoc
	if err := json.Unmarshal(older, &got); err != nil {
		t.Fatalf("decode older EffectDoc shape: %v", err)
	}
	want := EffectDoc{Subject: "build", Status: "changed", Verb: "created", Object: "file"}
	if got != want {
		t.Fatalf("older-shape decode = %+v, want %+v", got, want)
	}
	if got.Quantity != nil {
		t.Fatalf("Quantity = %v, want nil for a fixture that never set it", got.Quantity)
	}
}

// runSchema reads schema/run.v2.json through schemaDir — the gate the
// tests below actually enforce: these compat tests previously only
// exercised encoding/json on Go structs, so they would still pass if the
// schema added additionalProperties:false or made a new field required
// without any Go-side signal.
func runSchema(t *testing.T) []byte {
	t.Helper()
	schema, err := fs.ReadFile(schemaDir, "run.v2.json")
	if err != nil {
		t.Fatalf("read schema/run.v2.json: %v", err)
	}
	return schema
}

// baseRunDocument returns a real "evo.run" document (one task, one Problem)
// produced by EncodeRun — a known-valid baseline whose data.tasks[0] and
// data.problems[0] a test below overwrites with an older- or current-shape
// fixture, so every other required field (schema_version, run_id, evidence,
// progress, timing, ...) stays populated exactly as production code emits
// it instead of being hand-guessed.
func baseRunDocument(t *testing.T) map[string]any {
	t.Helper()
	result := core.Result{Conclusion: withConc(func(c *core.Conclusion) {
		c.Tasks = []core.TaskSnapshot{
			core.NewTaskSnapshot(core.TaskSnapshot{
				ID: "task_1", Name: "build", State: core.Done,
				Problems: []core.Problem{{Code: "A1", Summary: "finding", Subject: "file.go", Detail: "why", Count: 2, Unit: "line"}},
			}, time.Time{}, false, false),
		}
	})}
	encoded, err := EncodeRun(result, testEvoVersion)
	if err != nil {
		t.Fatalf("EncodeRun: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(encoded, &doc); err != nil {
		t.Fatalf("decode baseline evo.run document: %v", err)
	}
	return doc
}

// validateSpliced re-marshals doc and validates it against schema/run.v2.json.
func validateSpliced(t *testing.T, doc map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal spliced document: %v", err)
	}
	if err := wireschema.Validate(runSchema(t), encoded); err != nil {
		t.Fatalf("spliced document does not conform to schema/run.v2.json:\n%v\n\ndocument:\n%s", err, encoded)
	}
}

func firstTask(doc map[string]any) map[string]any {
	return doc["data"].(map[string]any)["tasks"].([]any)[0].(map[string]any)
}

// firstProblem returns data.tasks[0].problems[0] — the run's top-level
// data.problems is always empty today (ToRunDocument only ever projects
// Problems per-task); TaskDoc.Problems is where a real ProblemDoc lives.
func firstProblem(doc map[string]any) map[string]any {
	return firstTask(doc)["problems"].([]any)[0].(map[string]any)
}

// TestSchemaCompat_ProblemDocOlderShapeValidatesAgainstSchema proves an
// older-shape Problem (written before evidence_tail existed) still
// validates against today's schema/run.v2.json — not merely decodes into
// today's Go type.
func TestSchemaCompat_ProblemDocOlderShapeValidatesAgainstSchema(t *testing.T) {
	doc := baseRunDocument(t)
	older := firstProblem(doc)
	delete(older, "evidence_tail")
	validateSpliced(t, doc)
}

// TestSchemaCompat_ProblemDocCurrentShapeValidatesAgainstSchema proves the
// additive evidence_tail field itself is schema-declared, guarding against
// the exact gap this ticket closed: run.v2.json carried "verification.facts"
// but not "problem.evidence_tail" despite wire.ProblemDoc emitting it.
func TestSchemaCompat_ProblemDocCurrentShapeValidatesAgainstSchema(t *testing.T) {
	doc := baseRunDocument(t)
	firstProblem(doc)["evidence_tail"] = "tail text"
	validateSpliced(t, doc)
}

// TestSchemaCompat_VerificationDocOlderShapeValidatesAgainstSchema proves
// an older-shape VerificationDoc (no facts) still validates.
func TestSchemaCompat_VerificationDocOlderShapeValidatesAgainstSchema(t *testing.T) {
	doc := baseRunDocument(t)
	firstTask(doc)["verification"] = []any{
		map[string]any{"name": "contents", "status": "satisfied"},
	}
	validateSpliced(t, doc)
}

// TestSchemaCompat_VerificationDocCurrentShapeValidatesAgainstSchema
// proves VerificationDoc.Facts — Patch/File's per-attribute machine
// provenance — validates against schema/run.v2.json's verification.facts.
func TestSchemaCompat_VerificationDocCurrentShapeValidatesAgainstSchema(t *testing.T) {
	doc := baseRunDocument(t)
	firstTask(doc)["verification"] = []any{
		map[string]any{
			"name": "permissions", "status": "error",
			"facts": []any{
				map[string]any{"name": "error", "value": "operation not permitted"},
				map[string]any{"name": "path", "value": fixtureResourcePath},
			},
		},
	}
	validateSpliced(t, doc)
}

// TestSchemaCompat_TrackedResourceDocShapesValidateAgainstSchema proves
// TrackedResourceDoc's older shape (no mode) and current shape (with mode)
// both validate against schema/run.v2.json's trackedResource $def.
func TestSchemaCompat_TrackedResourceDocShapesValidateAgainstSchema(t *testing.T) {
	older := baseRunDocument(t)
	firstTask(older)["tracked_resources"] = []any{
		map[string]any{"kind": "file", "path": fixtureResourcePath, "fingerprint": "sha256:abc"},
	}
	validateSpliced(t, older)

	current := baseRunDocument(t)
	firstTask(current)["tracked_resources"] = []any{
		map[string]any{"kind": "file", "path": fixtureResourcePath, "mode": "0644"},
	}
	validateSpliced(t, current)
}

func TestSchemaCompat_EffectDocQuantityIsAdditive(t *testing.T) {
	qty := int64(3)
	current := EffectDoc{Subject: "build", Status: "changed", Verb: "created", Object: "file", Quantity: &qty}
	encoded, err := json.Marshal(current)
	if err != nil {
		t.Fatalf("encode current EffectDoc: %v", err)
	}
	type oldConsumerEffectDoc struct {
		Subject string `json:"subject"`
		Status  string `json:"status"`
		Verb    string `json:"verb"`
		Object  string `json:"object"`
	}
	var old oldConsumerEffectDoc
	if err := json.Unmarshal(encoded, &old); err != nil {
		t.Fatalf("old consumer must decode today's document ignoring quantity: %v", err)
	}
	if old.Subject != current.Subject || old.Status != current.Status || old.Verb != current.Verb || old.Object != current.Object {
		t.Fatalf("old consumer decode = %+v, want fields it knows about preserved", old)
	}
}
