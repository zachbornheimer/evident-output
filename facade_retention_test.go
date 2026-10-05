package evo

import (
	"context"
	"io"
	"runtime"
	"testing"
	"weak"

	"github.com/zachbornheimer/evident-output/internal/engine"
)

// TestClosedOutputIsCollectable proves the public wrappers do not pin
// their engine state: once a caller drops an Output, its engine Output,
// with every Task, journal, and evidence, is garbage. A process-global
// wrapper table used to keep every Output ever created alive.
func TestClosedOutputIsCollectable(t *testing.T) {
	inner := func() weak.Pointer[engine.Output] {
		out := Init(Config{Isolated: true, Stdout: io.Discard, Stderr: io.Discard})
		group := out.Group("g")
		for _, name := range []string{"a", "b", "c"} {
			group.Task(name).Define(func(context.Context) error { return nil })
		}
		_ = group.Wait()
		out.Task("failed").Define(func(context.Context) error { return io.EOF })
		_ = out.Close()
		return weak.Make(out.inner)
	}()
	for range 10 {
		runtime.GC()
		if inner.Value() == nil {
			return
		}
	}
	t.Fatal("a dropped Output's engine state is still reachable after GC")
}
