package graph

import (
	"fmt"
	"testing"
)

// A plain caller that Waits on work the pool has not started runs it inline in
// a free slot, and lists the goroutines it had started so the callback is not
// charged with them. While no goroutine starts, the list cannot change, so
// the claims share what one dump taught and none stops the world again.
func TestPlainCallerInlineClaimsTakeNoDumpWhileNoGoroutineStarts(t *testing.T) {
	if _, counted := goroutinesCreated(); !counted {
		t.Skip("this toolchain does not count the goroutines the process started")
	}
	var dumps int64
	withBusyCallback(t, idleGoroutines, func(g *Graph) {
		dumps = dumpsDuring(func() {
			for i := range plainCalls {
				task := declare(g, fmt.Sprint("inline", i))
				submitWork(g, task, settlingWork(g, task, func() error { return nil }))
				// No Kick: the waiting caller runs the Task inline.
				if err := g.Wait(task); err != nil {
					t.Fatal(err)
				}
			}
		})
	})
	if dumps > fewDumps {
		t.Fatalf("%d inline claims took %d dumps, over %d: each stops the world", plainCalls, dumps, fewDumps)
	}
}
