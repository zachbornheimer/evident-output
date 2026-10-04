// Package harness holds the shared setup every contract conformance test
// uses: an isolated, plain, buffer-backed Output and snapshot lookups.
package harness

import (
	"bytes"
	"context"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// New returns an isolated plain Output whose human streams both land in the
// returned buffer. mutate adjusts the Config before Init. The Output is
// closed when the test ends.
func New(t *testing.T, mutate ...func(*evo.Config)) (*evo.Output, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	cfg := evo.Config{Isolated: true, Plain: true, Title: "run", Stdout: &buf, Stderr: &buf}
	for _, m := range mutate {
		m(&cfg)
	}
	out := evo.Init(cfg)
	t.Cleanup(func() { _ = out.Close() })
	return out, &buf
}

// Succeed defines an empty successful callback on task and waits for it.
func Succeed(task *evo.TaskHandle) error {
	return task.Define(func(context.Context) error { return nil }).Wait()
}

// Text finishes out and returns everything it wrote to the human stream.
func Text(out *evo.Output, buf *bytes.Buffer) string {
	_ = out.Finish()
	return buf.String()
}

// Lookup returns the snapshot of the task with the given name, searching
// top-level tasks and every nested Group or Sequence.
func Lookup(snap evo.Snapshot, name string) (evo.TaskSnapshot, bool) {
	for _, task := range snap.Tasks {
		if task.Name == name {
			return task, true
		}
	}
	for _, c := range snap.Collections {
		if task, ok := findIn(c, name); ok {
			return task, true
		}
	}
	return evo.TaskSnapshot{}, false
}

func findIn(c evo.TasksSnapshot, name string) (evo.TaskSnapshot, bool) {
	for _, task := range c.Tasks {
		if task.Name == name {
			return task, true
		}
	}
	for _, child := range c.Collections {
		if task, ok := findIn(child, name); ok {
			return task, true
		}
	}
	return evo.TaskSnapshot{}, false
}

// MustFind is Lookup that fails the test when the task is absent.
func MustFind(t *testing.T, out *evo.Output, name string) evo.TaskSnapshot {
	t.Helper()
	task, ok := Lookup(out.Snapshot(), name)
	if !ok {
		t.Fatalf("task %q not in snapshot", name)
	}
	return task
}
