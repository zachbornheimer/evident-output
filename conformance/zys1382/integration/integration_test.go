// Package integration_test exercises the ZYS-1382 primitives together:
// each flow crosses at least two of File, Tree, Checksum, Download,
// Extract, Exec, Basis, and Patch through the public evo package only.
package integration_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"sync/atomic"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// npmPackage is a small npm-style package: every entry sits under the
// conventional "package/" root that npm tarballs carry.
var npmPackage = map[string]string{
	"package.json":     `{"name":"left-pad","version":"1.3.0","main":"index.js"}`,
	"index.js":         "module.exports = require('./lib/pad');\n",
	"lib/pad.js":       "module.exports = (s, n) => String(s).padStart(n);\n",
	"README.md":        "# left-pad\n",
	"lib/util/noop.js": "module.exports = () => {};\n",
}

// contractRun runs fn as one Task's Define callback in an isolated Output
// and returns its error.
func contractRun(t *testing.T, cfg evo.Config, fn func(ctx context.Context) error) error {
	t.Helper()
	cfg.Isolated, cfg.Plain = true, true
	cfg.Stdout, cfg.Stderr = io.Discard, io.Discard
	if cfg.StateDir == "" {
		cfg.StateDir = t.TempDir()
	}
	out := evo.Init(cfg)
	ran := false
	task := out.Task("integration").Define(func(ctx context.Context) error {
		ran = true
		return fn(ctx)
	})
	err := task.Wait()
	_ = out.Finish()
	if !ran {
		t.Fatal("integration Task callback never ran")
	}
	return err
}

// plant writes files (slash-separated relative path -> content) under root.
func plant(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// npmTarball builds a gzip tar of files under "package/", in a fixed order
// so the bytes (and so the Integrity) are deterministic.
func npmTarball(t *testing.T, files map[string]string) []byte {
	t.Helper()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, name := range names {
		body := files[name]
		hdr := &tar.Header{Name: "package/" + name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha512SRI(data []byte) string {
	sum := sha512.Sum512(data)
	return "sha512-" + base64.StdEncoding.EncodeToString(sum[:])
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// A registry tarball is downloaded under its Integrity, extracted with its
// "package" root stripped, and the resulting tree's checksum equals the
// checksum of the same files planted by hand. A second pass is satisfied
// without touching the network.
func TestDownloadExtractChecksumOfAnNpmTarball(t *testing.T) {
	tarball := npmTarball(t, npmPackage)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write(tarball)
	}))
	t.Cleanup(srv.Close)

	work := t.TempDir()
	archive := evo.File{
		Path:    filepath.Join(work, "cache", "left-pad-1.3.0.tgz"),
		Content: evo.Download{URL: srv.URL + "/left-pad/-/left-pad-1.3.0.tgz", Integrity: sha512SRI(tarball)},
	}
	pkg := evo.Tree{Path: filepath.Join(work, "node_modules", "left-pad"), Content: evo.Extract{File: archive, Root: "package"}}
	install := func(ctx context.Context) error {
		if err := os.MkdirAll(filepath.Dir(archive.Path), 0o755); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(pkg.Path), 0o755); err != nil {
			return err
		}
		if err := archive.Write(ctx); err != nil {
			return err
		}
		return pkg.Write(ctx)
	}
	var installed string
	if err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		if err := install(ctx); err != nil {
			return err
		}
		var err error
		installed, err = pkg.Checksum(ctx)
		return err
	}); err != nil {
		t.Fatalf("install: %v", err)
	}

	reference := filepath.Join(t.TempDir(), "left-pad")
	plant(t, reference, npmPackage)
	want, err := evo.Tree{Path: reference}.Checksum(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if installed != want {
		t.Fatalf("installed tree checksum %s, want the hand-planted tree's %s", installed, want)
	}
	leaf, err := evo.File{Path: filepath.Join(pkg.Path, "package.json")}.Checksum(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if leaf != sha256Hex([]byte(npmPackage["package.json"])) {
		t.Fatalf("extracted package.json checksum %s, want the SHA-256 of its bytes", leaf)
	}

	if err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		if err := install(ctx); err != nil {
			return err
		}
		return pkg.Verify(ctx)
	}); err != nil {
		t.Fatalf("second install: %v", err)
	}
	if n := hits.Load(); n != 1 {
		t.Fatalf("registry served %d requests, want 1 (the satisfied archive must not refetch)", n)
	}
}

// basisPass runs one Run of a Task whose Basis is inputs against a shared
// StateDir and reports whether its Define callback ran.
func basisPass(t *testing.T, state string, inputs ...any) bool {
	t.Helper()
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: state})
	ran := false
	task := out.Task("build")
	for _, in := range inputs {
		switch v := in.(type) {
		case evo.File:
			task.Basis(v)
		case evo.Tree:
			task.Basis(v)
		}
	}
	task.Define(func(context.Context) error { ran = true; return nil })
	if err := task.Wait(); err != nil {
		t.Fatalf("build: %v", err)
	}
	_ = out.Finish()
	return ran
}

