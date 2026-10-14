package evo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/project/wire"
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
	out.Task("branches").Problem("2 branches need attention")
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

// shippedSchemaVersions maps each published wire document kind to the schema_version the encoder writes.
var shippedSchemaVersions = map[string]string{
	"run":   wire.RunSchemaVersion,
	"event": wire.EventSchemaVersion,
}

// TestSchemaVersionConstMatchesFile pins one schema per wire document, named <kind>.v<N>.json, whose
// schema_version const is N.0 and the version the encoder writes: a consumer reading a file's name knows
// the version it validates, and a stale schema cannot linger beside the current one.
func TestSchemaVersionConstMatchesFile(t *testing.T) {
	files, err := filepath.Glob("schema/*.json")
	if err != nil {
		t.Fatalf("glob schema: %v", err)
	}
	seen := map[string]string{}
	for _, file := range files {
		kind, major, ok := strings.Cut(strings.TrimSuffix(filepath.Base(file), ".json"), ".v")
		if _, shipped := shippedSchemaVersions[kind]; !ok || !shipped {
			t.Errorf("%s is not a shipped wire schema (<kind>.v<N>.json for kind run or event)", file)
			continue
		}
		if prior, dup := seen[kind]; dup {
			t.Errorf("two schemas for the %s document: %s and %s; keep one per wire version", kind, prior, file)
		}
		seen[kind] = file

		body, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		var schema struct {
			Properties struct {
				SchemaVersion struct {
					Const string `json:"const"`
				} `json:"schema_version"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(body, &schema); err != nil {
			t.Fatalf("decode %s: %v", file, err)
		}
		got := schema.Properties.SchemaVersion.Const
		if want := major + ".0"; got != want {
			t.Errorf("%s declares schema_version const %q, want %q from its file name", file, got, want)
		}
		if want := shippedSchemaVersions[kind]; got != want {
			t.Errorf("%s declares schema_version const %q, but the encoder writes %q", file, got, want)
		}
	}
	for kind := range shippedSchemaVersions {
		if seen[kind] == "" {
			t.Errorf("no schema/%s.v<N>.json is published", kind)
		}
	}
}
