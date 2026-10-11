package graph

import (
	"fmt"
	"testing"
)

// maxDumpsWhenNothingStarts is the dumps a stretch of work that starts no
// goroutine may take: the first one, which learns what is alive, and a spare
// for a goroutine the runtime itself starts meanwhile. Never one per call.
const maxDumpsWhenNothingStarts = 2

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
		before := processLineage.dumps.Load()
		for i := range plainCalls {
			task := declare(g, fmt.Sprint("inline", i))
			submitWork(g, task, settlingWork(g, task, func() error { return nil }))
			// No Kick: the waiting caller runs the Task inline.
			if err := g.Wait(task); err != nil {
				t.Fatal(err)
			}
		}
		dumps = processLineage.dumps.Load() - before
	})
	if dumps > maxDumpsWhenNothingStarts {
		t.Fatalf("%d inline claims took %d dumps, over %d: each stops the world", plainCalls, dumps, maxDumpsWhenNothingStarts)
	}
}
