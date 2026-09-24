package engine

import (
	"context"
	"io"
	"testing"
)

var reasonAlreadyCurrent = Reason("already current")

// settledRun is an embedded Output whose run callback has returned with its
// one Task already complete — the moment between the last Task finishing
// and the run reading its Conclusion.
func settledRun(t *testing.T, callerEndedAtReturn bool) *Output {
	t.Helper()
	out := Init(Config{Isolated: true, Embedded: true, Format: FormatExternal, Stdout: io.Discard, Stderr: io.Discard})
	out.beginRunContext(context.Background())
	out.Task("install agent").Kept(reasonAlreadyCurrent)
	out.endRunCallback(func() bool { return callerEndedAtReturn })
	return out
}

// A caller whose ctx ends after every Task finished has cancelled nothing:
// the completed run keeps its own verdict instead of flipping to 130.
func TestInterrupt_AfterEveryTaskFinishedLeavesVerdict(t *testing.T) {
	out := settledRun(t, false)
	out.interrupt(interruptionByCaller)
	_ = out.Finish()
	if c := out.Conclusion(); c.State == StateCancelled || c.ExitCode == ExitCancelled {
		t.Fatalf("completed run concluded %s/%d after a late caller cancel, want its own verdict", c.State, c.ExitCode)
	}
}

// A caller ctx already ended when the run callback returned is a cancelled
// request, even when no Task was left running.
func TestInterrupt_CallerEndedBeforeRunReturnedStillCancels(t *testing.T) {
	out := settledRun(t, true)
	out.interrupt(interruptionByCaller)
	_ = out.Finish()
	if c := out.Conclusion(); c.State != StateCancelled || c.ExitCode != ExitCancelled {
		t.Fatalf("run concluded %s/%d, want %s/%d", c.State, c.ExitCode, StateCancelled, ExitCancelled)
	}
}

// A CLI run owns ^C for its whole lifetime (DEC-CANCEL-004 is scoped to
// embedded runs): a ^C that lands after the run callback returned and
// every Task finished still stops the run, as the person at the terminal
// asked. Only a caller's context gets the "nothing left to stop" rule.
func TestInterrupt_SignalAfterEveryTaskFinishedStillCancelsCLIRun(t *testing.T) {
	out := Init(Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard})
	out.beginRunContext(context.Background())
	out.Task("install agent").Kept(reasonAlreadyCurrent)
	out.endRunCallback(inertCallerWatch.ended)
	out.interrupt(interruptionBySignal)
	_ = out.Finish()
	if c := out.Conclusion(); c.State != StateCancelled || c.ExitCode != ExitCancelled {
		t.Fatalf("CLI run concluded %s/%d after ^C, want %s/%d", c.State, c.ExitCode, StateCancelled, ExitCancelled)
	}
}
