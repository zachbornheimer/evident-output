// Package basis_test is the ZYS-1382 execution contract for Task Basis:
// freshness inputs whose identity decides whether a Task is current or must
// rerun. Names and open-syntax choices: docs/zys-1382/contract-decisions.md.
package basis_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

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

// runOnce runs one Task named "install" with basis on a fresh Output that
// shares stateDir with earlier runs, and reports whether its callback ran.
func runOnce(t *testing.T, stateDir string, basis func(task *evo.TaskHandle) *evo.TaskHandle) bool {
	t.Helper()
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: stateDir,
	})
	var ran atomic.Bool
	task := basis(out.Task("install")).Define(func(context.Context) error {
		ran.Store(true)
		return nil
	})
	if err := task.Wait(); err != nil {
		t.Fatalf("task: %v", err)
	}
	if err := out.Finish(); err != nil {
		t.Fatalf("finish: %v", err)
	}
	return ran.Load()
}

// Basis takes File, Tree, Value, and App in one call and returns the same
// TaskHandle, so it chains before Define like Key and After.
func TestBasisAcceptsFileTreeAndFingerprintInputs(t *testing.T) {
	work := t.TempDir()
	plant(t, work, map[string]string{"package.json": "{}", "patches/a.patch": "a"})
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir()})
	handle := out.Task("mixed")
	chained := handle.Basis(
		evo.File{Path: filepath.Join(work, "package.json")},
		evo.Tree{Path: filepath.Join(work, "patches")},
		evo.Value("node", "22.1.0"),
		evo.App(),
	)
	if chained != handle {
		t.Fatalf("Basis returned %p, want the same TaskHandle %p for chaining", chained, handle)
	}
	if err := chained.Define(func(context.Context) error { return nil }).Wait(); err != nil {
		t.Fatalf("task with a mixed Basis: %v", err)
	}
	if err := out.Finish(); err != nil {
		t.Fatalf("finish: %v", err)
	}
}

func TestBasisUnchangedMakesTheTaskCurrent(t *testing.T) {
	work, state := t.TempDir(), t.TempDir()
	plant(t, work, map[string]string{"package.json": "{}", "package-lock.json": "{}"})
	basis := func(task *evo.TaskHandle) *evo.TaskHandle {
		return task.Basis(
			evo.File{Path: filepath.Join(work, "package.json")},
			evo.File{Path: filepath.Join(work, "package-lock.json")},
		)
	}
	if !runOnce(t, state, basis) {
		t.Fatalf("first run: the Task never ran")
	}
	if runOnce(t, state, basis) {
		t.Fatalf("second run with an unchanged Basis reran the Task; want it current")
	}
}

