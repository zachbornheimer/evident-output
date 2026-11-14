package evo

import (
	"context"
	"io"
	"testing"

	"github.com/zachbornheimer/evident-output/v2/internal/process"
)

// recordSubscriptions swaps the signal facade for one that records each subscription's signals.
func recordSubscriptions(t *testing.T) *[][]process.Signal {
	t.Helper()
	prev := notifySignals
	t.Cleanup(func() { notifySignals = prev })
	var got [][]process.Signal
	notifySignals = func(_ chan<- process.Signal, sig ...process.Signal) { got = append(got, sig) }
	return &got
}

func quietConfig() Config {
	return Config{Output: Plain, Facades: Facades{Stdout: io.Discard, Stderr: io.Discard}}
}

// TestExecuteInstallsNoSignalHandlers pins the Execute contract: the caller owns the context and
// the exit, so Execute subscribes to no signal; Main alone subscribes to SIGINT and SIGTERM.
func TestExecuteInstallsNoSignalHandlers(t *testing.T) {
	got := recordSubscriptions(t)

	Execute(context.Background(), quietConfig(), func(context.Context, *Run) error { return nil })
	if len(*got) != 0 {
		t.Fatalf("Execute subscribed to signals %v; it must watch only its context", *got)
	}

	Main(quietConfig(), func(context.Context, *Run) error { return nil })
	if len(*got) != 1 || len((*got)[0]) != 2 || (*got)[0][0] != process.Interrupt || (*got)[0][1] != process.Terminate {
		t.Fatalf("Main subscriptions = %v; want one for Interrupt and Terminate", *got)
	}
}
