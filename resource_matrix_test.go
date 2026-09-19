package evo_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/apisurface"
	"github.com/zachbornheimer/evident-output/testkit"
)

func isolatedOutput(t *testing.T, fileFS evo.FileFS) *evo.Output {
	t.Helper()
	cfg := evo.Config{
		Isolated:       true,
		StateDir:       t.TempDir(),
		Plain:          true,
		Color:          evo.ColorNever,
		Stdout:         io.Discard,
		Stderr:         io.Discard,
		MaxConcurrency: 4,
	}
	if fileFS != nil {
		cfg.FileFS = fileFS
	}
	out := evo.Init(cfg)
	t.Cleanup(func() { _ = out.Close() })
	return out
}

func waitPhase(t *testing.T, task *evo.TaskHandle, substr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(task.Snapshot().Phase, substr) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("phase %q never contained %q", task.Snapshot().Phase, substr)
}

type writeGateFS struct {
	inner       *testkit.FileFS
	started     chan struct{}
	proceed     chan struct{}
	blockedPath string
	inFlight    atomic.Int32
	overlapped  atomic.Int32
}

func newWriteGateFS() *writeGateFS {
	return &writeGateFS{inner: testkit.NewFileFS(), started: make(chan struct{}), proceed: make(chan struct{})}
}

func (f *writeGateFS) Lstat(path string) (fs.FileInfo, error) { return f.inner.Lstat(path) }
func (f *writeGateFS) ReadFile(path string) ([]byte, error)   { return f.inner.ReadFile(path) }
func (f *writeGateFS) Chmod(path string, mode fs.FileMode) error {
	return f.inner.Chmod(path, mode)
}
func (f *writeGateFS) WriteAtomic(path string, contents []byte, mode fs.FileMode) error {
	if f.blockedPath != "" && path != f.blockedPath {
		return f.inner.WriteAtomic(path, contents, mode)
	}
	n := f.inFlight.Add(1)
	if n > 1 {
		f.overlapped.Add(1)
	}
	defer f.inFlight.Add(-1)
	select {
	case <-f.started:
	default:
		close(f.started)
	}
	<-f.proceed
	return f.inner.WriteAtomic(path, contents, mode)
}

type overlapReadFS struct {
	inner    *testkit.FileFS
	inFlight atomic.Int32
	max      atomic.Int32
	entered  chan struct{}
	release  chan struct{}
}

func newOverlapReadFS() *overlapReadFS {
	return &overlapReadFS{inner: testkit.NewFileFS(), entered: make(chan struct{}, 8), release: make(chan struct{})}
}

func (f *overlapReadFS) Lstat(path string) (fs.FileInfo, error) { return f.inner.Lstat(path) }
func (f *overlapReadFS) WriteAtomic(path string, contents []byte, mode fs.FileMode) error {
	return f.inner.WriteAtomic(path, contents, mode)
}
func (f *overlapReadFS) Chmod(path string, mode fs.FileMode) error {
	return f.inner.Chmod(path, mode)
}
func (f *overlapReadFS) ReadFile(path string) ([]byte, error) {
	n := f.inFlight.Add(1)
	defer f.inFlight.Add(-1)
	for {
		old := f.max.Load()
		if n <= old || f.max.CompareAndSwap(old, n) {
			break
		}
	}
	f.entered <- struct{}{}
	<-f.release
	return f.inner.ReadFile(path)
}

type reenterFS struct {
	inner  *testkit.FileFS
	ctx    context.Context
	nested string
	err    error
}

func (f *reenterFS) Lstat(path string) (fs.FileInfo, error) { return f.inner.Lstat(path) }
func (f *reenterFS) ReadFile(path string) ([]byte, error)   { return f.inner.ReadFile(path) }
func (f *reenterFS) Chmod(path string, mode fs.FileMode) error {
	return f.inner.Chmod(path, mode)
}
func (f *reenterFS) WriteAtomic(path string, contents []byte, mode fs.FileMode) error {
	f.err = evo.File(f.ctx, evo.FileSpec{Path: f.nested, Contents: []byte("nested")})
	return f.inner.WriteAtomic(path, contents, mode)
}