func TestBasisChangeRerunsTheTask(t *testing.T) {
	cases := []struct {
		name   string
		basis  func(work string) func(*evo.TaskHandle) *evo.TaskHandle
		mutate func(t *testing.T, work string)
	}{
		{"a File's bytes change", func(work string) func(*evo.TaskHandle) *evo.TaskHandle {
			return func(task *evo.TaskHandle) *evo.TaskHandle {
				return task.Basis(evo.File{Path: filepath.Join(work, "package.json")})
			}
		}, func(t *testing.T, work string) {
			plant(t, work, map[string]string{"package.json": `{"changed":true}`})
		}},
		{"a File disappears", func(work string) func(*evo.TaskHandle) *evo.TaskHandle {
			return func(task *evo.TaskHandle) *evo.TaskHandle {
				return task.Basis(evo.File{Path: filepath.Join(work, "package.json")})
			}
		}, func(t *testing.T, work string) {
			if err := os.Remove(filepath.Join(work, "package.json")); err != nil {
				t.Fatal(err)
			}
		}},
		{"a second File in the Basis changes", func(work string) func(*evo.TaskHandle) *evo.TaskHandle {
			return func(task *evo.TaskHandle) *evo.TaskHandle {
				return task.Basis(evo.File{Path: filepath.Join(work, "package.json")}, evo.File{Path: filepath.Join(work, "package-lock.json")})
			}
		}, func(t *testing.T, work string) {
			plant(t, work, map[string]string{"package-lock.json": `{"v":2}`})
		}},
		{"a file inside a Tree changes", func(work string) func(*evo.TaskHandle) *evo.TaskHandle {
			return func(task *evo.TaskHandle) *evo.TaskHandle {
				return task.Basis(evo.Tree{Path: filepath.Join(work, "patches")})
			}
		}, func(t *testing.T, work string) {
			plant(t, work, map[string]string{"patches/a.patch": "changed"})
		}},
		{"a file is added inside a Tree", func(work string) func(*evo.TaskHandle) *evo.TaskHandle {
			return func(task *evo.TaskHandle) *evo.TaskHandle {
				return task.Basis(evo.Tree{Path: filepath.Join(work, "patches")})
			}
		}, func(t *testing.T, work string) {
			plant(t, work, map[string]string{"patches/b.patch": "new"})
		}},
		{"a file is removed from a Tree", func(work string) func(*evo.TaskHandle) *evo.TaskHandle {
			return func(task *evo.TaskHandle) *evo.TaskHandle {
				return task.Basis(evo.Tree{Path: filepath.Join(work, "patches")})
			}
		}, func(t *testing.T, work string) {
			if err := os.Remove(filepath.Join(work, "patches", "a.patch")); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			work, state := t.TempDir(), t.TempDir()
			plant(t, work, map[string]string{"package.json": "{}", "package-lock.json": "{}", "patches/a.patch": "a"})
			basis := tc.basis(work)
			if !runOnce(t, state, basis) {
				t.Fatalf("first run: the Task never ran")
			}
			if runOnce(t, state, basis) {
				t.Fatalf("unchanged Basis reran the Task")
			}
			tc.mutate(t, work)
			if !runOnce(t, state, basis) {
				t.Fatalf("changed Basis left the Task current; want a rerun")
			}
		})
	}
}

// The Basis is a set of identities: adding or dropping an input is a
// change, so the Task reruns even when every file is untouched.
func TestBasisInputSetChangeRerunsTheTask(t *testing.T) {
	work, state := t.TempDir(), t.TempDir()
	plant(t, work, map[string]string{"package.json": "{}", "package-lock.json": "{}"})
	one := func(task *evo.TaskHandle) *evo.TaskHandle {
		return task.Basis(evo.File{Path: filepath.Join(work, "package.json")})
	}
	two := func(task *evo.TaskHandle) *evo.TaskHandle {
		return task.Basis(evo.File{Path: filepath.Join(work, "package.json")}, evo.File{Path: filepath.Join(work, "package-lock.json")})
	}
	if !runOnce(t, state, one) {
		t.Fatalf("first run never ran")
	}
	if !runOnce(t, state, two) {
		t.Fatalf("adding a Basis input left the Task current")
	}
	if runOnce(t, state, two) {
		t.Fatalf("unchanged two-input Basis reran the Task")
	}
	if !runOnce(t, state, one) {
		t.Fatalf("dropping a Basis input left the Task current")
	}
}

func TestBasisValueChangeRerunsTheTask(t *testing.T) {
	state := t.TempDir()
	with := func(v string) func(*evo.TaskHandle) *evo.TaskHandle {
		return func(task *evo.TaskHandle) *evo.TaskHandle { return task.Basis(evo.Value("node", v)) }
	}
	if !runOnce(t, state, with("22.1.0")) {
		t.Fatalf("first run never ran")
	}
	if runOnce(t, state, with("22.1.0")) {
		t.Fatalf("same Value reran the Task")
	}
	if !runOnce(t, state, with("22.2.0")) {
		t.Fatalf("changed Value left the Task current")
	}
}

