package engine

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/fingerprint"
)

// contentionDeadline bounds how long a test waits for a claim it expects
// to be contended to show as waiting activity.
const contentionDeadline = 2 * time.Second

// contentionPollInterval is how often awaitWaitingActivity re-reads a
// task's activity text.
const contentionPollInterval = time.Millisecond

// taskPhase reads a task's current live activity text.
func taskPhase(o *Output, taskID string) string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if st := o.taskByRef[taskID]; st != nil {
		return st.phase
	}
	return ""
}

// awaitWaitingActivity reports whether taskID shows resource-waiting
// activity before deadline elapses.
func awaitWaitingActivity(o *Output, taskID string, deadline time.Duration) bool {
	poll := time.NewTicker(contentionPollInterval)
	defer poll.Stop()
	timeout := time.NewTimer(deadline)
	defer timeout.Stop()
	for {
		if strings.HasPrefix(taskPhase(o, taskID), resourceWaitingPrefix) {
			return true
		}
		select {
		case <-poll.C:
		case <-timeout.C:
			return false
		}
	}
}

// tornWriteFS commits target in two halves with a visible gap between
// them — the widest window a concurrent observer could ever race into.
// Every other path passes straight through to the real filesystem.
type tornWriteFS struct {
	osFileFS
	target    string
	midCommit chan struct{}
	// resume blocks the second half until it returns.
	resume func()
}

func (f *tornWriteFS) WriteAtomic(path string, contents []byte, mode fs.FileMode) error {
	if path != f.target {
		return f.osFileFS.WriteAtomic(path, contents, mode)
	}
	if err := os.WriteFile(path, contents[:len(contents)/2], mode); err != nil {
		return err
	}
	close(f.midCommit)
	f.resume()
	return os.WriteFile(path, contents, mode)
}

func fsPathDigest(t *testing.T, path string) string {
	t.Helper()
	v, err := fingerprint.FSPath(path).Fingerprint(context.Background())
	if err != nil {
		t.Fatalf("fingerprint %q: %v", path, err)
	}
	return hex.EncodeToString(v.Digest[:])
}

// A File whose Basis observes a path another Task is committing must see
// either the whole old state or the whole new state — never the torn
// middle of the commit.
func TestBasisObservationCannotRaceFileCommit(t *testing.T) {
	dir := t.TempDir()
	shared := filepath.Join(dir, "shared.txt")
	derived := filepath.Join(dir, "derived.txt")
	if err := os.WriteFile(shared, []byte("old old old old"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out *Output
	var readerID string
	fsys := &tornWriteFS{target: shared, midCommit: make(chan struct{})}
	fsys.resume = func() {
		// Give the observer every chance to race: hold the torn state until
		// it is visibly waiting, or long enough that it would have read.
		awaitWaitingActivity(out, readerID, 300*time.Millisecond)
	}
	out = newOutput("race", to(io.Discard), plain(), withNoColor(), withStateDir(t.TempDir()), withFileFS(fsys), maxConcurrency(4))
	t.Cleanup(func() { _ = out.Close() })

	writer := out.Task("write shared")
	reader := out.Task("derive from shared")
	readerID = reader.id
	writer.Define(func(ctx context.Context) error {
		return File(ctx, FileSpec{Path: shared, Contents: []byte("new new new new new new")})
	})
	reader.Define(func(ctx context.Context) error {
		<-fsys.midCommit
		return File(ctx, FileSpec{Path: derived, Contents: []byte("derived"), Basis: []fingerprint.Fingerprint{fingerprint.FSPath(shared)}})
	})
	if err := writer.Wait(); err != nil {
		t.Fatalf("writer: %v", err)
	}
	if err := reader.Wait(); err != nil {
		t.Fatalf("reader: %v", err)
	}

	out.mu.Lock()
	ops := out.taskByRef[reader.id].manifestOps
	out.mu.Unlock()
	if len(ops) != 1 || len(ops[0].Basis) != 1 {
		t.Fatalf("reader recorded %d ops, want 1 with one Basis entry: %+v", len(ops), ops)
	}
	if got, want := ops[0].Basis[0].Digest, fsPathDigest(t, shared); got != want {
		t.Fatalf("Basis observed a torn commit: digest %s, want the committed state %s", got, want)
	}
}

// File claims write-side ownership of its own path with no caller code, so
// it waits out an Effect's coarse claim on an ancestor directory — and the
// wait is visible as activity while it lasts.
func TestFileWaitsForCoarseEffectClaim(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "go.mod")
	out := newOutput("module", to(io.Discard), plain(), withNoColor(), withStateDir(t.TempDir()), maxConcurrency(4))
	t.Cleanup(func() { _ = out.Close() })

	entered, release := make(chan struct{}), make(chan struct{})
	tidy := out.Task("tidy module")
	tidy.Define(func(ctx context.Context) error {
		spec := EffectSpec{Verb: EffectUpdate, Object: "module", Quantity: 1, Resource: FSResource(dir)}
		return Effect(ctx, spec, func(context.Context) error {
			close(entered)
			<-release
			return nil
		})
	})
	write := out.Task("write go.mod")
	write.Define(func(ctx context.Context) error {
		<-entered
		return File(ctx, FileSpec{Path: target, Contents: []byte("module x\n")})
	})

	<-entered
	waiting := awaitWaitingActivity(out, write.id, contentionDeadline)
	_, statErr := os.Stat(target)
	close(release)
	if err := tidy.Wait(); err != nil {
		t.Fatalf("tidy: %v", err)
	}
	if err := write.Wait(); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !errors.Is(statErr, fs.ErrNotExist) {
		t.Fatalf("File wrote %s while an Effect held its directory (stat err %v)", target, statErr)
	}
	if !waiting {
		t.Fatalf("contended File never showed waiting activity; phase = %q", taskPhase(out, write.id))
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "module x\n" {
		t.Fatalf("File after release = %q, %v", got, err)
	}
}

// holdBriefly keeps a claim held across enough scheduler turns that an
// overlapping holder, if coordination were missing, would be observed.
func holdBriefly() {
	for range 200 {
		runtime.Gosched()
	}
}

// Effects claiming one coarse logical resource never overlap, and the
// caller writes no locking code to get that.
func TestEffectsOnOneLogicalResourceNeverOverlap(t *testing.T) {
	name := "package-db " + t.Name()
	out := newOutput("install", to(io.Discard), plain(), withNoColor(), maxConcurrency(8))
	t.Cleanup(func() { _ = out.Close() })

	var active, peak atomic.Int32
	group := out.Group("packages")
	tasks := make([]*TaskHandle, 0, 8)
	for _, pkg := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		task := group.Task("install " + pkg)
		// Surrounding whitespace names the same logical resource.
		resource := LogicalResource(name)
		if pkg == "b" {
			resource = LogicalResource("  " + name + "\t")
		}
		task.Define(func(ctx context.Context) error {
			spec := EffectSpec{Verb: EffectAdd, Object: "package", Quantity: 1, Resource: resource}
			return Effect(ctx, spec, func(context.Context) error {
				n := active.Add(1)
				for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
				}
				holdBriefly()
				active.Add(-1)
				return nil
			})
		})
		tasks = append(tasks, task)
	}
	for _, task := range tasks {
		if err := task.Wait(); err != nil {
			t.Fatalf("%s: %v", task.id, err)
		}
	}
	if got := peak.Load(); got != 1 {
		t.Fatalf("peak concurrent holders of one logical resource = %d, want 1", got)
	}
}