type panicFS struct{ inner *testkit.FileFS }

func (f *panicFS) Lstat(path string) (fs.FileInfo, error) { return f.inner.Lstat(path) }
func (f *panicFS) ReadFile(path string) ([]byte, error)   { return f.inner.ReadFile(path) }
func (f *panicFS) Chmod(path string, mode fs.FileMode) error {
	return f.inner.Chmod(path, mode)
}
func (f *panicFS) WriteAtomic(path string, contents []byte, mode fs.FileMode) error {
	panic("filefs boom")
}

func greetingDiff() []byte {
	return []byte("" +
		"--- a/hello.txt\n" +
		"+++ b/hello.txt\n" +
		"@@ -1,2 +1,2 @@\n" +
		" hello\n" +
		"-world\n" +
		"+there\n")
}

func TestResourceMatrix_ReadReadOverlap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fsys := newOverlapReadFS()
	out := isolatedOutput(t, fsys)
	g := out.Group("reads")
	a := g.Task("a")
	b := g.Task("b")
	run := func(ctx context.Context) error {
		_, err := evo.Patch(ctx, evo.PatchSpec{Diff: greetingDiff(), Dir: dir})
		return err
	}
	a.Define(run)
	b.Define(run)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && len(fsys.entered) < 2 {
		time.Sleep(5 * time.Millisecond)
	}
	if len(fsys.entered) < 2 {
		t.Fatal("read holds did not overlap")
	}
	close(fsys.release)
	if err := a.Wait(); err != nil {
		t.Fatalf("a: %v", err)
	}
	if err := b.Wait(); err != nil {
		t.Fatalf("b: %v", err)
	}
	if fsys.max.Load() < 2 {
		t.Fatalf("max in-flight reads = %d, want >= 2", fsys.max.Load())
	}
}

func TestResourceMatrix_ReadWriteExclusion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	if err := os.WriteFile(path, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gate := newWriteGateFS()
	out := isolatedOutput(t, gate)
	g := out.Group("rw")
	writer := g.Task("writer")
	reader := g.Task("reader")
	writer.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: []byte("held\n")})
	})
	select {
	case <-gate.started:
	case <-time.After(3 * time.Second):
		t.Fatal("writer never started")
	}
	reader.Define(func(ctx context.Context) error {
		_, err := evo.Patch(ctx, evo.PatchSpec{Diff: greetingDiff(), Dir: dir})
		return err
	})
	waitPhase(t, reader, "waiting")
	close(gate.proceed)
	if err := writer.Wait(); err != nil {
		t.Fatalf("writer: %v", err)
	}
	if err := reader.Wait(); err == nil {
		t.Fatal("reader should not have observed the source mid-write")
	}
}

func TestResourceMatrix_WriteWriteExclusion(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	gate := newWriteGateFS()
	out := isolatedOutput(t, gate)
	g := out.Group("ww")
	a := g.Task("a")
	b := g.Task("b")
	a.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: []byte("a")})
	})
	select {
	case <-gate.started:
	case <-time.After(3 * time.Second):
		t.Fatal("first writer never started")
	}
	b.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: []byte("b")})
	})
	waitPhase(t, b, "waiting")
	if gate.inFlight.Load() != 1 {
		t.Fatalf("in-flight writes = %d, want 1", gate.inFlight.Load())
	}
	close(gate.proceed)
	errA := a.Wait()
	errB := b.Wait()
	if errA != nil {
		t.Fatalf("first writer: %v", errA)
	}
	if errB == nil {
		t.Fatal("second writer must fail as a conflicting producer after waiting")
	}
	if gate.overlapped.Load() != 0 {
		t.Fatal("write holds overlapped")
	}
}

func TestResourceMatrix_CanonicalPathIdentity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "n.txt")
	gate := newWriteGateFS()
	out := isolatedOutput(t, gate)
	g := out.Group("canon")
	a := g.Task("abs")
	b := g.Task("clean")
	a.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: []byte("a")})
	})
	select {
	case <-gate.started:
	case <-time.After(3 * time.Second):
		t.Fatal("abs writer never started")
	}
	b.Define(func(ctx context.Context) error {
		cleaned := filepath.Clean(path + "/../" + filepath.Base(path))
		return evo.File(ctx, evo.FileSpec{Path: cleaned, Contents: []byte("b")})
	})
	waitPhase(t, b, "waiting")
	close(gate.proceed)
	_ = a.Wait()
	_ = b.Wait()
}

