package wire

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
	"github.com/zachbornheimer/evident-output/internal/wireschema"
)

// TestToRunDocument_SkippedTaskEmitsSkippedDispositionOnly pins E-120: the
// vocabulary freeze removed "kept" from the wire enum (schema/run.v2.json's
// "disposition" $def), with no compatibility value — a Skipped Task's
// disposition record can only ever read "skipped", never the retired
// "kept". This is the one wire fixture that actually exercises a
// disposition record; TestToRunDocument_ValidatesAgainstSchema's fixtures
// never populate TaskSnapshot.Skipped, so a stale "kept" alias in the
// schema went unnoticed until this test.
func TestToRunDocument_SkippedTaskEmitsSkippedDispositionOnly(t *testing.T) {
	result := core.Result{Conclusion: withConc(func(c *core.Conclusion) {
		c.State = core.StateReady
		c.ExitCode = core.ExitOK
		c.Tasks = []core.TaskSnapshot{
			core.NewTaskSnapshot(core.TaskSnapshot{
				ID: "task_1", Key: "branches", Name: "branches", State: core.Skipped,
				Skipped: []core.TaxonomyRecord{{Reason: "protected", Name: "branches"}},
			}, time.Time{}, false),
		}
	})}

	doc, err := EncodeRun(result, testEvoVersion)
	if err != nil {
		t.Fatalf("EncodeRun: %v", err)
	}

	var parsed struct {
		Data struct {
			Tasks []struct {
				Dispositions []struct {
					Disposition string `json:"disposition"`
					Reason      string `json:"reason"`
				} `json:"dispositions"`
			} `json:"tasks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(doc, &parsed); err != nil {
		t.Fatalf("unmarshal evo.run document: %v", err)
	}
	tasks := parsed.Data.Tasks
	if len(tasks) != 1 || len(tasks[0].Dispositions) != 1 {
		t.Fatalf("expected exactly one disposition record, got: %s", doc)
	}
	if got := tasks[0].Dispositions[0].Disposition; got != "skipped" {
		t.Fatalf("wire disposition = %q, want the sole 1.1 value %q", got, "skipped")
	}

	schema, err := os.ReadFile("../../schema/run.v2.json")
	if err != nil {
		t.Fatalf("read schema/run.v2.json: %v", err)
	}
	if err := wireschema.Validate(schema, doc); err != nil {
		t.Fatalf("evo.run document does not conform to schema/run.v2.json:\n%v\n\ndocument:\n%s", err, doc)
	}
}

// TestSchema_DispositionField_RejectsRetiredKeptValue pins the schema
// change directly: "kept" must fail the disposition $def now that the
// vocabulary freeze retired it with no compatibility alias. Before this
// lane's schema fix, the $def's enum still listed "kept" alongside
// "skipped" (a leftover from when TaskHandle.Kept existed), so a machine
// consumer reading the schema alone could not tell "kept" was gone.
func TestSchema_DispositionField_RejectsRetiredKeptValue(t *testing.T) {
	schema, err := os.ReadFile("../../schema/run.v2.json")
	if err != nil {
		t.Fatalf("read schema/run.v2.json: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(schema, &root); err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	defs := root["$defs"].(map[string]any)
	dispositionDef, ok := defs["disposition"].(map[string]any)
	if !ok {
		t.Fatalf("schema/run.v2.json has no $defs.disposition")
	}
	defSchema, err := json.Marshal(map[string]any{
		"$defs":  defs,
		"$ref":   "#/$defs/disposition",
		"__self": dispositionDef,
	})
	if err != nil {
		t.Fatalf("marshal wrapper schema: %v", err)
	}
	keptDoc := []byte(`{"disposition":"kept","reason":"protected","name":"branches"}`)
	if err := wireschema.Validate(defSchema, keptDoc); err == nil {
		t.Fatalf("schema/run.v2.json's disposition $def still accepts the retired \"kept\" value")
	}
	skippedDoc := []byte(`{"disposition":"skipped","reason":"protected","name":"branches"}`)
	if err := wireschema.Validate(defSchema, skippedDoc); err != nil {
		t.Fatalf("schema/run.v2.json's disposition $def rejects the sole 1.1 value \"skipped\": %v", err)
	}
}
