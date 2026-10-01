package basis_test

// Adversarial hardening tests for Task Basis freshness. Each test is named
// for the weakness it guards; sources are in
// docs/zys-1382/adversarial-research.md.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// advBasis declares a Task's Basis; it is how a test names inputs of the
// sealed Basis parameter type.
type advBasis func(*evo.TaskHandle) *evo.TaskHandle

// advRunner runs one named Task per call against a shared StateDir, so
// successive calls model successive Runs of the same program.
type advRunner struct {
	tb       testing.TB
	stateDir string
	runs     int
}

func newAdvRunner(tb testing.TB) *advRunner {
	return &advRunner{tb: tb, stateDir: tb.TempDir()}
}

// run executes one Run and reports whether the Task's callback executed.
func (r *advRunner) run(name string, basis advBasis, fn func(context.Context) error) (bool, error) {
	r.tb.Helper()
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: r.stateDir})
	defer func() { _ = out.Close() }()
	ran := false
	err := basis(out.Task(name)).Define(func(ctx context.Context) error {
		ran = true
		if fn != nil {
			return fn(ctx)
		}
		return nil
	}).Wait()
	_ = out.Finish()
	return ran, err
}

// expect asserts whether the callback ran on this Run.
func (r *advRunner) expect(name string, basis advBasis, wantRan bool, why string) {
	r.tb.Helper()
	r.runs++
	ran, err := r.run(name, basis, nil)
	if err != nil {
		r.tb.Fatalf("run %d (%s): %v", r.runs, why, err)
	}
	if ran != wantRan {
		r.tb.Fatalf("run %d (%s): callback ran = %v, want %v", r.runs, why, ran, wantRan)
	}
}

func advFileBasis(path string) advBasis {
	return func(t *evo.TaskHandle) *evo.TaskHandle { return t.Basis(evo.File{Path: path}) }
}

func advTreeBasis(path string) advBasis {
	return func(t *evo.TaskHandle) *evo.TaskHandle { return t.Basis(evo.Tree{Path: path}) }
}

func advWrite(tb testing.TB, path, body string) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		tb.Fatal(err)
	}
}

// Racy git: same size, mtime restored. Only content identity catches it.
func TestAdversarial_ContentChangeWithRestoredStampsReruns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "package-lock.json")
	advWrite(t, path, "aaaa")
	r := newAdvRunner(t)
	r.expect("install", advFileBasis(path), true, "first run")
	r.expect("install", advFileBasis(path), false, "unchanged")

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	advWrite(t, path, "bbbb")
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	r.expect("install", advFileBasis(path), true, "same-size edit with restored mtime")
}

// Structural: a touch changes stamps but not identity, so nothing reruns.
func TestAdversarial_TouchWithoutContentChangeDoesNotRerun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "package.json")
	advWrite(t, path, "{}")
	r := newAdvRunner(t)
	r.expect("install", advFileBasis(path), true, "first run")
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	r.expect("install", advFileBasis(path), false, "touch and chmod only")
}

// Bazel checks inputs after execution: an input edited while the Task ran
// must not be recorded as current.
func TestAdversarial_EditDuringRunForcesRerun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "package.json")
	advWrite(t, path, "v1")
	r := newAdvRunner(t)
	ran, err := r.run("install", advFileBasis(path), func(context.Context) error {
		advWrite(t, path, "v2 written mid-run")
		return nil
	})
	if err != nil || !ran {
		t.Fatalf("first run: ran=%v err=%v", ran, err)
	}
	r.expect("install", advFileBasis(path), true, "input changed while the previous run executed")
	r.expect("install", advFileBasis(path), false, "stable since")
}

func TestAdversarial_MissingBasisAppearingReruns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pnpm-lock.yaml")
	r := newAdvRunner(t)
	r.expect("install", advFileBasis(path), true, "first run, input missing")
	r.expect("install", advFileBasis(path), false, "still missing")
	advWrite(t, path, "lockfileVersion: 9")
	r.expect("install", advFileBasis(path), true, "input appeared")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	r.expect("install", advFileBasis(path), true, "input disappeared")
}

func TestAdversarial_TreeBasisNewFileReruns(t *testing.T) {
	root := filepath.Join(t.TempDir(), "src")
	advWrite(t, filepath.Join(root, "a.ts"), "a")
	r := newAdvRunner(t)
	r.expect("build", advTreeBasis(root), true, "first run")
	r.expect("build", advTreeBasis(root), false, "unchanged")
	advWrite(t, filepath.Join(root, "deep", "nested", "b.ts"), "b")
	r.expect("build", advTreeBasis(root), true, "a new file appeared deep in the tree")
}

func TestAdversarial_TreeBasisRenameReruns(t *testing.T) {
	root := filepath.Join(t.TempDir(), "src")
	advWrite(t, filepath.Join(root, "a.ts"), "same")
	r := newAdvRunner(t)
	r.expect("build", advTreeBasis(root), true, "first run")
	if err := os.Rename(filepath.Join(root, "a.ts"), filepath.Join(root, "b.ts")); err != nil {
		t.Fatal(err)
	}
	r.expect("build", advTreeBasis(root), true, "same content under a new name")
}

