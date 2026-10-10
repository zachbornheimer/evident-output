package engine

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const strictStallChildEnv = "EVO_STRICT_STALL_CHILD"

// strictStallSurvived is what the child prints when the run outlived the
// strict misuse the stall recorded.
const strictStallSurvived = "survived"

// A strict run panics at a misuse. The stall below is found by the pooled
// worker that finishes the last running Task, a goroutine with no caller to
// take a panic, so the process must not die: the misuse is recorded, the
// waiter is released with ErrWaitDeadlock, and Finish reports it.
func TestSlice36_AStrictStallFoundByAFinishingWorkerDoesNotKillTheProcess(t *testing.T) {
	if os.Getenv(strictStallChildEnv) == "1" {
		runStrictStallFoundByFinishingWorker(t)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
	cmd.Env = append(os.Environ(), strictStallChildEnv+"=1")
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), strictStallSurvived) {
		t.Fatalf("child run died: err=%v\n%s", err, output)
	}
}

func runStrictStallFoundByFinishingWorker(t *testing.T) {
	out := newOutput("job", to(&bytes.Buffer{}), maxConcurrency(4), strict())
	a := out.Task("a")
	c := out.Task("c").After(a).Define(noop)
	out.Task("d").Define(func(context.Context) error { time.Sleep(50 * time.Millisecond); return nil })
	a.Define(func(context.Context) error { return c.Wait() })

	waitErr, returned := within(5*time.Second, a.Wait)

	if !returned || !errors.Is(waitErr, ErrWaitDeadlock) {
		t.Fatalf("a.Wait returned=%v err=%v, want ErrWaitDeadlock", returned, waitErr)
	}
	if waits := out.graph.Waits(); waits != 0 {
		t.Errorf("parked waits = %d, want 0", waits)
	}
	if !errors.Is(out.firstMisuse(), ErrWaitDeadlock) {
		t.Errorf("misuse = %v, want ErrWaitDeadlock", out.firstMisuse())
	}
	t.Log(strictStallSurvived)
}
