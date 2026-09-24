package engine

import (
	"context"
	"io"
	"os"
	"syscall"
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

// DEC-CANCEL-005: FormatExternal is a rendering choice. A 1.1 host that
// renders Snapshot() itself keeps evo's ^C handling, on both construction
// paths.
func TestRun_FormatExternalStillOwnsProcessSignals(t *testing.T) {
	configs := map[string]Config{
		"config":  {Isolated: true, Format: FormatExternal, Stdout: io.Discard, Stderr: io.Discard},
		"options": {Isolated: true, Options: []Option{ExternalProjection(), To(io.Discard)}},
	}
	for name, cfg := range configs {
		t.Run(name, func(t *testing.T) {
			registered := recordSignalRegistration(t)
			out := Init(cfg)
			out.Run(context.Background(), func(context.Context) error { return nil })
			if !registered() {
				t.Fatal("a FormatExternal run stopped handling SIGINT/SIGTERM (1.1 contract)")
			}
		})
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

// captureSignalChannel replaces the notifySignals facade for the duration
// of a test and hands back the channel the run under test registered, so a
// test can deliver as many signals as it needs.
func captureSignalChannel(t *testing.T) (registered <-chan chan<- os.Signal) {
	t.Helper()
	prev := notifySignals
	t.Cleanup(func() { notifySignals = prev })
	delivered := make(chan chan<- os.Signal, 1)
	notifySignals = func(c chan<- os.Signal, _ ...os.Signal) { delivered <- c }
	return delivered
}

// signalSettleWindow is how long a test waits to show that a delivered
// signal did nothing. A run that wrongly acts on it concludes well inside
// this window.
const signalSettleWindow = 100 * time.Millisecond

// signalWindowFormats is every construction a 1.1 host could have used:
// each Format evo renders, FormatExternal, and the raw Options path.
func signalWindowFormats() map[string]Config {
	return map[string]Config{
		"human":            {Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard},
		"data":             {Isolated: true, Format: FormatData, Stdout: io.Discard, Stderr: io.Discard},
		"json":             {Isolated: true, Format: FormatJSON, Stdout: io.Discard, Stderr: io.Discard},
		"jsonl":            {Isolated: true, Format: FormatJSONL, Stdout: io.Discard, Stderr: io.Discard},
		"external":         {Isolated: true, Format: FormatExternal, Stdout: io.Discard, Stderr: io.Discard},
		"external-options": {Isolated: true, Options: []Option{ExternalProjection(), To(io.Discard)}},
	}
}

// Owner rule (ZYS-946, DEC-CANCEL-005): a minor release changes no 1.1
// behavior, CLI formats included. Evo acts on SIGINT/SIGTERM only while
// the run callback runs; once it returns, a signal is caught and ignored,
// so the Define work Finish waits on completes — on every format.
func TestRun_SignalAfterCallbackIsIgnoredOnEveryFormat(t *testing.T) {
	for name, cfg := range signalWindowFormats() {
		t.Run(name, func(t *testing.T) {
			registered := captureSignalChannel(t)
			out := Init(cfg)
			started, release := make(chan struct{}), make(chan struct{})
			result := make(chan Result, 1)
			go func() {
				result <- out.Run(context.Background(), func(context.Context) error {
					out.Task("request").Define(func(ctx context.Context) error {
						close(started)
						select {
						case <-release:
							return nil
						case <-ctx.Done():
							return ctx.Err()
						}
					})
					return nil
				})
			}()

			signals := <-registered
			<-started
			signals <- syscall.SIGTERM
			signals <- syscall.SIGTERM
			select {
			case got := <-result:
				t.Fatalf("a signal after the callback returned ended the run (%s, exit %d); 1.1 let the work finish", got.Conclusion.State, got.ExitCode())
			case <-time.After(signalSettleWindow):
			}
			close(release)

			select {
			case got := <-result:
				if got.Conclusion.State != StateReady || got.ExitCode() != ExitOK {
					t.Fatalf("concluded %s, exit %d; want ready, exit %d (1.1 lifecycle)", got.Conclusion.State, got.ExitCode(), ExitOK)
				}
			case <-time.After(interruptBudget):
				t.Fatal("the run never concluded after its work finished")
			}
		})
	}
}

// The 1.1 window still covers the callback itself: a signal while the run
// callback runs interrupts the run, on every format.
func TestRun_SignalDuringCallbackStopsTheRun(t *testing.T) {
	for name, cfg := range signalWindowFormats() {
		t.Run(name, func(t *testing.T) {
			interrupt := sendOneSignal(t)
			out := Init(cfg)
			code := make(chan int, 1)
			go func() {
				code <- out.Run(context.Background(), func(ctx context.Context) error {
					<-ctx.Done()
					return ctx.Err()
				}).ExitCode()
			}()

			interrupt()
			select {
			case got := <-code:
				if got != ExitCancelled {
					t.Fatalf("exit %d, want %d (ExitCancelled)", got, ExitCancelled)
				}
			case <-time.After(interruptBudget):
				t.Fatal("a signal during the run callback never stopped the run")
			}
		})
	}
}