func TestResourceMatrix_SymlinkAliasSafety(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(real, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "hello.txt")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	gate := newWriteGateFS()
	out := isolatedOutput(t, gate)
	g := out.Group("alias")
	writer := g.Task("writer")
	reader := g.Task("reader")
	writer.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: real, Contents: []byte("held\n")})
	})
	select {
	case <-gate.started:
	case <-time.After(3 * time.Second):
		t.Fatal("writer never started")
	}
	reader.Define(func(ctx context.Context) error {
		_, err := evo.Patch(ctx, evo.PatchSpec{Diff: greetingDiff(), Dir: dir})
		return err
	})
	waitPhase(t, reader, "waiting")
	close(gate.proceed)
	if err := writer.Wait(); err != nil {
		t.Fatalf("writer: %v", err)
	}
	if err := reader.Wait(); err == nil {
		t.Fatal("reader should not have observed the source mid-write")
	}
}

func TestResourceMatrix_CancellationReleases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	other := filepath.Join(dir, "other.txt")
	gate := newWriteGateFS()
	gate.blockedPath = path
	out := isolatedOutput(t, gate)
	g := out.Group("cancel")
	holder := g.Task("holder")
	waiter := g.Task("waiter")
	unrelated := g.Task("unrelated")
	ready := make(chan context.CancelFunc, 1)
	holder.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: []byte("a")})
	})
	select {
	case <-gate.started:
	case <-time.After(3 * time.Second):
		t.Fatal("holder never started")
	}
	waiter.Define(func(ctx context.Context) error {
		waitCtx, cancel := context.WithCancel(ctx)
		ready <- cancel
		return evo.File(waitCtx, evo.FileSpec{Path: path, Contents: []byte("b")})
	})
	var cancel context.CancelFunc
	select {
	case cancel = <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("waiter never started")
	}
	waitPhase(t, waiter, "waiting")
	cancel()
	errWait := waiter.Wait()
	if !errors.Is(errWait, context.Canceled) {
		t.Fatalf("waiter=%v, want context.Canceled", errWait)
	}
	unrelated.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: other, Contents: []byte("ok")})
	})
	if err := unrelated.Wait(); err != nil {
		t.Fatalf("unrelated resource blocked after cancel: %v", err)
	}
	close(gate.proceed)
	if err := holder.Wait(); err != nil {
		t.Fatalf("holder: %v", err)
	}
}

func TestResourceMatrix_ErrorReleases(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.txt")
	next := filepath.Join(dir, "next.txt")
	out := isolatedOutput(t, nil)
	task := out.Task("err")
	task.Define(func(ctx context.Context) error {
		err := evo.File(ctx, evo.FileSpec{Path: missing})
		if !errors.Is(err, evo.ErrFileUnmanagedContentsMissing) {
			return err
		}
		return evo.File(ctx, evo.FileSpec{Path: next, Contents: []byte("ok")})
	})
	if err := task.Wait(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(next)
	if err != nil || string(got) != "ok" {
		t.Fatalf("got %q err=%v", got, err)
	}
}

func TestResourceMatrix_PanicCannotLeak(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	out := isolatedOutput(t, &panicFS{inner: testkit.NewFileFS()})
	panicked := out.Task("panic")
	panicked.Define(func(ctx context.Context) error {
		defer func() { _ = recover() }()
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: []byte("x")})
	})
	_ = panicked.Wait()

	second := isolatedOutput(t, nil)
	task := second.Task("after")
	task.Define(func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: []byte("y")})
	})
	if err := task.Wait(); err != nil {
		t.Fatalf("hold leaked after panic: %v", err)
	}
}