// An Effect's callback runs holding the Effect's resource, so tracked work
// inside it that would need a second resource is misuse, not a deadlock.
func TestFileInsideResourceEffectIsNestedMisuse(t *testing.T) {
	dir := t.TempDir()
	out := newOutput("nested", to(io.Discard), plain(), withNoColor(), withStateDir(t.TempDir()))
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("publish")
	var fileErr error
	task.Define(func(ctx context.Context) error {
		spec := EffectSpec{Verb: EffectPush, Object: "tag", Quantity: 1, Resource: LogicalResource("remote " + t.Name())}
		return Effect(ctx, spec, func(held context.Context) error {
			fileErr = File(held, FileSpec{Path: filepath.Join(dir, "stamp"), Contents: []byte("x")})
			return fileErr
		})
	})
	_ = task.Wait()
	if !errors.Is(fileErr, ErrNestedResourceAcquisition) {
		t.Fatalf("File inside a resource-holding Effect = %v, want ErrNestedResourceAcquisition", fileErr)
	}
	if _, err := os.Stat(filepath.Join(dir, "stamp")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("nested File still wrote its path (stat err %v)", err)
	}
}

// An Effect naming an invalid resource is refused on every run, dry or
// not, before its callback runs.
func TestEffectRejectsInvalidResource(t *testing.T) {
	for _, dry := range []bool{false, true} {
		opts := []Option{to(io.Discard), plain(), withNoColor()}
		if dry {
			opts = append(opts, dryRun())
		}
		out := newOutput("invalid", opts...)
		task := out.Task("push")
		invoked := false
		var effectErr error
		task.Define(func(ctx context.Context) error {
			spec := EffectSpec{Verb: EffectPush, Object: "tag", Quantity: 1, Resource: LogicalResource(" ")}
			effectErr = Effect(ctx, spec, func(context.Context) error { invoked = true; return nil })
			return effectErr
		})
		_ = task.Wait()
		_ = out.Close()
		if !errors.Is(effectErr, ErrInvalidResource) {
			t.Fatalf("dry=%v: Effect err = %v, want ErrInvalidResource", dry, effectErr)
		}
		if invoked {
			t.Fatalf("dry=%v: Effect invoked its callback despite an invalid resource", dry)
		}
	}
}

// An uncontended resource claim is invisible: an Effect that names a
// resource renders byte-for-byte like one that does not.
func TestUncontendedEffectResourceIsSilent(t *testing.T) {
	render := func(resource Resource) string {
		var buf bytes.Buffer
		out := newOutput("prune", to(&buf), plain(), withNoColor())
		task := out.Task("prune worktrees")
		task.Define(func(ctx context.Context) error {
			spec := EffectSpec{Verb: EffectDelete, Object: "worktree", Quantity: 2, Resource: resource}
			return Effect(ctx, spec, func(context.Context) error { return nil })
		})
		if err := task.Wait(); err != nil {
			t.Fatalf("prune: %v", err)
		}
		_ = out.Finish()
		return buf.String()
	}
	bare := render(nil)
	claimed := render(FSResource(t.TempDir()))
	if bare != claimed {
		t.Fatalf("uncontended claim changed the rendering:\n--- bare ---\n%s\n--- claimed ---\n%s", bare, claimed)
	}
}
