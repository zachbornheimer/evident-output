package engine

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
)

var reasonAlreadyCurrent = Reason("already current")

// concludedRunAttempts repeats the signal-after-conclusion race enough
// times that a missing guard fails reliably; each attempt takes well
// under a millisecond.
const concludedRunAttempts = 50

// signalOnDocumentWrite is a FormatJSON Stdout that sends one ^C the
// moment the final "evo.run" document is written — after Finish has
// fixed the Conclusion, before Run hands the Result back.
type signalOnDocumentWrite struct {
	once   sync.Once
	signal func()
}

func (w *signalOnDocumentWrite) Write(p []byte) (int, error) {
	if strings.Contains(string(p), `"evo.run"`) {
		w.once.Do(w.signal)
	}
	return len(p), nil
}

// A ^C that arrives after the run concluded has nothing left to stop. It
// must not rewrite the Output's state: the Result Run returns, the
// Output's own Conclusion, and its cancellation bookkeeping all keep
// agreeing that the run succeeded.
func TestRun_SignalAfterConclusionLeavesTheResult(t *testing.T) {
	for range concludedRunAttempts {
		interrupt := sendOneSignal(t)
		out := Init(Config{Isolated: true, Plain: true, Format: FormatJSON,
			Stdout: &signalOnDocumentWrite{signal: interrupt}, Stderr: io.Discard})
		result := out.Run(context.Background(), func(context.Context) error {
			out.Task("install agent").Define(func(context.Context) error { return nil })
			return nil
		})

		if result.ExitCode() != ExitOK {
			t.Fatalf("exit %d, want %d: the run finished before the signal", result.ExitCode(), ExitOK)
		}
		if got := out.Conclusion(); got.State != result.Conclusion.State || got.ExitCode != result.Conclusion.ExitCode {
			t.Fatalf("Output concluded %s/%d but Run returned %s/%d", got.State, got.ExitCode, result.Conclusion.State, result.Conclusion.ExitCode)
		}
		out.mu.Lock()
		cancelled, cause := out.schedCancelled, out.cancelledBy.cause
		out.mu.Unlock()
		if cancelled || cause != "" {
			t.Fatalf("a signal after conclusion rewrote the concluded Output (schedCancelled=%v, cause=%q)", cancelled, cause)
		}
	}
}

// The same guard, without the scheduling race: interrupting an Output
// that already finished changes nothing.
func TestInterrupt_AfterConclusionIsANoOp(t *testing.T) {
	out := Init(Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard})
	out.beginRunContext(context.Background())
	out.Task("install agent").Kept(reasonAlreadyCurrent)
	_ = out.Finish()
	before := out.Conclusion()

	out.interrupt(interruptionBySignal)

	out.mu.Lock()
	cancelled, cause := out.schedCancelled, out.cancelledBy.cause
	out.mu.Unlock()
	if cancelled || cause != "" {
		t.Fatalf("interrupt rewrote a finished Output (schedCancelled=%v, cause=%q)", cancelled, cause)
	}
	if after := out.Conclusion(); after.State != before.State || after.ExitCode != before.ExitCode {
		t.Fatalf("Conclusion moved from %s/%d to %s/%d", before.State, before.ExitCode, after.State, after.ExitCode)
	}
}
