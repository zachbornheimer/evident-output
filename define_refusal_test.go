package evo_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// errAmbiguous is the refusal cause these tests carry.
var errAmbiguous = errors.New("ambiguous")

// refusalOutput is an isolated plain Output for the refusal tests.
func refusalOutput(t *testing.T, buf *bytes.Buffer) *evo.Output {
	t.Helper()
	return evo.Init(evo.Config{Isolated: true, StateDir: t.TempDir(), Stdout: buf, Title: "refuse", Color: evo.ColorNever, Plain: true})
}

// TestDefine_BlockRefuses pins the one way a Define callback refuses in 1.1
// (Blockf is removed): Block with its Action as a ProblemOption, then
// return. The Task concludes Blocked (exit 1), the Action reaches the
// run's next steps, Wait reports the work did not succeed, and a later
// Sequence step never starts, exactly as the removed `return
// task.Blockf(...)` did.
func TestDefine_BlockRefuses(t *testing.T) {
	var buf bytes.Buffer
	out := refusalOutput(t, &buf)
	steps := out.Sequence("converge")
	gate := steps.Task("gate")
	gate.Define(func(context.Context) error {
		gate.Block("needs review", evo.Detail("ambiguous changes need a human decision before merge"), evo.NextCommand("git", "status"))
		return errAmbiguous
	})
	apply := steps.Task("apply")
	ran := false
	apply.Define(func(context.Context) error { ran = true; return nil })
	if err := gate.Wait(); err == nil {
		t.Fatal("gate.Wait() = nil, want an error: a Blocked Task did not succeed")
	}
	_ = out.Finish()
	if c := out.Conclusion(); c.State != evo.StateBlocked || c.ExitCode != evo.ExitBlocked {
		t.Fatalf("conclusion = %s exit %d, want %s exit %d\n%s", c.State, c.ExitCode, evo.StateBlocked, evo.ExitBlocked, buf.String())
	}
	if ran {
		t.Fatalf("a Sequence step after a Blocked step ran\n%s", buf.String())
	}
	if got := apply.Snapshot().State; got != evo.NotStarted {
		t.Fatalf("follower state = %s, want %s\n%s", got, evo.NotStarted, buf.String())
	}
	for _, want := range []string{"needs review", "human decision", "git status"} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("output missing %q\n%s", want, buf.String())
		}
	}
	_ = out.Close()
}

// TestDefine_ReturnedErrorFailsWithCause pins the one way a Define callback
// fails with a cause: return the error. The Task concludes Failed and Wait
// keeps the cause reachable through errors.Is.
func TestDefine_ReturnedErrorFailsWithCause(t *testing.T) {
	var buf bytes.Buffer
	out := refusalOutput(t, &buf)
	task := out.Task("clone")
	task.Define(func(context.Context) error {
		return fmt.Errorf("clone repo: %w", errAmbiguous)
	})
	if err := task.Wait(); !errors.Is(err, errAmbiguous) {
		t.Fatalf("Wait() = %v, want errors.Is(err, errAmbiguous)", err)
	}
	_ = out.Finish()
	if c := out.Conclusion(); c.State != evo.StateFailed {
		t.Fatalf("conclusion = %s, want %s\n%s", c.State, evo.StateFailed, buf.String())
	}
	_ = out.Close()
}

// TestFail_NextOptionReachesNextSteps pins the canonical Action path for a
// Fail: the Next ProblemOption, not a chained return value.
func TestFail_NextOptionReachesNextSteps(t *testing.T) {
	var buf bytes.Buffer
	out := refusalOutput(t, &buf)
	out.Task("network").Fail("offline", evo.Next(evo.Label("check network access")))
	_ = out.Finish()
	if !strings.Contains(buf.String(), "check network access") {
		t.Fatalf("output missing the Fail's Next action\n%s", buf.String())
	}
	_ = out.Close()
}

// TestResolution_NoFormattedForms pins the 1.1 removal of the formatted
// resolution verbs: Block and Fail take a summary and ProblemOptions, and
// nothing returns a chainable failure value.
func TestResolution_NoFormattedForms(t *testing.T) {
	for _, c := range []struct {
		recv   reflect.Type
		method string
	}{
		{reflect.TypeFor[*evo.TaskHandle](), "Blockf"},
		{reflect.TypeFor[*evo.TaskHandle](), "Failf"},
		{reflect.TypeFor[*evo.Output](), "Failf"},
	} {
		if _, ok := c.recv.MethodByName(c.method); ok {
			t.Errorf("%s.%s exists; it is removed in 1.1", c.recv.Elem().Name(), c.method)
		}
	}
}
