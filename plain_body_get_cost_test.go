package evo

import (
	"context"
	"io"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/graph"
)

const (
	getCostIdleGoroutines = 5000
	getCostReads          = 200
	// fewGetDumps is the stops of the world a stretch of reads may take
	// together; a read that stops the world each time takes getCostReads.
	fewGetDumps = 3
)

// withBusyCallbackAndSettledProducer runs body from the test goroutine while
// one Task's callback is running, idle goroutines are alive in the process,
// and producer is settled with 7. It returns the stops of the world body took.
func withBusyCallbackAndSettledProducer(t *testing.T, body func(producer *Computed[int])) int64 {
	t.Helper()
	stop := make(chan struct{})
	defer close(stop)
	for range getCostIdleGoroutines {
		go func() { <-stop }()
	}
	out := initRun(Config{Output: Plain, Facades: Facades{Stderr: io.Discard}})
	defer func() { _ = out.close() }()
	made := out.Task("make")
	producer := Compute(made, func(context.Context) (int, error) { return 7, nil })
	if err := made.wait(); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	out.Task("busy").Define(func(context.Context) error { close(started); <-release; return nil })
	<-started
	before := graph.GoroutineDumps()
	body(producer)
	dumps := graph.GoroutineDumps() - before
	close(release)
	if err := out.finish(); err != nil {
		t.Fatalf("Finish = %v", err)
	}
	return dumps
}

// The application body reading a settled Computed while another Task's
// callback is still running is the ordinary shape (read an inventory, keep
// going). Each Get must stay cheap however many goroutines the process has:
// it holds the Output's lock, so a slow Get stalls every row's rendering.
func TestPlainBodyGetIsCheapWhileACallbackRuns(t *testing.T) {
	sum := 0
	dumps := withBusyCallbackAndSettledProducer(t, func(producer *Computed[int]) {
		for range getCostReads {
			sum += producer.Get()
		}
	})
	if sum != 7*getCostReads {
		t.Fatalf("sum = %d, want %d", sum, 7*getCostReads)
	}
	if dumps > fewGetDumps {
		t.Fatalf("%d Get() with %d idle goroutines took %d goroutine dumps, want at most %d: each Get stops the world while holding the Output lock",
			getCostReads, getCostIdleGoroutines, dumps, fewGetDumps)
	}
}

// The body hands each item to a fresh goroutine that reads a settled Computed
// once, while another Task's callback runs. The body that started them is one
// goroutine, so what the process knows about it answers every read: a read
// must not stop the world to find out who started the goroutine asking.
func TestFreshGoroutineGetIsCheapWhileACallbackRuns(t *testing.T) {
	sum := 0
	dumps := withBusyCallbackAndSettledProducer(t, func(producer *Computed[int]) {
		for range getCostReads {
			got := make(chan int)
			go func() { got <- producer.Get() }()
			sum += <-got
		}
	})
	if sum != 7*getCostReads {
		t.Fatalf("sum = %d, want %d", sum, 7*getCostReads)
	}
	if dumps > fewGetDumps {
		t.Fatalf("%d fresh-goroutine Get() with %d idle goroutines took %d goroutine dumps, want at most %d: each one stops the world",
			getCostReads, getCostIdleGoroutines, dumps, fewGetDumps)
	}
}