// Retargeting a symlink changes identity whether Basis observes the link
// itself or what it points to: both targets exist with different bytes.
func TestAdversarial_SymlinkTargetChangeReruns(t *testing.T) {
	dir := t.TempDir()
	advWrite(t, filepath.Join(dir, "v1"), "one")
	advWrite(t, filepath.Join(dir, "v2"), "two")
	link := filepath.Join(dir, "current")
	if err := os.Symlink("v1", link); err != nil {
		t.Fatal(err)
	}
	r := newAdvRunner(t)
	r.expect("use", advFileBasis(link), true, "first run")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("v2", link); err != nil {
		t.Fatal(err)
	}
	r.expect("use", advFileBasis(link), true, "symlink retargeted")
}

func TestAdversarial_FailedRunIsNeverCurrent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "package.json")
	advWrite(t, path, "{}")
	r := newAdvRunner(t)
	errBoom := errors.New("boom")
	ran, err := r.run("install", advFileBasis(path), func(context.Context) error { return errBoom })
	if !ran || !errors.Is(err, errBoom) {
		t.Fatalf("failing run: ran=%v err=%v", ran, err)
	}
	r.expect("install", advFileBasis(path), true, "previous run failed")
}

// Freshness keys include Task identity: one Task's success never makes a
// different Task with the same Basis current.
func TestAdversarial_FreshnessIsPerTask(t *testing.T) {
	path := filepath.Join(t.TempDir(), "package.json")
	advWrite(t, path, "{}")
	r := newAdvRunner(t)
	r.expect("install", advFileBasis(path), true, "first task")
	r.expect("audit", advFileBasis(path), true, "different task, same Basis")
	r.expect("audit", advFileBasis(path), false, "second task now current")
}

// An unreadable input is an observation failure, never "unchanged".
func TestAdversarial_UnreadableBasisNeverCurrent(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads mode-000 files")
	}
	path := filepath.Join(t.TempDir(), "package.json")
	advWrite(t, path, "{}")
	r := newAdvRunner(t)
	r.expect("install", advFileBasis(path), true, "first run")
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	ran, err := r.run("install", advFileBasis(path), nil)
	if !ran && err == nil {
		t.Fatal("an unreadable Basis input was treated as unchanged")
	}
}

// Basis order is not identity: the same inputs listed in another order
// leave the Task current.
func TestAdversarial_BasisOrderIsNotIdentity(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "package.json"), filepath.Join(dir, "package-lock.json")
	advWrite(t, a, "{}")
	advWrite(t, b, "{}")
	ab := func(t *evo.TaskHandle) *evo.TaskHandle { return t.Basis(evo.File{Path: a}, evo.File{Path: b}) }
	ba := func(t *evo.TaskHandle) *evo.TaskHandle { return t.Basis(evo.File{Path: b}, evo.File{Path: a}) }
	r := newAdvRunner(t)
	r.expect("install", ab, true, "first run")
	r.expect("install", ba, false, "same inputs, reordered")
}

// Two files with identical bytes are still two identities: swapping which
// path a Basis names is a change even though the digest set is equal.
func TestAdversarial_PathIsPartOfFileIdentity(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.json"), filepath.Join(dir, "b.json")
	advWrite(t, a, "{}")
	advWrite(t, b, "{}")
	r := newAdvRunner(t)
	r.expect("install", advFileBasis(a), true, "first run")
	r.expect("install", advFileBasis(b), true, "a different path with identical bytes")
}

// A Value's name is part of its identity: equal values under different
// names never collide.
func TestAdversarial_ValueNameIsPartOfIdentity(t *testing.T) {
	r := newAdvRunner(t)
	with := func(name string) advBasis {
		return func(t *evo.TaskHandle) *evo.TaskHandle { return t.Basis(evo.Value(name, "22.1.0")) }
	}
	r.expect("install", with("node"), true, "first run")
	r.expect("install", with("bun"), true, "same value under another name")
}

// Cardinality swap: a File Basis whose path becomes a directory, or a Tree
// Basis whose path becomes a regular file, is never current.
func TestAdversarial_CardinalitySwapIsNeverCurrent(t *testing.T) {
	t.Run("file becomes a directory", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config")
		advWrite(t, path, "x")
		r := newAdvRunner(t)
		r.expect("use", advFileBasis(path), true, "first run")
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		advWrite(t, filepath.Join(path, "x"), "x")
		ran, err := r.run("use", advFileBasis(path), nil)
		if !ran && err == nil {
			t.Fatal("a File Basis whose path became a directory was treated as unchanged")
		}
	})
	t.Run("tree becomes a file", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "src")
		advWrite(t, filepath.Join(root, "x"), "x")
		r := newAdvRunner(t)
		r.expect("build", advTreeBasis(root), true, "first run")
		if err := os.RemoveAll(root); err != nil {
			t.Fatal(err)
		}
		advWrite(t, root, "x")
		ran, err := r.run("build", advTreeBasis(root), nil)
		if !ran && err == nil {
			t.Fatal("a Tree Basis whose path became a regular file was treated as unchanged")
		}
	})
}

