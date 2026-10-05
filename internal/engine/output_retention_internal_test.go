package engine

import (
	"context"
	"io"
	"runtime"
	"testing"
	"weak"
)

// TestClosedOutputIsCollectable: once a caller drops a closed Output, its
// state is garbage. A settled Task's plain heartbeat timer used to keep
// the Output reachable until the timer fired, 30s later.
func TestClosedOutputIsCollectable(t *testing.T) {
	out := func() weak.Pointer[Output] {
		o := Init(Config{Isolated: true, Stdout: io.Discard, Stderr: io.Discard})
		_ = o.Task("a").Define(func(context.Context) error { return nil }).Wait()
		_ = o.Close()
		return weak.Make(o)
	}()
	for range 10 {
		runtime.GC()
		if out.Value() == nil {
			return
		}
	}
	t.Fatal("a dropped, closed Output is still reachable after GC")
}