// A Task's Basis mixes a File and a Tree; any content change to either
// reruns it, while unchanged content (and a mode-only change) keeps it
// current.
func TestBasisWithFileAndTreeInputsInvalidatesTask(t *testing.T) {
	work, state := t.TempDir(), t.TempDir()
	plant(t, work, map[string]string{"package.json": `{"v":1}`, "src/a.js": "a", "src/b.js": "b"})
	config := evo.File{Path: filepath.Join(work, "package.json")}
	sources := evo.Tree{Path: filepath.Join(work, "src")}
	steps := []struct {
		name    string
		prepare func()
		wantRun bool
	}{
		{"first run", func() {}, true},
		{"unchanged", func() {}, false},
		{"File content changed", func() { plant(t, work, map[string]string{"package.json": `{"v":2}`}) }, true},
		{"unchanged after File change", func() {}, false},
		{"Tree gained a file", func() { plant(t, work, map[string]string{"src/c.js": "c"}) }, true},
		{"Tree leaf content changed", func() { plant(t, work, map[string]string{"src/a.js": "A"}) }, true},
		{"Tree leaf mode-only change", func() {
			if err := os.Chmod(filepath.Join(work, "src", "b.js"), 0o600); err != nil {
				t.Fatal(err)
			}
		}, false},
		{"Tree lost a file", func() { _ = os.Remove(filepath.Join(work, "src", "c.js")) }, true},
	}
	for _, step := range steps {
		step.prepare()
		if got := basisPass(t, state, config, sources); got != step.wantRun {
			t.Fatalf("%s: Task ran = %v, want %v", step.name, got, step.wantRun)
		}
	}
}

// One Run of a two-Task pipeline: "generate" runs an Exec that declares a
// Tree Output, and "consume" names that Output as its Basis. It reports
// which Define callbacks ran.
func pipelinePass(t *testing.T, work, state string) (generated, consumed bool) {
	t.Helper()
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: state})
	gen := out.Task("generate").Basis(evo.File{Path: filepath.Join(work, "input.txt")}).Define(func(ctx context.Context) error {
		generated = true
		_, err := evo.Exec{
			Path:    "/bin/sh",
			Args:    []string{"-c", "rm -rf gen && mkdir gen && cp input.txt gen/a.txt && printf fixed > gen/b.txt"},
			Dir:     work,
			Outputs: evo.Outputs{evo.Tree{Path: "gen"}},
		}.Run(ctx)
		return err
	})
	use := out.Task("consume").After(gen).Basis(evo.Tree{Path: filepath.Join(work, "gen")}).Define(func(context.Context) error {
		consumed = true
		return nil
	})
	if err := use.Wait(); err != nil {
		t.Fatalf("pipeline: %v", err)
	}
	_ = out.Finish()
	return generated, consumed
}

// Exec Outputs and Basis compose across Tasks: regenerating an Output with
// new content reruns the Task whose Basis names it; tampering with the
// Output reruns the producer, which restores the recorded identity, so the
// consumer stays current.
func TestExecOutputsFeedADownstreamBasis(t *testing.T) {
	work, state := t.TempDir(), t.TempDir()
	plant(t, work, map[string]string{"input.txt": "v1"})
	steps := []struct {
		name               string
		prepare            func()
		wantGen, wantUsing bool
	}{
		{"first run", func() {}, true, true},
		{"unchanged", func() {}, false, false},
		{"input changed", func() { plant(t, work, map[string]string{"input.txt": "v2"}) }, true, true},
		{"Output tampered", func() { plant(t, work, map[string]string{"gen/b.txt": "tampered"}) }, true, false},
		{"Output removed", func() { _ = os.RemoveAll(filepath.Join(work, "gen")) }, true, false},
		{"settled", func() {}, false, false},
	}
	for _, step := range steps {
		step.prepare()
		gen, using := pipelinePass(t, work, state)
		if gen != step.wantGen || using != step.wantUsing {
			t.Fatalf("%s: generate ran = %v, consume ran = %v; want %v, %v", step.name, gen, using, step.wantGen, step.wantUsing)
		}
	}
}

const integrationPatch = `diff --git a/config.txt b/config.txt
--- a/config.txt
+++ b/config.txt
@@ -1,3 +1,3 @@
 alpha
-beta
+BETA
 gamma
`

// Patch commits through File: the edited file is replaced atomically (new
// inode, no temporaries), verifies as a File with the patched bytes, has
// the File checksum of those bytes, and invalidates a Task whose Basis
// names it.
func TestPatchCommitsThroughFileMachinery(t *testing.T) {
	work, state := t.TempDir(), t.TempDir()
	plant(t, work, map[string]string{"config.txt": "alpha\nbeta\ngamma\n"})
	t.Chdir(work)
	target := filepath.Join(work, "config.txt")
	if !basisPass(t, state, evo.File{Path: target}) {
		t.Fatal("first run of the Basis Task did not run")
	}
	before, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		return evo.Patch(ctx, []byte(integrationPatch))
	}); err != nil {
		t.Fatalf("Patch: %v", err)
	}
	after, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Fatal("Patch rewrote the file in place; File commits replace it atomically")
	}
	entries, err := os.ReadDir(work)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("Patch left %d entries beside the file, want only config.txt", len(entries))
	}
	want := []byte("alpha\nBETA\ngamma\n")
	if err := contractRun(t, evo.Config{}, evo.File{Path: target, Content: evo.Bytes(want)}.Verify); err != nil {
		t.Fatalf("patched file does not verify as File{Content: Bytes(result)}: %v", err)
	}
	sum, err := evo.File{Path: target}.Checksum(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if sum != sha256Hex(want) {
		t.Fatalf("patched file checksum %s, want the SHA-256 of the patched bytes", sum)
	}
	if !basisPass(t, state, evo.File{Path: target}) {
		t.Fatal("Patch changed a Basis File, but the Task stayed current")
	}
	if err := contractRun(t, evo.Config{}, func(ctx context.Context) error {
		return evo.Patch(ctx, []byte(integrationPatch))
	}); err != nil {
		t.Fatalf("re-applying an applied Patch = %v, want nil", err)
	}
	if basisPass(t, state, evo.File{Path: target}) {
		t.Fatal("re-applying an applied Patch changed the file")
	}
}
