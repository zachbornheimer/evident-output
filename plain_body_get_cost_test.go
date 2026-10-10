package evo_test

import (
	"context"
	"io"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output"
)

// The application body reading a settled Computed while another Task's
// callback is still running is the ordinary shape (read an inventory, keep
// going). Each Get must stay cheap however many goroutines the process has:
// it holds the Output's lock, so a slow Get stalls every row's rendering.
func TestPlainBodyGetIsCheapWhileACallbackRuns(t *testing.T) {
	const idle, reads = 5000, 200
	stop := make(chan struct{})
	defer close(stop)
	for range idle {
		go func() { <-stop }()
	}
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, Plain: true})
	defer func() { _ = out.Close() }()
	made := out.Task("make")
	producer := evo.Compute(made, func(context.Context) (int, error) { return 7, nil })
	if err := made.Wait(); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	out.Task("busy").Define(func(context.Context) error { close(started); <-release; return nil })
	<-started
	start := time.Now()
	sum := 0
	for range reads {
		sum += producer.Get()
	}
	took := time.Since(start)
	close(release)
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish = %v", err)
	}
	if sum != 7*reads {
		t.Fatalf("sum = %d, want %d", sum, 7*reads)
	}
	if took > 100*time.Millisecond {
		t.Fatalf("%d Get() with %d idle goroutines took %v: each Get stops the world while holding the Output lock", reads, idle, took)
	}
}
