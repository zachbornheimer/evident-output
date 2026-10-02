package outcomes_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/conformance/contract/harness"
)

// concluded runs one task whose Define is built by define, finishes the run,
// and returns the Output plus the task's settled snapshot.
func concluded(t *testing.T, define func(task *evo.TaskHandle)) (*evo.Output, *bytes.Buffer, evo.TaskSnapshot) {
	t.Helper()
	out, buf := harness.New(t)
	task := out.Task("job")
	define(task)
	_ = task.Wait()
	_ = out.Finish()
	return out, buf, harness.MustFind(t, out, "job")
}

func returning(err error) func(*evo.TaskHandle) {
	return func(task *evo.TaskHandle) {
		task.Define(func(context.Context) error { return err })
	}
}

func TestC01_001_OneTruthDrivesEveryProjection(t *testing.T) {
	out, buf := harness.New(t)
	res := out.Run(context.Background(), func(context.Context) error {
		return out.Task("build").Define(func(context.Context) error { return errors.New("boom") }).Wait()
	})
	var js bytes.Buffer
	if err := evo.WriteJSON(&js, res); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Outcome  string `json:"outcome"`
		ExitCode int    `json:"exit_code"`
	}
	if err := json.Unmarshal(js.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	if res.Conclusion.State != evo.StateFailed || res.ExitCode() != evo.ExitFailed {
		t.Fatalf("conclusion %v exit %d", res.Conclusion.State, res.ExitCode())
	}
	if doc.Outcome != "failed" || doc.ExitCode != evo.ExitFailed {
		t.Fatalf("json projection disagrees: %+v", doc)
	}
	if !strings.Contains(buf.String(), "[failed") {
		t.Fatalf("human projection disagrees:\n%s", buf.String())
	}
}

func TestC01_002_FalseAlreadySatisfiedNeverReported(t *testing.T) {
	ran := false
	_, _, task := concluded(t, func(task *evo.TaskHandle) {
		task.Verify(func(context.Context) (bool, error) { return false, nil })
		task.Define(func(context.Context) error { ran = true; return nil })
	})
	if !ran {
		t.Fatal("a false before-check must run the callback")
	}
	if task.State != evo.Failed || task.Resolution == evo.ResolutionAlreadySatisfied {
		t.Fatalf("unproven state reported as %v/%v", task.State, task.Resolution)
	}
}

func TestC02_001_OutcomesAreRuntimeSemantics(t *testing.T) {
	_, _, succeeded := concluded(t, returning(nil))
	if succeeded.State != evo.Done || succeeded.Resolution != evo.ResolutionExecuted {
		t.Fatalf("Succeeded = %v/%v", succeeded.State, succeeded.Resolution)
	}
	_, _, satisfied := concluded(t, func(task *evo.TaskHandle) {
		task.Verify(func(context.Context) (bool, error) { return true, nil })
		task.Define(func(context.Context) error { return errors.New("must not run") })
	})
	if satisfied.State != evo.Done || satisfied.Resolution != evo.ResolutionAlreadySatisfied {
		t.Fatalf("AlreadySatisfied = %v/%v", satisfied.State, satisfied.Resolution)
	}
	_, _, skipped := concluded(t, func(task *evo.TaskHandle) {
		task.Define(func(context.Context) error { task.Skipped(evo.Reason("protected")); return nil })
	})
	if skipped.State != evo.Skipped {
		t.Fatalf("Skipped = %v", skipped.State)
	}
	_, _, blocked := concluded(t, func(task *evo.TaskHandle) {
		task.Define(func(context.Context) error { task.Block("refused"); return nil })
	})
	if blocked.State != evo.Blocked {
		t.Fatalf("Blocked = %v", blocked.State)
	}
	_, _, failed := concluded(t, returning(errors.New("boom")))
	if failed.State != evo.Failed {
		t.Fatalf("Failed = %v", failed.State)
	}
}

func TestC02_002_ConclusionIsOKBlockedOrFailed(t *testing.T) {
	ok, _, _ := concluded(t, returning(nil))
	blocked, _, _ := concluded(t, func(task *evo.TaskHandle) {
		task.Define(func(context.Context) error { task.Block("refused"); return nil })
	})
	failed, _, _ := concluded(t, returning(errors.New("boom")))
	want := []struct {
		out  *evo.Output
		exit int
	}{{ok, evo.ExitOK}, {blocked, evo.ExitBlocked}, {failed, evo.ExitFailed}}
	for _, w := range want {
		if got := w.out.Conclusion().ExitCode; got != w.exit {
			t.Fatalf("exit = %d, want %d (state %v)", got, w.exit, w.out.Conclusion().State)
		}
	}
}