// Moving bytes between two files of one Tree keeps the digest multiset but
// changes structure; the Tree identity must notice.
func TestAdversarial_TreeBasisContentSwapReruns(t *testing.T) {
	root := filepath.Join(t.TempDir(), "src")
	advWrite(t, filepath.Join(root, "a.ts"), "alpha")
	advWrite(t, filepath.Join(root, "b.ts"), "beta")
	r := newAdvRunner(t)
	r.expect("build", advTreeBasis(root), true, "first run")
	advWrite(t, filepath.Join(root, "a.ts"), "beta")
	advWrite(t, filepath.Join(root, "b.ts"), "alpha")
	r.expect("build", advTreeBasis(root), true, "contents swapped between two paths")
}

// Basis is not a resource claim: a Task with a File Basis can itself Write
// other files from its callback without ErrNestedResourceAcquisition, and
// that write does not disturb its own freshness.
func TestAdversarial_BasisTaskCanWriteFromItsCallback(t *testing.T) {
	dir := t.TempDir()
	input, output := filepath.Join(dir, "package.json"), filepath.Join(dir, "node_modules", ".stamp")
	advWrite(t, input, "{}")
	r := newAdvRunner(t)
	write := func(ctx context.Context) error {
		return evo.File{Path: output, Content: evo.Bytes("installed")}.Write(ctx)
	}
	ran, err := r.run("install", advFileBasis(input), write)
	if !ran || err != nil {
		t.Fatalf("Basis Task writing another file: ran=%v err=%v; Basis must hold no claim", ran, err)
	}
	r.expect("install", advFileBasis(input), false, "the Task's own output is not its Basis")
}

// Basis chains like After: a second call adds inputs, never replaces the
// first, so a change to either input reruns the Task.
func TestAdversarial_ChainedBasisCallsAccumulate(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "package.json"), filepath.Join(dir, "package-lock.json")
	advWrite(t, a, "{}")
	advWrite(t, b, "{}")
	chained := func(t *evo.TaskHandle) *evo.TaskHandle {
		return t.Basis(evo.File{Path: a}).Basis(evo.File{Path: b})
	}
	r := newAdvRunner(t)
	r.expect("install", chained, true, "first run")
	r.expect("install", chained, false, "unchanged")
	advWrite(t, a, `{"a":2}`)
	r.expect("install", chained, true, "the first chained input changed")
	advWrite(t, b, `{"b":2}`)
	r.expect("install", chained, true, "the second chained input changed")
}

// A Basis call after Define is misuse and ignored: the late input never
// joins the Task's freshness, so changing it leaves the Task current.
func TestAdversarial_BasisAfterDefineIsIgnored(t *testing.T) {
	dir := t.TempDir()
	declared, late := filepath.Join(dir, "package.json"), filepath.Join(dir, "late.json")
	advWrite(t, declared, "{}")
	advWrite(t, late, "{}")
	state := t.TempDir()
	run := func() bool {
		t.Helper()
		out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: state})
		defer func() { _ = out.Close() }()
		var ran atomic.Bool
		task := out.Task("install").Basis(evo.File{Path: declared}).Define(func(context.Context) error {
			ran.Store(true)
			return nil
		})
		task.Basis(evo.File{Path: late})
		if err := task.Wait(); err != nil {
			t.Fatalf("task: %v", err)
		}
		_ = out.Finish()
		if !errors.Is(out.Err(), evo.ErrBasisAfterDefine) {
			t.Fatalf("Err() = %v, want ErrBasisAfterDefine", out.Err())
		}
		return ran.Load()
	}
	if !run() {
		t.Fatalf("first run never ran")
	}
	if run() {
		t.Fatalf("unchanged declared Basis reran the Task")
	}
	advWrite(t, late, `{"changed":true}`)
	if run() {
		t.Fatalf("a Basis input added after Define changed freshness; the late call must be ignored")
	}
}

func BenchmarkBasisCurrentCheck_Tree2000Files(b *testing.B) {
	root := filepath.Join(b.TempDir(), "node_modules")
	for i := range 2000 {
		advWrite(b, filepath.Join(root, fmt.Sprintf("pkg%03d", i%100), fmt.Sprintf("f%04d.js", i)), fmt.Sprint(i))
	}
	r := newAdvRunner(b)
	if _, err := r.run("install", advTreeBasis(root), nil); err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		ran, err := r.run("install", advTreeBasis(root), nil)
		if err != nil {
			b.Fatal(err)
		}
		if ran {
			b.Fatal("an unchanged tree Basis reran the Task")
		}
	}
}
