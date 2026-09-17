package evo_test

import (
	"encoding/json"
	"fmt"
	"io"

	evo "github.com/zachbornheimer/evident-output"
)

// ExampleEncodeJSON encodes a finished Snapshot as the final wire
// JSONDocument (§25.1).
func ExampleEncodeJSON() {
	out := evo.Init(evo.Config{Stdout: io.Discard, Stderr: io.Discard, Plain: true, Isolated: true})
	out.Task("apply patch").Done()
	_ = out.Finish()
	data, err := evo.EncodeJSON(out.Snapshot())
	var doc evo.JSONDocument
	_ = json.Unmarshal(data, &doc)
	fmt.Println(err, doc.Conclusion.State)
	// Output:
	// <nil> ready
}

// ExampleEncodeJSONL encodes durable events as JSON Lines (§25.2).
func ExampleEncodeJSONL() {
	events := []evo.Event{{Type: "task.done", Name: "apply patch"}}
	data, err := evo.EncodeJSONL(events)
	fmt.Println(err)
	fmt.Println(len(data) > 0)
	// Output:
	// <nil>
	// true
}

// ExampleEncodeEventJSON encodes one journal event as a single JSON object,
// with no trailing newline.
func ExampleEncodeEventJSON() {
	data, err := evo.EncodeEventJSON(evo.Event{Type: "task.done", Name: "apply patch"})
	fmt.Println(err)
	fmt.Println(string(data))
	// Output:
	// <nil>
	// {"schema_version":"0.3","sequence":0,"type":"task.done","name":"apply patch","timestamp":"0001-01-01T00:00:00Z"}
}

// ExampleJSONDocument shows the final machine projection's top-level shape.
func ExampleJSONDocument() {
	doc := evo.JSONDocument{SchemaVersion: evo.JSONSchemaVersion}
	fmt.Println(doc.SchemaVersion)
	// Output:
	// 0.4
}

// ExampleJSONMessage shows a wire-format user-facing message.
func ExampleJSONMessage() {
	m := evo.JSONMessage{Visibility: "normal", Text: "reading configuration"}
	fmt.Println(m.Visibility, m.Text)
	// Output:
	// normal reading configuration
}

// ExampleJSONOutputMeta identifies the output instance on the wire.
func ExampleJSONOutputMeta() {
	meta := evo.JSONOutputMeta{Subject: "repo-retire"}
	fmt.Println(meta.Subject)
	// Output:
	// repo-retire
}

// ExampleConclusionJSON shows the JSON-friendly conclusion projection.
func ExampleConclusionJSON() {
	c := evo.ConclusionJSON{State: evo.StateReady, ExitCode: evo.ExitOK}
	fmt.Println(c.State, c.ExitCode)
	// Output:
	// ready 0
}

// ExampleJSONProblem shows a wire-format problem (no raw Cause by default).
func ExampleJSONProblem() {
	p := evo.JSONProblem{Summary: "schema mismatch", Code: "E_SCHEMA"}
	fmt.Println(p.Summary, p.Code)
	// Output:
	// schema mismatch E_SCHEMA
}

// ExampleJSONTask shows a wire-format task.
func ExampleJSONTask() {
	t := evo.JSONTask{Name: "apply patch", State: evo.Done}
	fmt.Println(t.Name, t.State)
	// Output:
	// apply patch done
}

// ExampleJSONProgress shows wire-format progress.
func ExampleJSONProgress() {
	p := evo.JSONProgress{Kind: evo.Determinate, Completed: 50, Total: 100}
	fmt.Println(p.Kind, p.Completed, p.Total)
	// Output:
	// determinate 50 100
}

// ExampleJSONCollection shows a wire-format task collection with child IDs
// (§25.1).
func ExampleJSONCollection() {
	c := evo.JSONCollection{Name: "packages", Children: []string{"curl"}}
	fmt.Println(c.Name, c.Children)
	// Output:
	// packages [curl]
}

// ExampleJSONChanges shows wire-format changes.
func ExampleJSONChanges() {
	c := evo.JSONChanges{Subject: "branches", Records: []evo.JSONEffectRecord{{Verb: "deleted", Object: "stale branch"}}}
	fmt.Println(c.Subject, c.Records[0].Verb)
	// Output:
	// branches deleted
}

// ExampleJSONPlan shows wire-format plan.
func ExampleJSONPlan() {
	p := evo.JSONPlan{Subject: "branches", Records: []evo.JSONEffectRecord{{Verb: "delete", Object: "stale branch"}}}
	fmt.Println(p.Subject, p.Records[0].Verb)
	// Output:
	// branches delete
}

// ExampleJSONEffectRecord shows a change/plan row on the wire.
func ExampleJSONEffectRecord() {
	q := int64(2)
	r := evo.JSONEffectRecord{Verb: "deleted", Quantity: &q, Object: "stale branch"}
	fmt.Println(r.Verb, *r.Quantity, r.Object)
	// Output:
	// deleted 2 stale branch
}

// ExampleJSONAction shows a wire-format recommended next step.
func ExampleJSONAction() {
	a := evo.JSONAction{Label: "retry with --force"}
	fmt.Println(a.Label)
	// Output:
	// retry with --force
}

// ExampleJSONCommand shows argv for display on the wire.
func ExampleJSONCommand() {
	c := evo.JSONCommand{Executable: "git", Args: []string{"push", "--force-with-lease"}}
	fmt.Println(c.Executable, c.Args)
	// Output:
	// git [push --force-with-lease]
}

// ExampleEventJSON shows a JSON Lines event record (§25.2).
func ExampleEventJSON() {
	e := evo.EventJSON{Type: "task.done", Name: "apply patch"}
	fmt.Println(e.Type, e.Name)
	// Output:
	// task.done apply patch
}