func TestC02_003_BlockAndFailAreDifferent(t *testing.T) {
	out, _, task := concluded(t, func(task *evo.TaskHandle) {
		task.Define(func(context.Context) error { task.Block("policy refused"); return nil })
	})
	if task.State != evo.Blocked || out.Conclusion().ExitCode != evo.ExitBlocked {
		t.Fatalf("Block: %v exit %d", task.State, out.Conclusion().ExitCode)
	}
	out, _, task = concluded(t, func(task *evo.TaskHandle) {
		task.Define(func(context.Context) error { task.Fail("i/o failed"); return nil })
	})
	if task.State != evo.Failed || out.Conclusion().ExitCode != evo.ExitFailed {
		t.Fatalf("Fail: %v exit %d", task.State, out.Conclusion().ExitCode)
	}
}

func TestC02_004_ErrorProblemFailsDefineWarningDoesNot(t *testing.T) {
	_, _, errored := concluded(t, func(task *evo.TaskHandle) {
		task.Define(func(context.Context) error { task.Problem("invalid configuration"); return nil })
	})
	if errored.State != evo.Failed {
		t.Fatalf("error Problem left task %v, want Failed", errored.State)
	}
	_, _, warned := concluded(t, func(task *evo.TaskHandle) {
		task.Define(func(context.Context) error {
			task.Problem("tool version differs", evo.Severity(evo.SeverityWarning))
			return nil
		})
	})
	if warned.State != evo.Done {
		t.Fatalf("warning Problem left task %v, want Done", warned.State)
	}
}

func TestC02_005_WarningSetsWarnedAndNeverChangesExit(t *testing.T) {
	out, _, _ := concluded(t, func(task *evo.TaskHandle) {
		task.Define(func(context.Context) error {
			task.Problem("drift", evo.Severity(evo.SeverityWarning))
			return nil
		})
	})
	c := out.Conclusion()
	if !c.Warned || c.ExitCode != evo.ExitOK {
		t.Fatalf("warned=%v exit=%d", c.Warned, c.ExitCode)
	}
}

func TestC02_006_ProgressAndDoingAreOrthogonal(t *testing.T) {
	var during evo.TaskSnapshot
	concluded(t, func(task *evo.TaskHandle) {
		task.Define(func(context.Context) error {
			task.Progress(3, 10).Doing("urllib3")
			during = task.Snapshot()
			return nil
		})
	})
	if during.Progress.Completed != 3 || during.Progress.Total != 10 || during.Phase != "urllib3" {
		t.Fatalf("progress %+v activity %q", during.Progress, during.Phase)
	}
}

func TestC02_007_SummaryResolvesNothing(t *testing.T) {
	out, _ := harness.New(t)
	task := out.Task("check")
	task.Summary("459 checked")
	if got := harness.MustFind(t, out, "check").State; got != evo.Pending {
		t.Fatalf("Summary alone resolved the task: %v", got)
	}
	if err := harness.Succeed(task); err != nil {
		t.Fatal(err)
	}
	done := harness.MustFind(t, out, "check")
	if done.State != evo.Done || done.Summary != "459 checked" {
		t.Fatalf("after Define: %v %q", done.State, done.Summary)
	}
}

func TestC02_008_SkippedIsAResolutionNotSatisfaction(t *testing.T) {
	out, buf, task := concluded(t, func(task *evo.TaskHandle) {
		task.Define(func(context.Context) error { task.Skipped(evo.Reason("protected")); return nil })
	})
	if task.State != evo.Skipped || task.Resolution == evo.ResolutionAlreadySatisfied {
		t.Fatalf("skipped task = %v/%v", task.State, task.Resolution)
	}
	if out.Conclusion().Warned || !strings.Contains(buf.String(), "- skipped 1 (protected)") {
		t.Fatalf("skipped rendering:\n%s", buf.String())
	}
}

func TestC02_009_RunScopeFailFailsTheRun(t *testing.T) {
	out, _ := harness.New(t)
	out.Fail("cannot open repository")
	_ = out.Finish()
	c := out.Conclusion()
	if c.State != evo.StateFailed || c.ExitCode != evo.ExitFailed {
		t.Fatalf("run-level Fail concluded %v exit %d", c.State, c.ExitCode)
	}
}
