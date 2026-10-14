//go:build ignore

// Package harness holds the shared setup every contract conformance test
// uses: a plain, buffer-backed Run driven through evo.Execute.
package harness

import (
	"bytes"
	"context"
	"testing"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// New starts a Run, holds it open until the test ends, and returns it with
// the human stream buffer. mutate adjusts Config before Execute.
func New(t *testing.T, mutate ...func(*evo.Config)) (*evo.Run, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	cfg := evo.Config{Name: "run", Output: evo.Plain, Facades: evo.Facades{
		Stdout: &buf,
		Stderr: &buf,
	}}
	for _, m := range mutate {
		m(&cfg)
	}
	ready := make(chan *evo.Run, 1)
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_ = evo.Execute(context.Background(), cfg, func(ctx context.Context, r *evo.Run) error {
			ready <- r
			<-done
			return nil
		})
	}()
	t.Cleanup(func() {
		select {
		case <-done:
		default:
			close(done)
		}
		<-finished
	})
	select {
	case r := <-ready:
		return r, &buf
	case <-finished:
		t.Fatal("run ended before body")
		return nil, &buf
	}
}

// Succeed defines an empty successful callback on task.
func Succeed(task *evo.Task) error {
	task.Define(func(context.Context) error { return nil })
	return nil
}

// Text returns everything written to the human stream.
func Text(out *evo.Run, buf *bytes.Buffer) string {
	_ = out
	return buf.String()
}

// TaskSnapshot is a 2.0 stand-in for the removed snapshot row.
type TaskSnapshot struct {
	Name string
}

// Lookup is a 2.0 stand-in; snapshots are no longer a public surface.
func Lookup(_ struct{}, name string) (TaskSnapshot, bool) {
	return TaskSnapshot{}, false
}

// MustFind fails the test: snapshots are no longer a public surface.
func MustFind(t *testing.T, _ *evo.Run, name string) TaskSnapshot {
	t.Helper()
	t.Fatalf("task %q not in snapshot", name)
	return TaskSnapshot{}
}
