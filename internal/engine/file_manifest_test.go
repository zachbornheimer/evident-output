package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/fingerprint"
	"github.com/zachbornheimer/evident-output/internal/manifest"
)

// runFileTask runs one Task named name whose Define calls evo.File(spec)
// once against out, waiting for it to settle. It returns the File call's
// own error (not the Task's) so tests can distinguish a File-level failure
// from Task bookkeeping.
func runFileTask(t *testing.T, out *Output, name string, spec FileSpec) error {
	t.Helper()
	var fileErr error
	task := out.Task(name)
	task.Define(func(ctx context.Context) error {
		fileErr = File(ctx, spec)
		return fileErr
	})
	_ = task.Wait()
	return fileErr
}

func statOrFatal(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	return info
}

// TestFileManifestFreshnessSkipsUnchangedOperation proves spec §11.4/§11.5:
// an unchanged File invocation across two Runs sharing one manifest Store
// performs no mutation and no write syscall on the second Run — observed
// here as an untouched mtime, the same evidence file_test.go's live-
// filesystem sibling uses.
func TestFileManifestFreshnessSkipsUnchangedOperation(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(t.TempDir(), "managed.txt")
	spec := FileSpec{Path: path, Contents: []byte("desired")}

	first := Init(Config{Isolated: true, StateDir: state})
	if err := runFileTask(t, first, "file", spec); err != nil {
		t.Fatalf("first run: %v", err)
	}
	_ = first.Close()
	before := statOrFatal(t, path)

	second := Init(Config{Isolated: true, StateDir: state})
	t.Cleanup(func() { _ = second.Close() })
	if err := runFileTask(t, second, "file", spec); err != nil {
		t.Fatalf("second run: %v", err)
	}
	after := statOrFatal(t, path)
	if !os.SameFile(before, after) || !after.ModTime().Equal(before.ModTime()) {
		t.Fatalf("current operation mutated the file: before=%v after=%v", before.ModTime(), after.ModTime())
	}
}

// TestFileManifestBasisDriftForcesReconciliation proves spec §11.4: a
// change in a Basis fingerprint invalidates the prior operation record even
// though Path/Contents/Mode are unchanged, so the second Run does not take
// the manifest-current fast path.
func TestFileManifestBasisDriftForcesReconciliation(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(t.TempDir(), "managed.txt")

	first := Init(Config{Isolated: true, StateDir: state})
	spec1 := FileSpec{Path: path, Contents: []byte("desired"), Basis: []fingerprint.Fingerprint{fingerprint.Value("input", 1)}}
	if err := runFileTask(t, first, "file", spec1); err != nil {
		t.Fatalf("first run: %v", err)
	}
	_ = first.Close()

	second := Init(Config{Isolated: true, StateDir: state})
	t.Cleanup(func() { _ = second.Close() })
	task := second.Task("file")
	store, err := second.manifestFor(context.Background())
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	second.mu.Lock()
	key, _, ok := second.taskManifestKeyLocked(task.id)
	second.mu.Unlock()
	if !ok {
		t.Fatal("expected a declared task")
	}
	priorBefore, hadPrior := store.Operation(key, 0)
	if !hadPrior {
		t.Fatal("expected a prior operation record from the first run")
	}

	spec2 := FileSpec{Path: path, Contents: []byte("desired"), Basis: []fingerprint.Fingerprint{fingerprint.Value("input", 2)}}
	current, prior, reason, checkErr := second.fileConsultManifest(context.Background(), task.id, spec2, second.resolveWorkspacePath(path))
	if checkErr != nil {
		t.Fatalf("consult manifest: %v", checkErr)
	}
	if current {
		t.Fatal("Basis drift must not report the operation as current")
	}
	if reason != freshnessReasonBasisDrift {
		t.Fatalf("freshness reason = %q, want %q (spec §38: Basis drift must be distinguishable)", reason, freshnessReasonBasisDrift)
	}
	if prior.DefinitionFingerprint != priorBefore.DefinitionFingerprint {
		t.Fatalf("prior record fingerprint mismatch: got %q want %q", prior.DefinitionFingerprint, priorBefore.DefinitionFingerprint)
	}
	task.Define(func(ctx context.Context) error { return nil })
	_ = task.Wait()
}

