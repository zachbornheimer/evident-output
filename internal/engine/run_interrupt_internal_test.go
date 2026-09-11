package engine

import (
	"io"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// interruptBudget bounds a run that a single ^C is supposed to stop. It is a
// hang detector, not a timing dependency.
const interruptBudget = 5 * time.Second

// sendOneSignal replaces the notifySignals facade for the duration of a test
// and hands back the trigger that delivers exactly one SIGINT to the run
// under test — the whole signal path without a real process signal.
func sendOneSignal(t *testing.T) (interrupt func()) {
	t.Helper()
	prev := notifySignals
	t.Cleanup(func() { notifySignals = prev })
	delivered := make(chan chan<- os.Signal, 1)
	notifySignals = func(c chan<- os.Signal, _ ...os.Signal) { delivered <- c }
	return func() {
		select {
		case c := <-delivered:
			c <- syscall.SIGINT
		case <-time.After(interruptBudget):
			t.Error("run never registered a signal handler")
		}
	}
}

// TestRun_SingleInterrupt_CancelsRunningAndAbandonsTheQueue pins the user-visible contract of one
// ^C (spec: Ctrl-C, "early termination"): work already finished stays
// finished, the row that was running says it was cancelled, everything
// queued behind it says it never started, and the exit code is 130. Before
// this, one signal cancelled a single row while the scheduler kept
// dispatching the rest of the run to completion.
func TestRun_SingleInterrupt_CancelsRunningAndAbandonsTheQueue(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{
		Isolated: true, Plain: true, Color: ColorNever,
		MaxConcurrency: 1, Stdout: &buf, Stderr: io.Discard,
	})
	interrupt := sendOneSignal(t)

	blocking := make(chan struct{})
	group := out.Group("install")
	names := []string{"pkg1", "pkg2", "pkg3", "pkg4", "pkg5"}
	code := make(chan int, 1)
	go func() {
		code <- out.Run(func(o *Output) error {
			for name, task := range group.Each(names) {
				first := name == names[0]
				task.Define(func() error {
					if first {
						close(blocking)
						<-task.Context().Done()
					}
					return nil
				})
			}
			return nil
		})
	}()

	<-blocking
	interrupt()

	var got int
	select {
	case got = <-code:
	case <-time.After(interruptBudget):
		t.Fatalf("one interrupt did not stop the run; output so far:\n%s", buf.String())
	}

	if got != ExitCancelled {
		t.Fatalf("exit %d, want %d (ExitCancelled); output:\n%s", got, ExitCancelled, buf.String())
	}
	rendered := buf.String()
	// One child was running, so exactly one child row says cancelled. (The
	// collection's own header carries the same glyph because the collection
	// is cancelled — that is its derived state, not a second claim about
	// work.)
	if n := strings.Count(rendered, "■ pkg"); n != 1 {
		t.Fatalf("want exactly one cancelled child row, got %d:\n%s", n, rendered)
	}
	if !strings.Contains(rendered, "4 not started") {
		t.Fatalf("want the queued children accounted for as not started:\n%s", rendered)
	}
	if strings.Contains(rendered, "✓") {
		t.Fatalf("nothing may complete after the cancel:\n%s", rendered)
	}
	if got := out.Conclusion().State; got != StateCancelled {
		t.Fatalf("conclusion = %v, want StateCancelled", got)
	}
}

// TestRun_Interrupt_CancelPreservesCompletedWorkAndCommittedEffects pins the rest
// of the spec's early-termination block: a task that already finished keeps
// its ✓ row, and the effects it committed are named on the "! already
// mutated: ..." line rather than being lost with the run.
func TestRun_Interrupt_CancelPreservesCompletedWorkAndCommittedEffects(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{
		Isolated: true, Plain: true, Color: ColorNever,
		MaxConcurrency: 1, Stdout: &buf, Stderr: io.Discard,
	})
	interrupt := sendOneSignal(t)

	seq := out.Sequence("python")
	scan := seq.Task("scan")
	venv := seq.Task("venv")
	install := seq.Task("install")

	blocking := make(chan struct{})
	code := make(chan int, 1)
	go func() {
		code <- out.Run(func(o *Output) error {
			scan.Define(func() error { return nil })
			venv.Create(".venv directory", func() error {
				close(blocking)
				<-venv.Context().Done()
				return nil
			})
			install.Define(func() error { return nil })
			return install.Wait()
		})
	}()

	<-blocking
	interrupt()

	select {
	case got := <-code:
		if got != ExitCancelled {
			t.Fatalf("exit %d, want %d; output:\n%s", got, ExitCancelled, buf.String())
		}
	case <-time.After(interruptBudget):
		t.Fatalf("one interrupt did not stop the run; output so far:\n%s", buf.String())
	}

	rendered := buf.String()
	for _, want := range []string{"✓ scan", "■ venv", "- install", "already mutated: 1 .venv directory created"} {
		if !strings.Contains(strings.Join(strings.Fields(rendered), " "), strings.Join(strings.Fields(want), " ")) {
			t.Fatalf("want %q in:\n%s", want, rendered)
		}
	}
}