func TestResourceMatrix_NestedFileRejected(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	gate := newWriteGateFS()
	out := isolatedOutput(t, gate)
	task := out.Task("nested")
	nested := make(chan error, 1)
	task.Define(func(ctx context.Context) error {
		go func() {
			<-gate.started
			nested <- evo.File(ctx, evo.FileSpec{Path: b, Contents: []byte("b")})
			close(gate.proceed)
		}()
		return evo.File(ctx, evo.FileSpec{Path: a, Contents: []byte("a")})
	})
	if err := task.Wait(); err != nil {
		t.Fatalf("outer File: %v", err)
	}
	select {
	case err := <-nested:
		if !errors.Is(err, evo.ErrNestedResource) {
			t.Fatalf("nested File=%v, want ErrNestedResource", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("nested File did not return")
	}
}

func TestResourceMatrix_SequentialFilesAllowed(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.txt")
	b := filepath.Join(dir, "b.txt")
	out := isolatedOutput(t, nil)
	task := out.Task("seq")
	task.Define(func(ctx context.Context) error {
		if err := evo.File(ctx, evo.FileSpec{Path: a, Contents: []byte("a")}); err != nil {
			return err
		}
		return evo.File(ctx, evo.FileSpec{Path: b, Contents: []byte("b")})
	})
	if err := task.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestResourceMatrix_HelperNestedFileRejected(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "dest.txt")
	nested := filepath.Join(dir, "nested.txt")
	fsys := &reenterFS{inner: testkit.NewFileFS(), nested: nested}
	out := isolatedOutput(t, fsys)
	task := out.Task("helper")
	task.Define(func(ctx context.Context) error {
		fsys.ctx = ctx
		return evo.File(ctx, evo.FileSpec{Path: dest, Contents: []byte("dest")})
	})
	if err := task.Wait(); err != nil {
		t.Fatalf("outer File: %v", err)
	}
	if !errors.Is(fsys.err, evo.ErrNestedResource) {
		t.Fatalf("helper reentry=%v, want ErrNestedResource", fsys.err)
	}
	if _, err := os.Stat(nested); !os.IsNotExist(err) {
		t.Fatal("nested File must not write")
	}
}

func TestResourceMatrix_UnrelatedStayConcurrent(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.txt")
	bPath := filepath.Join(dir, "b.txt")
	gate := newWriteGateFS()
	out := isolatedOutput(t, gate)
	g := out.Group("unrelated")
	a := g.Task("a")
	b := g.Task("b")
	a.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: aPath, Contents: []byte("a")})
	})
	b.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: bPath, Contents: []byte("b")})
	})
	select {
	case <-gate.started:
	case <-time.After(3 * time.Second):
		t.Fatal("first write never started")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && gate.inFlight.Load() < 2 {
		time.Sleep(5 * time.Millisecond)
	}
	if gate.inFlight.Load() < 2 {
		t.Fatal("unrelated writes did not overlap")
	}
	close(gate.proceed)
	if err := a.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := b.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestResourceMatrix_WaitSetsDoing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	gate := newWriteGateFS()
	out := isolatedOutput(t, gate)
	g := out.Group("doing")
	holder := g.Task("holder")
	waiter := g.Task("waiter")
	holder.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: []byte("a")})
	})
	select {
	case <-gate.started:
	case <-time.After(3 * time.Second):
		t.Fatal("holder never started")
	}
	waiter.Define(func(ctx context.Context) error {
		return evo.File(ctx, evo.FileSpec{Path: path, Contents: []byte("b")})
	})
	waitPhase(t, waiter, "waiting")
	close(gate.proceed)
	_ = holder.Wait()
	_ = waiter.Wait()
}

func TestResourceMatrix_NoCallerLockAPI(t *testing.T) {
	t.Parallel()
	live, err := apisurface.Walk(".")
	if err != nil {
		t.Fatal(err)
	}
	retired := []string{
		"IsLocked", "ReadLock", "WriteLock", "MultiLock",
		"TransactionBuilder", "ApplyPatch", "File.Patch", "Unlock", "Converge",
	}
	report := apisurface.Check(live, live, nil, retired)
	if len(report.RetiredPresent) > 0 {
		t.Fatalf("caller lock API present: %v", report.RetiredPresent)
	}
}
