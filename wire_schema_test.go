package evo_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/wireschema"
	"github.com/zachbornheimer/evident-output/testkit"
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
	schema, err = wireschema.Strict(schema)
	if err != nil {
		t.Fatalf("wireschema.Strict(output.v1.json): %v", err)
	}

	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
	succeed(out.Task("working tree"))
	out.Task("branches").Warn("2 branches need attention")
	seq := out.Sequence("cleanup")
	succeed(seq.Task("remove tags"))
	_ = out.Finish()

	doc, err := evo.EncodeJSON(out.Snapshot())
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	if err := wireschema.Validate(schema, doc); err != nil {
		t.Fatalf("rendered document does not conform to schema/output.v1.json:\n%v\n\ndocument:\n%s", err, doc)
	}
}

// TestWireSchema_TaskVerificationValidates is ZYS-823's schema-compat proof
// for JSONTask.Verification (render/json.go): a task whose evo.File attempt
// records a real permissions VerificationError — with Facts — must still
// validate against schema/output.v1.json now that "verification" is a
// declared task property, not merely round-trip through encoding/json.
func TestWireSchema_TaskVerificationValidates(t *testing.T) {
	schema, err := os.ReadFile("schema/output.v1.json")
	if err != nil {
		t.Fatalf("read schema/output.v1.json: %v", err)
	}
	schema, err = wireschema.Strict(schema)
	if err != nil {
		t.Fatalf("wireschema.Strict(output.v1.json): %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "agent.plist")
	fsys := testkit.NewFileFS()
	fsys.FailChmod(path, errors.New("operation not permitted"))

	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, FileFS: fsys, StateDir: dir})
	out.Task("write launch agent").Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: []byte("x"), Mode: 0o644})
	})
	_ = out.Finish()

	doc, err := evo.EncodeJSON(out.Snapshot())
	if err != nil {
		t.Fatalf("EncodeJSON: %v", err)
	}
	if err := wireschema.Validate(schema, doc); err != nil {
		t.Fatalf("rendered document does not conform to schema/output.v1.json:\n%v\n\ndocument:\n%s", err, doc)
	}
	var got struct {
		Tasks []struct {
			Verification []struct {
				Name  string `json:"name"`
				Facts []struct {
					Name string `json:"name"`
				} `json:"facts"`
			} `json:"verification"`
		} `json:"tasks"`
	}
	if err := json.Unmarshal(doc, &got); err != nil {
		t.Fatalf("decode rendered document: %v", err)
	}
	if len(got.Tasks) != 1 || len(got.Tasks[0].Verification) == 0 {
		t.Fatalf("rendered document carries no verification: %s", doc)
	}
}
