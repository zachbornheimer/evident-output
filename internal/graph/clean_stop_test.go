package graph

import (
	"context"
	"errors"
	"syscall"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/process"
)

func TestCleanStopEndsANamedCallbackSucceeded(t *testing.T) {
	var stops CleanStops
	started := make(chan struct{})
	done := make(chan error, 1)
	run := stops.Wrap([]process.Signal{syscall.SIGTERM}, func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	go func() { done <- run(context.Background()) }()
	<-started
	if stops.Stop(syscall.SIGINT) {
		t.Fatal("a signal the callback did not name was taken as its stop")
	}
	if !stops.Stop(syscall.SIGTERM) {
		t.Fatal("the named signal stopped nothing")
	}
	if err := <-done; err != nil {
		t.Fatalf("callback stopped by its named signal returned %v, want success", err)
	}
}

func TestCleanStopWithoutSignalsRunsTheCallbackAsIs(t *testing.T) {
	var stops CleanStops
	want := errors.New("boom")
	if err := stops.Wrap(nil, func(context.Context) error { return want })(context.Background()); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}