func TestBasisIdentityIgnoresModeAndMtime(t *testing.T) {
	work, state := t.TempDir(), t.TempDir()
	plant(t, work, map[string]string{"package.json": "{}"})
	path := filepath.Join(work, "package.json")
	basis := func(task *evo.TaskHandle) *evo.TaskHandle { return task.Basis(evo.File{Path: path}) }
	if !runOnce(t, state, basis) {
		t.Fatalf("first run never ran")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil { // same bytes, new mtime
		t.Fatal(err)
	}
	if runOnce(t, state, basis) {
		t.Fatalf("a mode/mtime-only change reran the Task; Basis identity is content")
	}
}

func TestBasisWithoutStateAlwaysRuns(t *testing.T) {
	work := t.TempDir()
	plant(t, work, map[string]string{"package.json": "{}"})
	basis := func(task *evo.TaskHandle) *evo.TaskHandle {
		return task.Basis(evo.File{Path: filepath.Join(work, "package.json")})
	}
	// Separate state dirs: no prior record, so each run is a first run.
	for i := range 2 {
		if !runOnce(t, t.TempDir(), basis) {
			t.Fatalf("run %d with no prior record did not run", i)
		}
	}
}

// overlapTimeout bounds how long a test waits for two Tasks that must
// overlap. Hitting it means Evo serialized them; the test fails, never hangs.
const overlapTimeout = 10 * time.Second

// awaitWithin fails the test when ch is not closed within overlapTimeout.
func awaitWithin(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(overlapTimeout):
		t.Fatalf("%s did not happen within %v", what, overlapTimeout)
	}
}

// Basis is freshness, After is ordering. A Task whose Basis names a file
// does not wait for a sibling Task that is about to write that file: the
// consumer finishes while the producer is still blocked before its Write.
func TestBasisIsNotOrdering(t *testing.T) {
	work := t.TempDir()
	plant(t, work, map[string]string{"package.json": "{}"})
	path := filepath.Join(work, "package.json")
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir()})
	group := out.Group("pipeline")
	release := make(chan struct{})
	producerStarted := make(chan struct{})
	consumerDone := make(chan struct{})
	producer := group.Task("write package.json").Define(func(ctx context.Context) error {
		close(producerStarted)
		<-release
		return evo.File{Path: path, Content: evo.Bytes(`{"v":2}`)}.Write(ctx)
	})
	consumer := group.Task("install").Basis(evo.File{Path: path}).Define(func(context.Context) error {
		close(consumerDone)
		return nil
	})
	awaitWithin(t, producerStarted, "the producer starting")
	select {
	case <-consumerDone:
	case <-time.After(overlapTimeout):
		close(release)
		t.Fatalf("a Task with a File Basis waited for a Task that writes that file; Basis is not ordering")
	}
	close(release)
	if err := consumer.Wait(); err != nil {
		t.Fatalf("consumer: %v", err)
	}
	if err := producer.Wait(); err != nil {
		t.Fatalf("producer: %v", err)
	}
	if err := out.Finish(); err != nil {
		t.Fatalf("finish: %v", err)
	}
}

// After orders; Basis is observed when the Task starts, after its
// predecessors. A producer that rewrites identical bytes every Run therefore
// leaves the consumer current from the second Run on.
func TestBasisComposesWithAfter(t *testing.T) {
	work, state := t.TempDir(), t.TempDir()
	path := filepath.Join(work, "package.json")
	run := func() (consumerRan bool) {
		t.Helper()
		out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: state})
		producer := out.Task("write package.json").Define(func(ctx context.Context) error {
			return evo.File{Path: path, Content: evo.Bytes("{}")}.Write(ctx)
		})
		var ran atomic.Bool
		consumer := out.Task("install").After(producer).Basis(evo.File{Path: path}).Define(func(context.Context) error {
			if _, err := os.Stat(path); err != nil {
				return fmt.Errorf("consumer ran before its After predecessor wrote %s: %w", path, err)
			}
			ran.Store(true)
			return nil
		})
		if err := consumer.Wait(); err != nil {
			t.Fatalf("consumer: %v", err)
		}
		if err := out.Finish(); err != nil {
			t.Fatalf("finish: %v", err)
		}
		return ran.Load()
	}
	if !run() {
		t.Fatalf("first run: the consumer never ran")
	}
	if run() {
		t.Fatalf("second run reran the consumer although its Basis was unchanged since it last started; Basis must be observed at Task start, after After predecessors")
	}
}