// TestFileManifestOutputDriftForcesReconciliation proves spec §11.5: a
// tracked output edited outside Evo between Runs invalidates the prior
// operation record (the on-disk digest no longer matches), so the next Run
// repairs it back to the desired Contents.
func TestFileManifestOutputDriftForcesReconciliation(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(t.TempDir(), "managed.txt")
	spec := FileSpec{Path: path, Contents: []byte("desired")}

	first := Init(Config{Isolated: true, StateDir: state})
	if err := runFileTask(t, first, "file", spec); err != nil {
		t.Fatalf("first run: %v", err)
	}
	_ = first.Close()

	if err := os.WriteFile(path, []byte("tampered"), 0o644); err != nil {
		t.Fatalf("simulate external edit: %v", err)
	}

	second := Init(Config{Isolated: true, StateDir: state})
	t.Cleanup(func() { _ = second.Close() })
	if err := runFileTask(t, second, "file", spec); err != nil {
		t.Fatalf("second run: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "desired" {
		t.Fatalf("output drift was not repaired: contents=%q err=%v", got, err)
	}
}

// TestFileManifestDryRunNeverCommitsAndRecordsPlannedEffect proves spec
// §8.2/§51: a dry run that would mutate a stale File records a planned
// Effect and performs no write, and never commits manifest state — a
// following non-dry-run Run still sees no prior record.
func TestFileManifestDryRunNeverCommitsAndRecordsPlannedEffect(t *testing.T) {
	state := t.TempDir()
	path := filepath.Join(t.TempDir(), "managed.txt")
	spec := FileSpec{Path: path, Contents: []byte("desired")}

	dry := Init(Config{Isolated: true, DryRun: true, StateDir: state})
	if err := runFileTask(t, dry, "file", spec); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	snap := dry.Snapshot()
	_ = dry.Close()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("dry-run File must not create the path, stat err=%v", err)
	}
	foundPlanned := false
	for _, p := range snap.Plans {
		for _, r := range p.Records {
			if r.Object == path {
				foundPlanned = true
			}
		}
	}
	if !foundPlanned {
		t.Fatalf("dry-run must record a planned Effect for %q, plans=%+v", path, snap.Plans)
	}

	applied := Init(Config{Isolated: true, StateDir: state})
	t.Cleanup(func() { _ = applied.Close() })
	store, err := applied.manifestFor(context.Background())
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	if _, ok := store.Operation("task:/file", 0); ok {
		t.Fatal("dry-run must not have committed manifest state")
	}
}

