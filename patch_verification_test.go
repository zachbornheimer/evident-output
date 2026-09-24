package evo_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/wireschema"
)

// repoRoot is the read-only fs.FS this file reads schema/run.v2.json
// through, matching wire_schema_test.go's os.ReadFile("schema/...") calls
// at the same package level but going through an injectable fs.FS. It is
// captured as an absolute path at package init, before any test in this
// package calls t.Chdir, so a later per-test working-directory change (this
// test chdirs into a temp workspace so Patch's diff paths resolve there)
// never moves what "repo root" means for this fs.FS.
var repoRoot = mustAbsDirFS(".")

func mustAbsDirFS(dir string) fs.FS {
	abs, err := filepath.Abs(dir)
	if err != nil {
		panic("patch_verification_test: resolve repo root: " + err.Error())
	}
	return os.DirFS(abs)
}

// TestPatch_FileVerification_SurvivesEncodeRun is ZYS-823's end-to-end
// proof for the "Patch" acceptance bullet: a real evo.Patch-derived
// FileSet, committed through evo.File, must leave the per-attribute
// verification outcome that commit produced intact all the way through
// evo.WriteJSON's "evo.run" encoding and schema/run.v2.json validation —
// not merely round-trip through a hand-built core.VerificationDetail
// fixture the way internal/wire/compat_test.go's spliced-document tests
// do. This is Patch's own machine provenance, one hop past File's (Patch
// derives the desired state File then commits and verifies).
func TestPatch_FileVerification_SurvivesEncodeRun(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("draft\n"), 0o644); err != nil {
		t.Fatalf("seed notes.txt: %v", err)
	}
	const diff = "--- a/notes.txt\n+++ b/notes.txt\n@@ -1 +1 @@\n-draft\n+final\n"

	out := evo.Init(evo.Config{Isolated: true, StateDir: dir, Stdout: io.Discard, Stderr: io.Discard})
	result := out.Run(context.Background(), func(context.Context) error {
		task := out.Task("finalize notes")
		task.Define(func(ctx context.Context) error {
			set, err := evo.Patch(ctx, []byte(diff))
			if err != nil {
				return err
			}
			return evo.Files(ctx, set)
		})
		return task.Wait()
	})

	var body bytes.Buffer
	if err := evo.WriteJSON(&body, result); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}

	schema, err := fs.ReadFile(repoRoot, "schema/run.v2.json")
	if err != nil {
		t.Fatalf("read schema/run.v2.json: %v", err)
	}
	if err := wireschema.Validate(schema, body.Bytes()); err != nil {
		t.Fatalf("Patch-derived evo.run document does not conform to schema/run.v2.json:\n%v\n\ndocument:\n%s", err, body.String())
	}

	var doc struct {
		Data struct {
			Tasks []struct {
				Verification []struct {
					Name   string `json:"name"`
					Status string `json:"status"`
				} `json:"verification"`
			} `json:"tasks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body.Bytes(), &doc); err != nil {
		t.Fatalf("decode evo.run: %v", err)
	}
	if len(doc.Data.Tasks) != 1 || len(doc.Data.Tasks[0].Verification) == 0 {
		t.Fatalf("Patch-derived evo.run carries no per-attribute verification: %s", body.String())
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read committed notes.txt: %v", err)
	}
	if string(contents) != "final\n" {
		t.Fatalf("notes.txt = %q, want %q", contents, "final\n")
	}
}
