package engine

import (
	"context"
	"errors"
	"testing"
)

// TestWaitOnUndefinedTaskReportsNotStarted proves Wait never reports
// success for a Task nobody Defined: its work never ran.
func TestWaitOnUndefinedTaskReportsNotStarted(t *testing.T) {
	out := Init(Config{Isolated: true, StateDir: t.TempDir()})
	t.Cleanup(func() { _ = out.Close() })
	if err := out.Task("never defined").Wait(); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("Wait on an undefined Task = %v, want ErrNotStarted", err)
	}
}

// TestGroupWaitWithUndefinedChildReportsNotStarted proves a container
// with one undefined child never reports that every descendant succeeded.
func TestGroupWaitWithUndefinedChildReportsNotStarted(t *testing.T) {
	out := Init(Config{Isolated: true, StateDir: t.TempDir()})
	t.Cleanup(func() { _ = out.Close() })
	group := out.Group("items")
	group.Task("defined").Define(func(context.Context) error { return nil })
	group.Task("undefined")
	if err := group.Wait(); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("Group.Wait with an undefined child = %v, want ErrNotStarted", err)
	}
}