// TestFileManifestCommittedEffectRecordedOnMutation proves spec §27/§51: a
// non-dry-run File call that actually writes records a committed Effect in
// the Task's own Changes ledger.
func TestFileManifestCommittedEffectRecordedOnMutation(t *testing.T) {
	out := Init(Config{Isolated: true, StateDir: t.TempDir()})
	t.Cleanup(func() { _ = out.Close() })
	path := filepath.Join(t.TempDir(), "managed.txt")
	if err := runFileTask(t, out, "file", FileSpec{Path: path, Contents: []byte("desired")}); err != nil {
		t.Fatalf("run: %v", err)
	}
	snap := out.Snapshot()
	found := false
	for _, c := range snap.Changes {
		for _, r := range c.Records {
			if r.Object == path {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected a committed Effect for %q, changes=%+v", path, snap.Changes)
	}
}

// TestFileManifestConflictingProducersFailTask proves spec §8.3/§11.4: two
// Tasks in one Run both claiming the same canonical output path is a
// conflict, detected as soon as the second claim is observed.
func TestFileManifestConflictingProducersFailTask(t *testing.T) {
	out := Init(Config{Isolated: true, StateDir: t.TempDir()})
	t.Cleanup(func() { _ = out.Close() })
	path := filepath.Join(t.TempDir(), "shared.txt")

	if err := runFileTask(t, out, "first-producer", FileSpec{Path: path, Contents: []byte("a")}); err != nil {
		t.Fatalf("first producer: %v", err)
	}
	err := runFileTask(t, out, "second-producer", FileSpec{Path: path, Contents: []byte("b")})
	if err == nil {
		t.Fatal("second producer claiming the same path must fail")
	}
}

// TestFileManifestPartialFailurePreservesEarlierCommits proves spec
// §8.2/§11.3: a Task whose operation fails commits no manifest record for
// itself, while an earlier Task in the same Run that already succeeded
// keeps its committed record.
func TestFileManifestPartialFailurePreservesEarlierCommits(t *testing.T) {
	state := t.TempDir()
	okPath := filepath.Join(t.TempDir(), "ok.txt")
	dirAsFile := filepath.Join(t.TempDir(), "is-a-dir")
	if err := os.Mkdir(dirAsFile, 0o755); err != nil {
		t.Fatalf("setup dir: %v", err)
	}

	out := Init(Config{Isolated: true, StateDir: state})
	t.Cleanup(func() { _ = out.Close() })
	if err := runFileTask(t, out, "ok-task", FileSpec{Path: okPath, Contents: []byte("fine")}); err != nil {
		t.Fatalf("ok task: %v", err)
	}
	if err := runFileTask(t, out, "bad-task", FileSpec{Path: dirAsFile, Contents: []byte("x")}); err == nil {
		t.Fatal("File targeting a directory must fail")
	}

	store, err := out.manifestFor(context.Background())
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	if _, ok := store.Operation("task:/ok-task", 0); !ok {
		t.Fatal("earlier successful Task's commit must survive a later Task's failure")
	}
	if _, ok := store.Operation("task:/bad-task", 0); ok {
		t.Fatal("a failed Task must not commit a manifest record")
	}
}

// TestManifestCorruptFileIsSafeMissWithWarning proves spec §11.3: a
// present-but-untrustworthy manifest file is treated as an empty document
// (a safe cache miss), never as Evidence, and surfaces a run-scoped warning
// Fact rather than failing the Run.
func TestManifestCorruptFileIsSafeMissWithWarning(t *testing.T) {
	state := t.TempDir()
	if err := os.WriteFile(filepath.Join(state, "manifest-v1.json"), []byte("not json"), 0o600); err != nil {
		t.Fatalf("seed corrupt manifest: %v", err)
	}
	path := filepath.Join(t.TempDir(), "managed.txt")

	out := Init(Config{Isolated: true, StateDir: state})
	t.Cleanup(func() { _ = out.Close() })
	if err := runFileTask(t, out, "file", FileSpec{Path: path, Contents: []byte("desired")}); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "desired" {
		t.Fatalf("corrupt manifest must not block reconciliation: contents=%q err=%v", got, err)
	}
	snap := out.Snapshot()
	warned := false
	for _, f := range snap.Facts {
		if f.Name == "manifest" {
			warned = true
		}
	}
	if !warned {
		t.Fatal("a corrupt manifest must surface a run-scoped warning Fact")
	}
}

// TestFileManifestAppBasisDriftForcesReconciliation proves spec §11.2: an
// explicit evo.App() Basis entry ties an operation's freshness to the
// running application's own fingerprint, without requiring a manifest
// ApplicationRecord comparison — a changed App() digest is just an ordinary
// Basis drift.
func TestFileManifestAppBasisDriftForcesReconciliation(t *testing.T) {
	basisA := []manifest.BasisRecord{{Kind: "app", Key: "application", Digest: "sha-a"}}
	basisB := []manifest.BasisRecord{{Kind: "app", Key: "application", Digest: "sha-b"}}
	defA := fileDefinitionFingerprint("/x", true, []byte("same"), 0, basisA)
	defB := fileDefinitionFingerprint("/x", true, []byte("same"), 0, basisB)
	if defA == defB {
		t.Fatal("a changed App() Basis digest must change the operation definition fingerprint")
	}
}

// TestFileManifestPostDefineEvidenceSourcesFromOperations proves spec §9.2:
// a Task with no explicit Verify but at least one tracked evo.File
// operation derives after-Define Evidence from that operation, source
// "operations" — never left unevaluated the way an opaque callback with no
// modeled proof would be.
func TestFileManifestPostDefineEvidenceSourcesFromOperations(t *testing.T) {
	out := Init(Config{Isolated: true, StateDir: t.TempDir()})
	t.Cleanup(func() { _ = out.Close() })
	path := filepath.Join(t.TempDir(), "managed.txt")
	task := out.Task("file")
	task.Define(func(ctx context.Context) error {
		return File(ctx, FileSpec{Path: path, Contents: []byte("desired")})
	})
	_ = task.Wait()
	snap := task.Snapshot()
	if !snap.Evidence.After.Evaluated || !snap.Evidence.After.Satisfied {
		t.Fatalf("expected evaluated+satisfied after-Define Evidence, got %+v", snap.Evidence.After)
	}
	if snap.Evidence.After.Source != "operations" {
		t.Fatalf("Evidence.After.Source = %q, want %q", snap.Evidence.After.Source, "operations")
	}
}

// TestFileBasisRecordsRejectsDuplicateKind proves spec §11.1: duplicate
// (Kind, Key) Basis pairs within one operation are a programmer error.
func TestFileBasisRecordsRejectsDuplicateKind(t *testing.T) {
	_, err := fileBasisRecords(context.Background(), []fingerprint.Fingerprint{
		fingerprint.Value("same", 1),
		fingerprint.Value("same", 2),
	})
	if err == nil {
		t.Fatal("duplicate (kind,key) Basis entries must be rejected")
	}
}
