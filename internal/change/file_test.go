package change

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func readFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(body)
}

func TestFileOutsideADefineCallbackReturnsErrNoTaskContext(t *testing.T) {
	if err := File(context.Background(), FileSpec{Path: "a.txt", Contents: []byte("x")}); err == nil {
		t.Fatal("File without a scope returned nil")
	}
}

func TestFileRequiresAPath(t *testing.T) {
	_, ctx := scopedRun(t, newFakeHost(t.TempDir()))

	if err := File(ctx, FileSpec{Contents: []byte("x")}); !errors.Is(err, ErrFileSpecMissingPath) {
		t.Fatalf("File without a path = %v, want ErrFileSpecMissingPath", err)
	}
}

func TestFileWritesAMissingFileRecordsTheEffectAndNarratesTheSteps(t *testing.T) {
	host := newFakeHost(t.TempDir())
	_, ctx := scopedRun(t, host)
	path := filepath.Join(t.TempDir(), "managed.txt")

	err := File(ctx, FileSpec{Path: path, Contents: []byte("desired"), Mode: 0o640})
	if err != nil {
		t.Fatalf("File = %v, want nil", err)
	}
	if got := readFile(t, path); got != "desired" {
		t.Errorf("contents = %q, want desired", got)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640", info.Mode().Perm())
	}
	if got := host.recorded(); len(got) != 1 || got[0].Verb() != "write" {
		t.Errorf("recorded %v, want one write entry", got)
	}
	want := []ObservationKind{BasisFingerprinted, OperationStarted, ResourceObserved, OperationFinished}
	if got := host.observations(); !slices.Equal(got, want) {
		t.Errorf("observed %v, want %v", got, want)
	}
	if len(host.appended) != 1 || len(host.attached) != 2 {
		t.Errorf("appended %d operations and attached %d verifications, want 1 and 2 (contents, permissions)", len(host.appended), len(host.attached))
	}
}

func TestFileAlreadyMatchingRecordsNothingAndKeepsTheFile(t *testing.T) {
	host := newFakeHost(t.TempDir())
	_, ctx := scopedRun(t, host)
	path := filepath.Join(t.TempDir(), "managed.txt")
	if err := os.WriteFile(path, []byte("desired"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(path)

	if err := File(ctx, FileSpec{Path: path, Contents: []byte("desired")}); err != nil {
		t.Fatalf("File = %v, want nil", err)
	}

	after, _ := os.Stat(path)
	if !os.SameFile(before, after) || !after.ModTime().Equal(before.ModTime()) {
		t.Error("an already-matching File rewrote the file")
	}
	if got := host.recorded(); len(got) != 0 {
		t.Errorf("recorded %v, want nothing", got)
	}
}

func TestFileDryRunPlansTheWriteAndTouchesNothing(t *testing.T) {
	host := newFakeHost(t.TempDir())
	host.applies = false
	_, ctx := scopedRun(t, host)
	path := filepath.Join(t.TempDir(), "managed.txt")

	if err := File(ctx, FileSpec{Path: path, Contents: []byte("desired")}); err != nil {
		t.Fatalf("File = %v, want nil", err)
	}

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dry run created %s: %v", path, err)
	}
	if got := host.recorded(); len(got) != 1 || host.planned != 1 || len(host.appended) != 0 {
		t.Errorf("recorded %d entries, planned %d, appended %d; want 1, 1, 0", len(got), host.planned, len(host.appended))
	}
}

func TestFileRefusesWhatItCannotOwn(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.txt")
	link := filepath.Join(dir, "link.txt")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	cases := map[string]struct {
		spec FileSpec
		want error
	}{
		"symlink":            {FileSpec{Path: link, Contents: []byte("x")}, ErrFilePathIsSymlink},
		"directory":          {FileSpec{Path: dir, Contents: []byte("x")}, ErrFilePathTypeMismatch},
		"missing, unmanaged": {FileSpec{Path: filepath.Join(dir, "absent"), Mode: 0o600}, ErrFileUnmanagedContentsMissing},
	}
	for name, tc := range cases {
		_, ctx := scopedRun(t, newFakeHost(t.TempDir()))
		if err := File(ctx, tc.spec); !errors.Is(err, tc.want) {
			t.Errorf("%s: File = %v, want %v", name, err, tc.want)
		}
	}
}

func TestTwoTasksClaimingOneOutputConflict(t *testing.T) {
	run, _ := scopedRun(t, newFakeHost(t.TempDir()))

	if err := run.claimOutput("t1", "/out/a"); err != nil {
		t.Fatalf("first claim = %v", err)
	}
	if err := run.claimOutput("t1", "/out/a"); err != nil {
		t.Errorf("the same Task claiming again = %v, want nil", err)
	}
	if err := run.claimOutput("t2", "/out/a"); !errors.Is(err, ErrFileConflictingProducer) {
		t.Errorf("a second Task claiming the path = %v, want ErrFileConflictingProducer", err)
	}
}

func TestFileInsideAClaimIsNestedAcquisition(t *testing.T) {
	run, ctx := scopedRun(t, newFakeHost(t.TempDir()))

	err := run.Hold(ctx, LogicalResource("file-test-outer"), ClaimWrite, func(held context.Context) error {
		return File(held, FileSpec{Path: filepath.Join(t.TempDir(), "a.txt"), Contents: []byte("x")})
	})

	if !errors.Is(err, ErrNestedResourceAcquisition) {
		t.Fatalf("File under a claim = %v, want ErrNestedResourceAcquisition", err)
	}
}
