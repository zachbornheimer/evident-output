package evo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/freshness"
)

// manifestOutput is an isolated plain Output whose manifest lives in
// state and whose human stream is captured in the returned buffer.
func manifestOutput(state string) (*Run, *bytes.Buffer) {
	var buf bytes.Buffer
	out := initRun(Config{
		plain: true, color: colorNever, stateDir: state,
		stdout: &buf, stderr: &buf,
	})
	return out, &buf
}

// TestFinishPersistsManifestWithoutClose: an Init+Finish caller that never
// calls Close still writes every record the run committed or staged.
func TestFinishPersistsManifestWithoutClose(t *testing.T) {
	state := t.TempDir()
	out, _ := manifestOutput(state)
	t.Cleanup(func() { _ = out.close() })

	if err := runFileTask(t, out, "file", FileSpec{Path: filepath.Join(t.TempDir(), "f.txt"), Contents: []byte("x")}); err != nil {
		t.Fatalf("file task: %v", err)
	}
	opaque, opaqueKey := taskManifestKey(t, out, "opaque")
	opaque.Define(func(context.Context) error { return nil })
	if err := opaque.wait(); err != nil {
		t.Fatalf("opaque task: %v", err)
	}
	if err := out.finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	path, err := freshness.LocateManifest(freshness.ManifestConfig{StateDir: state}, freshness.NewSystemManifestEnvironment())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("manifest not on disk after Finish: %v", err)
	}
	var doc freshness.ManifestDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if _, ok := doc.Tasks[opaqueKey]; !ok {
		t.Fatalf("staged opaque record missing after Finish without Close; tasks = %v", doc.Tasks)
	}
}

// TestManifestSaveFailureIsReported: a manifest that cannot be written is
// stated on the run and returned from Close, never dropped.
func TestManifestSaveFailureIsReported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	state := t.TempDir()
	t.Cleanup(func() { _ = os.Chmod(state, 0o700) })
	out, buf := manifestOutput(state)

	task := out.Task("file")
	task.Define(func(ctx context.Context) error {
		if err := File(ctx, FileSpec{Path: filepath.Join(t.TempDir(), "f.txt"), Contents: []byte("x")}); err != nil {
			return err
		}
		// The Task's record is committed after this callback returns, so
		// the write that follows finds the state directory read-only.
		return os.Chmod(state, 0o500)
	})
	if err := task.wait(); err != nil {
		t.Fatalf("file task: %v", err)
	}
	_ = out.finish()
	closeErr := out.close()

	if closeErr == nil || !strings.Contains(closeErr.Error(), "manifest") {
		t.Fatalf("Close() = %v, want the manifest write error", closeErr)
	}
	if !strings.Contains(buf.String(), "manifest not saved") {
		t.Fatalf("output does not say the manifest was not saved:\n%s", buf.String())
	}
}

// TestBasisJudgedAfterCloseIsErrClosed: judging a Task's Basis once the
// Output closed reports ErrClosed, the one "closed" sentinel File and Exec
// report through manifestFor.
func TestBasisJudgedAfterCloseIsErrClosed(t *testing.T) {
	out, _ := manifestOutput(t.TempDir())
	task := out.Task("late")
	task.basis(freshness.Value("v", "x"))
	_ = out.close()
	if _, err := task.basisIsCurrent(context.Background()); !errors.Is(err, errClosed) {
		t.Fatalf("basisIsCurrent after Close = %v, want errors.Is ErrClosed", err)
	}
}
