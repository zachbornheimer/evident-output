package evo_test

import (
	"io"
	"os"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/wireschema"
)

// TestWireSchema_RenderedDocumentValidates is the gate schema/output.v1.json
// never had (P8): a real evo.EncodeJSON(out.Snapshot()) document, covering a
// warned task (JSONTask.Warnings, wire 0.4) and a blocked run
// (ConclusionJSON.Warned), must validate against evo's own published JSON
// Schema — so a future wire change that drifts from the schema fails here
// instead of only being caught by a downstream consumer.
func TestWireSchema_RenderedDocumentValidates(t *testing.T) {
	schema, err := os.ReadFile("schema/output.v1.json")
	if err != nil {
		t.Fatalf("read schema/output.v1.json: %v", err)
	}

	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	out.Task("working tree").Done()
	out.Task("branches").Warn("2 branches need attention", evo.Detail("push before retiring"))
	seq := out.Sequence("cleanup")
	seq.Task("remove tags").Done()
	_ = out.Finish()

	doc, err := evo.EncodeJSON(out.Snapshot())
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	if err := wireschema.Validate(schema, doc); err != nil {
		t.Fatalf("rendered document does not conform to schema/output.v1.json:\n%v\n\ndocument:\n%s", err, doc)
	}
}