// Two Tasks sharing a Basis file run concurrently: Basis is an observation,
// not a lock claim.
func TestBasisIsNotALock(t *testing.T) {
	work := t.TempDir()
	plant(t, work, map[string]string{"package.json": "{}"})
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir()})
	group := out.Group("readers")
	bothIn := make(chan struct{})
	var inFlight atomic.Int32
	var overlapped atomic.Bool
	for _, name := range []string{"lint", "typecheck"} {
		group.Task(name).Basis(evo.File{Path: filepath.Join(work, "package.json")}).Define(func(context.Context) error {
			if inFlight.Add(1) == 2 {
				close(bothIn)
			}
			defer inFlight.Add(-1)
			select {
			case <-bothIn:
				overlapped.Store(true)
			case <-time.After(overlapTimeout):
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		t.Fatalf("group: %v", err)
	}
	if err := out.Finish(); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if !overlapped.Load() {
		t.Fatalf("Tasks sharing a Basis file never overlapped within %v; Basis must not lock", overlapTimeout)
	}
}

// Not a lock, against a writer: a Task whose Basis names a file holds no
// claim on it while its callback runs, so a sibling Task's File.Write to
// that path commits while the Basis Task is still running. A shared read
// claim held for the Task's lifetime would pass TestBasisIsNotALock and
// fail here.
func TestBasisDoesNotBlockAWriterOfItsInput(t *testing.T) {
	work := t.TempDir()
	plant(t, work, map[string]string{"package.json": "{}"})
	path := filepath.Join(work, "package.json")
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir()})
	group := out.Group("pipeline")
	readerStarted := make(chan struct{})
	writerDone := make(chan struct{})
	reader := group.Task("install").Basis(evo.File{Path: path}).Define(func(context.Context) error {
		close(readerStarted)
		select {
		case <-writerDone:
			return nil
		case <-time.After(overlapTimeout):
			return fmt.Errorf("a File.Write to this Task's Basis input did not commit within %v while the Task ran; Basis must not lock", overlapTimeout)
		}
	})
	writer := group.Task("write package.json").Define(func(ctx context.Context) error {
		defer close(writerDone)
		select {
		case <-readerStarted:
		case <-time.After(overlapTimeout):
			return fmt.Errorf("the Basis Task did not start within %v", overlapTimeout)
		}
		return evo.File{Path: path, Content: evo.Bytes(`{"v":2}`)}.Write(ctx)
	})
	if err := writer.Wait(); err != nil {
		t.Fatalf("writer: %v", err)
	}
	if err := reader.Wait(); err != nil {
		t.Fatalf("reader: %v", err)
	}
	if err := out.Finish(); err != nil {
		t.Fatalf("finish: %v", err)
	}
}

// Basis is an observation, not provenance and not establishment: a File
// with declared Content in a Basis never writes that Content, and a Basis
// on a path nothing has created yet is not a failure.
func TestBasisNeverEstablishesItsInputs(t *testing.T) {
	work := t.TempDir()
	plant(t, work, map[string]string{"package.json": "{}"})
	declared := filepath.Join(work, "package.json")
	missing := filepath.Join(work, "pnpm-lock.yaml")
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir()})
	err := out.Task("install").Basis(
		evo.File{Path: declared, Content: evo.Bytes(`{"desired":true}`)},
		evo.File{Path: missing},
		evo.Tree{Path: filepath.Join(work, "patches")},
	).Define(func(context.Context) error { return nil }).Wait()
	if err != nil {
		t.Fatalf("task with missing Basis inputs: %v", err)
	}
	if err := out.Finish(); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if got, err := os.ReadFile(declared); err != nil || string(got) != "{}" {
		t.Fatalf("Basis File content after the Task = %q, %v; want the untouched %q (Basis never writes)", got, err, "{}")
	}
	for _, p := range []string{missing, filepath.Join(work, "patches")} {
		if _, err := os.Lstat(p); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("Basis created %s (Lstat err %v); Basis never establishes its inputs", p, err)
		}
	}
}

// Basis is Task configuration, frozen at Define like Key: a later call is
// recorded as misuse (the Output's Err) and ignored.
func TestBasisAfterDefineIsRecordedAsMisuse(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard, StateDir: t.TempDir()})
	task := out.Task("late").Define(func(context.Context) error { return nil })
	_ = task.Wait()
	task.Basis(evo.Value("x", 1))
	_ = out.Finish()
	if !errors.Is(out.Err(), evo.ErrBasisAfterDefine) {
		t.Fatalf("Err() = %v, want ErrBasisAfterDefine", out.Err())
	}
}
