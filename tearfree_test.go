package evo_test

import (
	"fmt"
	"sync"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

// TestDurableWrite_ClearedAndRedrawnAroundEveryPrintln is the red-first case
// for evo-rec.md item #12: a durable Println while the live region is active
// must be preceded by a complete clear and followed by a full redraw — never
// leaving the live surface holding a stale or half-drawn frame after the
// call returns. A concurrency stress test (below) proves this holds under
// contention; this test pins the single-goroutine sequencing contract the
// stress test depends on.
func TestDurableWrite_ClearedAndRedrawnAroundEveryPrintln(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive())
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("demo"), evo.Terminal(screen), evo.VisibilityDelay(0)}})
	task := out.Task("install")
	task.Phase("working")
	if screen.LiveFrameCount() == 0 {
		t.Fatal("setup: expected an initial live frame before Println")
	}

	out.Println("using cached wheel index")

	ops := screen.Operations()
	clearIdx, durableIdx, liveIdx := -1, -1, -1
	for i := len(ops) - 1; i >= 0; i-- {
		if ops[i].Kind == "durable" && durableIdx == -1 {
			durableIdx = i
		}
	}
	if durableIdx == -1 {
		t.Fatalf("expected a durable op recorded, got %+v", ops)
	}
	for i := durableIdx - 1; i >= 0; i-- {
		if ops[i].Kind == "clear" {
			clearIdx = i
			break
		}
	}
	for i := durableIdx + 1; i < len(ops); i++ {
		if ops[i].Kind == "live" {
			liveIdx = i
			break
		}
	}
	if clearIdx == -1 {
		t.Fatalf("durable write at %d has no preceding clear: %+v", durableIdx, ops)
	}
	if liveIdx == -1 {
		t.Fatalf("durable write at %d has no following live redraw: %+v", durableIdx, ops)
	}
	_ = out.Finish()
}

// TestDurableWrite_TearFreeUnderConcurrentProgress is the stress test the
// work order calls for: one goroutine drives Progress updates as fast as
// possible while another calls Println concurrently. Every durable line must
// land between a complete clear and a full redraw — never mid-frame.
func TestDurableWrite_TearFreeUnderConcurrentProgress(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive())
	out := evo.Init(evo.Config{Options: []evo.Option{evo.Title("demo"), evo.Terminal(screen), evo.VisibilityDelay(0)}})
	task := out.Task("install")
	task.Progress(0, 1000)

	const iterations = 500
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 1; i <= iterations; i++ {
			task.Progress(i, 1000)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations/10; i++ {
			out.Println(fmt.Sprintf("note %d", i))
		}
	}()
	wg.Wait()
	task.Done()
	_ = out.Finish()

	ops := screen.Operations()
	for i, op := range ops {
		if op.Kind != "durable" {
			continue
		}
		if i == 0 || ops[i-1].Kind != "clear" {
			t.Fatalf("durable op %d (%q) not preceded by a clear: %+v", i, op.Text, ops[max(0, i-3):min(len(ops), i+1)])
		}
		// A full redraw ("live") must follow before the next durable write or
		// the end of the run — a live frame written mid-clear (no clear
		// immediately before it) would indicate a torn frame.
		sawLive := false
		for j := i + 1; j < len(ops); j++ {
			if ops[j].Kind == "durable" {
				break
			}
			if ops[j].Kind == "live" {
				sawLive = true
				break
			}
		}
		if !sawLive && i != len(ops)-1 {
			t.Fatalf("durable op %d (%q) has no full redraw before run end: %+v", i, op.Text, ops)
		}
	}
}
