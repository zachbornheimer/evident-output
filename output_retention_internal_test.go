package evo

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
	out := func() weak.Pointer[Run] {
		o := initRun(Config{stdout: io.Discard, stderr: io.Discard})
		_ = o.Task("a").Define(func(context.Context) error { return nil }).wait()
		_ = o.close()
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
