package evo

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/project/wire"

	"github.com/zachbornheimer/evident-output/testkit"
	"github.com/zachbornheimer/evident-output/testkit/wireschema"
)

// TestWireSchema_RenderedDocumentValidates is the gate schema/output.v1.json
// never had (P8): a real wire.EncodeJSON(out.Snapshot()) document, covering a
// warned task (JSONTask.Warnings, wire 0.4) and a blocked run
// (ConclusionJSON.Warned), must validate against evo's own published JSON
// Schema — so a future wire change that drifts from the schema fails here
// instead of only being caught by a downstream consumer.
func TestWireSchema_RenderedDocumentValidates(t *testing.T) {
	schema := strictOutputSchema(t)

	out := initRun(Config{stdout: io.Discard})
	succeed(out.Task("working tree"))
	out.Task("branches").Problem("2 branches need attention", severity(severityWarning))
	seq := out.Sequence("cleanup")
	succeed(seq.Task("remove tags"))
	_ = out.finish()

	doc, err := wire.EncodeJSON(out.snapshot())
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	if err := wireschema.Validate(schema, doc); err != nil {
		t.Fatalf("rendered document does not conform to schema/output.v1.json:\n%v\n\ndocument:\n%s", err, doc)
	}
}

// TestWireSchema_RichProblemStaysWithinFrozenOutputV1 pins the 1.1 API
// freeze on wire.EncodeJSON: a Task whose Problem sets Location, Next and an
// EvidenceTail, beside a File verification failure with Facts, must still
// validate against the Strict output.v1 schema. None of that data may leak
// into output.v1 as an undeclared field; machine consumers read it from the
// run document (internal/wire), which carries all of it.
func TestWireSchema_RichProblemStaysWithinFrozenOutputV1(t *testing.T) {
	schema := strictOutputSchema(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "agent.plist")
	fsys := testkit.NewFileFS()
	fsys.FailChmod(path, errors.New("operation not permitted"))

	out := initRun(Config{stdout: io.Discard, fileFS: fsys, stateDir: dir})
	out.Task("write launch agent").Define(func(ctx context.Context) error {
		return File(ctx, FileSpec{Path: path, Contents: []byte("x"), Mode: 0o644})
	})
	lint := out.Task("lint")
	capture := lint.capture()
	_, _ = io.WriteString(capture.Stderr(), "main.go:12:3: undefined: x\n")
	_ = capture.Close()
	lint.fail("lint failed", capture.DetailTail(),
		location("main.go", 12, 3), nextCommand("go", "vet", "./..."))
	_ = out.finish()

	doc, err := wire.EncodeJSON(out.snapshot())
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	if err := wireschema.Validate(schema, doc); err != nil {
		t.Fatalf("output.v1 grew past its frozen schema:\n%v\n\ndocument:\n%s", err, doc)
	}
}

// strictOutputSchema reads schema/output.v1.json with wireschema.Strict's
// additionalProperties:false overlay, so an undeclared field fails.
func strictOutputSchema(t *testing.T) []byte {
	t.Helper()
	schema, err := os.ReadFile("schema/output.v1.json")
	if err != nil {
		t.Fatalf("read schema/output.v1.json: %v", err)
	}
	strict, err := wireschema.Strict(schema)
	if err != nil {
		t.Fatalf("wireschema.Strict(output.v1.json): %v", err)
	}
	return strict
}
