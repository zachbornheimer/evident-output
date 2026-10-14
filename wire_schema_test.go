package evo

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/testkit"
	"github.com/zachbornheimer/evident-output/v2/testkit/wireschema"
)

// TestWireSchema_RenderedDocumentValidates is the gate the published schema
// needs beyond its generated-bytes check: a real evo.run document, covering a
// warned task and a sequence, must validate against evo's own published JSON
// Schema — so a future wire change that drifts from the schema fails here
// instead of only being caught by a downstream consumer.
func TestWireSchema_RenderedDocumentValidates(t *testing.T) {
	schema := strictRunSchema(t)

	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	succeed(out.Task("working tree"))
	out.Task("branches").Problem("2 branches need attention", severity(severityWarning))
	seq := out.Sequence("cleanup")
	succeed(seq.Task("remove tags"))
	_ = out.finish()

	doc := out.runDocument(t)
	if err := wireschema.Validate(schema, doc); err != nil {
		t.Fatalf("rendered document does not conform to "+runSchemaPath+":\n%v\n\ndocument:\n%s", err, doc)
	}
}

// TestWireSchema_RichProblemStaysWithinPublishedRunSchema pins that a Task
// whose Problem sets Location, Next and an EvidenceTail, beside a File
// verification failure with Facts, validates against the Strict run schema:
// every field the run document carries for them is declared by the schema.
func TestWireSchema_RichProblemStaysWithinPublishedRunSchema(t *testing.T) {
	schema := strictRunSchema(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "agent.plist")
	fsys := testkit.NewFileFS()
	fsys.FailChmod(path, errors.New("operation not permitted"))

	out := initRun(Config{State: State{Dir: dir}, Facades: Facades{Stderr: io.Discard, FS: fsys}})
	out.Task("write launch agent").Define(func(ctx context.Context) error {
		return File(ctx, FileSpec{Path: path, Contents: []byte("x"), Mode: 0o644})
	})
	lint := out.Task("lint")
	capture := lint.capture()
	_, _ = io.WriteString(capture.Stderr(), "main.go:12:3: undefined: x\n")
	_ = capture.Close()
	lint.fail("lint failed", detailTail(capture),
		location("main.go", 12, 3), nextCommand("go", "vet", "./..."))
	_ = out.finish()

	doc := out.runDocument(t)
	if err := wireschema.Validate(schema, doc); err != nil {
		t.Fatalf("run document grew past its published schema:\n%v\n\ndocument:\n%s", err, doc)
	}
}

// runSchemaPath is the published schema of the evo.run document.
const runSchemaPath = "schema/run.v3.json"

// strictRunSchema reads the published run schema with wireschema.Strict's
// additionalProperties:false overlay, so an undeclared field fails.
func strictRunSchema(t *testing.T) []byte {
	t.Helper()
	schema, err := os.ReadFile(runSchemaPath)
	if err != nil {
		t.Fatalf("read %s: %v", runSchemaPath, err)
	}
	strict, err := wireschema.Strict(schema)
	if err != nil {
		t.Fatalf("wireschema.Strict(%s): %v", runSchemaPath, err)
	}
	return strict
}
