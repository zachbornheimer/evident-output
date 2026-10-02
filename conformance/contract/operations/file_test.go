//go:build unix

package operations_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/conformance/contract/harness"
)

// applyFile runs one File spec inside a fresh run and returns the run text
// and the File error.
func applyFile(t *testing.T, spec evo.File, mutate ...func(*evo.Config)) (string, error) {
	t.Helper()
	state := func(c *evo.Config) { c.StateDir = t.TempDir() }
	out, buf := harness.New(t, append([]func(*evo.Config){state}, mutate...)...)
	var fileErr error
	task := out.Task("write file")
	task.Define(func(ctx context.Context) error {
		fileErr = spec.Write(ctx)
		return fileErr
	})
	_ = task.Wait()
	text := harness.Text(out, buf)
	_ = out.Close()
	return text, fileErr
}

func TestC01_003_FileEstablishesDesiredStateAndIsQuietWhenSatisfied(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.json")
	first, err := applyFile(t, evo.File{Path: path, Content: evo.Bytes("{}")})
	if err != nil || !strings.Contains(first, "[changed]") {
		t.Fatalf("first run err=%v:\n%s", err, first)
	}
	if got, _ := os.ReadFile(path); string(got) != "{}" {
		t.Fatalf("contents = %q", got)
	}
	second, err := applyFile(t, evo.File{Path: path, Content: evo.Bytes("{}")})
	if err != nil || strings.Contains(second, "[changed]") {
		t.Fatalf("satisfied run err=%v:\n%s", err, second)
	}
}

func TestC04_001_UnmanagedContentsCannotCreateAMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	_, err := applyFile(t, evo.File{Path: path})
	if !errors.Is(err, evo.ErrContentMissing) {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(path); statErr == nil {
		t.Fatal("File created a file with unmanaged contents")
	}
}

func TestC04_002_ModeZeroKeepsExistingPermissionsAndNewFilesUseUmask(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing")
	if err := os.WriteFile(existing, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(existing, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := applyFile(t, evo.File{Path: existing, Content: evo.Bytes("new")}); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(existing); info.Mode().Perm() != 0o600 {
		t.Fatalf("existing mode = %v, want 0600", info.Mode().Perm())
	}
	umask := syscall.Umask(0)
	syscall.Umask(umask)
	fresh := filepath.Join(dir, "fresh")
	if _, err := applyFile(t, evo.File{Path: fresh, Content: evo.Bytes("x")}); err != nil {
		t.Fatal(err)
	}
	want := os.FileMode(0o666 &^ umask)
	if info, _ := os.Stat(fresh); info.Mode().Perm() != want {
		t.Fatalf("new mode = %v, want %v", info.Mode().Perm(), want)
	}
}

func TestC04_003_FileMutatesOnlyManagedAttributes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keep")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := applyFile(t, evo.File{Path: path, Content: evo.Bytes("original"), Mode: 0o600}); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	got, _ := os.ReadFile(path)
	if string(got) != "original" || info.Mode().Perm() != 0o600 {
		t.Fatalf("contents %q mode %v", got, info.Mode().Perm())
	}
}

func TestC04_004_FileRefusesSymlinkAndWrongType(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("t"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := applyFile(t, evo.File{Path: link, Content: evo.Bytes("x")}); !errors.Is(err, evo.ErrFilePathIsSymlink) {
		t.Fatalf("symlink err = %v", err)
	}
	if _, err := applyFile(t, evo.File{Path: dir, Content: evo.Bytes("x")}); !errors.Is(err, evo.ErrFilePathTypeMismatch) {
		t.Fatalf("directory err = %v", err)
	}
}

func TestC04_005_OperationsRequireTheDefineContext(t *testing.T) {
	bare := context.Background()
	spec := evo.EffectSpec{Verb: evo.EffectDelete, Object: "branch", Quantity: 1}
	_, execErr := evo.Exec{Path: "true"}.Run(bare)
	patchErr := evo.Patch(bare, []byte(""))
	for name, err := range map[string]error{
		"File":   evo.File{Path: "x", Content: evo.Bytes("x")}.Write(bare),
		"Tree":   evo.Tree{Path: "x"}.Write(bare),
		"Effect": evo.Effect(bare, spec, func(context.Context) error { return nil }),
		"Exec":   execErr,
		"Patch":  patchErr,
	} {
		if !errors.Is(err, evo.ErrNoTaskContext) {
			t.Errorf("%s err = %v, want ErrNoTaskContext", name, err)
		}
	}
}

func TestC04_006_FileEmitsPlannedThenChangedRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.json")
	planned, _ := applyFile(t, evo.File{Path: path, Content: evo.Bytes("{}")}, func(c *evo.Config) { c.DryRun = true })
	if !strings.Contains(planned, "[planned]") || strings.Contains(planned, "[changed]") {
		t.Fatalf("dry run:\n%s", planned)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("dry run wrote the file")
	}
	applied, _ := applyFile(t, evo.File{Path: path, Content: evo.Bytes("{}")})
	if !strings.Contains(applied, "[changed]") {
		t.Fatalf("apply:\n%s", applied)
	}
}

func TestC05_001_SuccessStaysCompactWithoutPerAttributeRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.plist")
	text, err := applyFile(t, evo.File{Path: path, Content: evo.Bytes("x"), Mode: 0o644})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(text, "contents") || strings.Contains(text, "permissions") || !strings.Contains(text, "✓ write file") {
		t.Fatalf("success output:\n%s", text)
	}
}
