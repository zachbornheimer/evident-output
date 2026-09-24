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
// failed one — is VerificationDoc; TrackedResourceDoc.Mode and
// VerificationDoc.Facts are what carry that File-provenance data today).
// "Problem" maps to ProblemDoc. BasisDoc (the "basis" $def) is a distinct,
// currently-always-empty wire field with no core data source yet — see
// ZYS-978, filed to design where that gets captured — so it has its own
// TestSchemaCompat_BasisDocEmptyShapeValidatesAgainstSchema below rather
// than an older/current-shape pair like the others. fixtureResourcePath is
// an obviously-fake path (never touches disk — these tests only exercise
// json.Marshal/Unmarshal on in-memory structs).
const fixtureResourcePath = "fixture://managed/x"

// runSchema reads schema/run.v2.json through schemaDir and strengthens it
// with wireschema.Strict — the gate the tests below actually enforce:
// these compat tests previously only exercised encoding/json on Go
// structs, so they would still pass if the schema dropped a field it used
// to declare (a reverted addition) without any Go-side signal. The
// published schema itself stays permissive (no additionalProperties:false
// on disk — see wireschema.Strict's doc comment for why); Strict is what
// turns "declared" into "enforced" for this test's own validation, without
// making every future additive field a breaking schema_version bump for
// real consumers.
func runSchema(t *testing.T) []byte {
	t.Helper()
	strict, err := wireschema.Strict(rawRunSchema(t))
	if err != nil {
		t.Fatalf("wireschema.Strict(run.v2.json): %v", err)
	}
	return strict
}

// rawRunSchema reads schema/run.v2.json through schemaDir exactly as
// published, with no strictness overlay — the shape a real consumer
// pinned to today's schema actually validates against.
func rawRunSchema(t *testing.T) []byte {
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
			}, time.Time{}, false),
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

// TestSchemaCompat_BasisDocEmptyShapeValidatesAgainstSchema proves the
// always-empty "basis" array toTaskDoc emits today validates against
// schema/run.v2.json's basis $def, and that a populated BasisDoc (the
// shape ZYS-978 will start emitting once core has a data source) is
// already schema-valid too — the $def is ready before the producer is.
func TestSchemaCompat_BasisDocEmptyShapeValidatesAgainstSchema(t *testing.T) {
	empty := baseRunDocument(t)
	firstTask(empty)["basis"] = []any{}
	validateSpliced(t, empty)

	populated := baseRunDocument(t)
	firstTask(populated)["basis"] = []any{
		map[string]any{"kind": "template", "value": fixtureResourcePath},
	}
	validateSpliced(t, populated)
}

// TestSchemaCompat_PublishedSchemaStaysPermissiveForFutureAdditions is the
// other half of the additionalProperties fix: this test validates a real
// document that carries a field never declared in the schema against the
// raw published schema/run.v2.json (rawRunSchema, not runSchema(t)'s
// wireschema.Strict-wrapped copy every other test in this file uses), and
// requires that to still pass — so a consumer pinned to today's schema
// does not break the day evo adds the next Problem/resource/Patch field.
func TestSchemaCompat_PublishedSchemaStaysPermissiveForFutureAdditions(t *testing.T) {
	doc := baseRunDocument(t)
	firstProblem(doc)["future_field_not_yet_declared"] = "value from a later evo version"
	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal spliced document: %v", err)
	}
	if err := wireschema.Validate(rawRunSchema(t), encoded); err != nil {
		t.Fatalf("published schema/run.v2.json must stay additive-compatible, got: %v", err)
	}
}
