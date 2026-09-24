package engine

import (
	"context"
	"io"
	"os"
	"testing"
	"time"
)

// recordSignalRegistration replaces the notifySignals facade for the
// duration of a test and reports whether any run registered a handler.
func recordSignalRegistration(t *testing.T) (registered func() bool) {
	t.Helper()
	prev := notifySignals
	t.Cleanup(func() { notifySignals = prev })
	calls := 0
	notifySignals = func(chan<- os.Signal, ...os.Signal) { calls++ }
	return func() bool { return calls > 0 }
}

// Spec §53 (ZYS-946): an embedded FormatExternal Output belongs to a host
// (an HTTP server) that owns process signals — SIGTERM there means
// graceful shutdown, which must let in-flight requests finish. Cancellation
// reaches the run only through the caller's context.
func TestRun_ExternalProjectionLeavesProcessSignalsToHost(t *testing.T) {
	registered := recordSignalRegistration(t)
	out := Init(Config{Isolated: true, Format: FormatExternal, Stdout: io.Discard, Stderr: io.Discard})
	out.Run(context.Background(), func(context.Context) error { return nil })
	if registered() {
		t.Fatal("a FormatExternal run registered SIGINT/SIGTERM handlers; the embedding host owns process signals")
	}
}

// The CLI forms keep owning ^C: an Isolated human-format Output still
// wires SIGINT/SIGTERM exactly as before.
func TestRun_HumanFormatStillOwnsProcessSignals(t *testing.T) {
	registered := recordSignalRegistration(t)
	out := Init(Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard})
	out.Run(context.Background(), func(context.Context) error { return nil })
	if !registered() {
		t.Fatal("an Isolated human-format run no longer registers SIGINT/SIGTERM handlers")
	}
}

// The ordinary evo shape declares Tasks and returns; the Define work then
// executes while Run finishes. A ^C in that window must still stop the
// run — previously the signal was only watched until the run callback
// returned, so a long Define (a server, a slow install) ignored it.
func TestRun_SignalDuringFinishStopsDeclaredWork(t *testing.T) {
	interrupt := sendOneSignal(t)
	out := Init(Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard})
	started := make(chan struct{})
	code := make(chan int, 1)
	go func() {
		code <- out.Run(context.Background(), func(context.Context) error {
			out.Task("serve").Define(func(ctx context.Context) error {
				close(started)
				<-ctx.Done()
				return nil
			})
			return nil
		}).ExitCode()
	}()

	<-started
	interrupt()
	select {
	case got := <-code:
		if got != ExitCancelled {
			t.Fatalf("exit %d, want %d (ExitCancelled)", got, ExitCancelled)
		}
	case <-time.After(interruptBudget):
		t.Fatal("a signal arriving after the run callback returned never stopped the declared work")
	}
}
