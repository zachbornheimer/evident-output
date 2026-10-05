// Package lifecycle_test binds contract §30 "Task lifecycle" and
// "Problems and warnings" rules to the public evo API.
package lifecycle_test

import (
	"context"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/core"
)

const apiGoldenPath = "../../../testdata/api_golden.txt"

func newOutput(t *testing.T) *evo.Output {
	t.Helper()
	out := evo.Init(evo.Config{
		Isolated: true, Plain: true, Color: evo.ColorNever,
		StateDir: t.TempDir(), Stdout: io.Discard, Stderr: io.Discard,
	})
	t.Cleanup(func() { _ = out.Close() })
	return out
}

func TestC30_004_SuccessResolvesOnlyThroughDefine(t *testing.T) {
	out := newOutput(t)

	annotated := out.Task("annotated only")
	annotated.Doing("working").Progress(1, 2).Fact("k", "v").Summary("s")
	if core.IsTerminalTask(annotated.Snapshot().State) {
		t.Fatalf("annotators alone settled the task: %s", annotated.Snapshot().State)
	}

	defined := out.Task("defined")
	defined.Define(func(context.Context) error { return nil })
	if err := defined.Wait(); err != nil {
		t.Fatalf("Define returning nil: %v", err)
	}
	if got := defined.Snapshot().State; got != evo.Done {
		t.Fatalf("Define returning nil settled %s, want Done", got)
	}

	_ = out.Finish()
	if got := annotated.Snapshot().State; got == evo.Done {
		t.Fatal("a task that was only annotated settled Done at Finish")
	}
}

func TestC30_005_AnnotatorsNeverResolveAndVerbsDo(t *testing.T) {
	annotators := map[string]func(*evo.TaskHandle){
		"Doing":       func(h *evo.TaskHandle) { h.Doing("x") },
		"Progress":    func(h *evo.TaskHandle) { h.Progress(1, 3) },
		"Bytes":       func(h *evo.TaskHandle) { h.Bytes(1, 3) },
		"Writer":      func(h *evo.TaskHandle) { _, _ = h.Writer().Write([]byte("line\n")) },
		"Fact":        func(h *evo.TaskHandle) { h.Fact("k", "v") },
		"Problem":     func(h *evo.TaskHandle) { h.Problem("p", evo.Severity(evo.SeverityWarning)) },
		"Summary":     func(h *evo.TaskHandle) { h.Summary("s") },
		"Next":        func(h *evo.TaskHandle) { h.Problem("p", evo.Next(evo.Label("go"))) },
		"NextCommand": func(h *evo.TaskHandle) { h.Problem("p", evo.NextCommand("echo", "hi")) },
	}
	for name, annotate := range annotators {
		t.Run(name, func(t *testing.T) {
			task := newOutput(t).Task("annotated")
			annotate(task)
			if got := task.Snapshot().State; core.IsTerminalTask(got) {
				t.Fatalf("%s resolved the task: %s", name, got)
			}
		})
	}
	verbs := map[string]struct {
		resolve func(*evo.TaskHandle)
		want    evo.EntityState
	}{
		"Block":   {func(h *evo.TaskHandle) { h.Block("b") }, evo.Blocked},
		"Fail":    {func(h *evo.TaskHandle) { h.Fail("f") }, evo.Failed},
		"Skipped": {func(h *evo.TaskHandle) { h.Skipped(evo.Reason("protected")) }, evo.Skipped},
		"Cancel":  {func(h *evo.TaskHandle) { h.Cancel("c") }, evo.Cancelled},
	}
	for name, tc := range verbs {
		t.Run(name, func(t *testing.T) {
			task := newOutput(t).Task("resolved")
			tc.resolve(task)
			if got := task.Snapshot().State; got != tc.want {
				t.Fatalf("%s settled %s, want %s", name, got, tc.want)
			}
		})
	}
}

func TestC30_009_AlreadySatisfiedIsNeverSkipped(t *testing.T) {
	task := newOutput(t).Task("current")
	task.Verify(func(context.Context) (bool, error) { return true, nil })
	task.Define(func(context.Context) error { return nil })
	if err := task.Wait(); err != nil {
		t.Fatal(err)
	}
	snap := task.Snapshot()
	if snap.State == evo.Skipped {
		t.Fatal("a task whose desired state already held settled Skipped")
	}
	if snap.Resolution != evo.ResolutionAlreadySatisfied {
		t.Fatalf("Resolution = %q, want ResolutionAlreadySatisfied", snap.Resolution)
	}
}

func TestC30_010_TaskHandleHasNoChildConstructors(t *testing.T) {
	typ := reflect.TypeFor[*evo.TaskHandle]()
	for _, name := range []string{"Task", "Group", "Sequence"} {
		if _, found := typ.MethodByName(name); found {
			t.Errorf("TaskHandle has child constructor %s", name)
		}
	}
}

func TestC30_035_CollectionWithBlockedChildSnapshotsBlocked(t *testing.T) {
	out := newOutput(t)
	group := out.Group("checks")
	group.Task("ok").Define(func(context.Context) error { return nil })
	group.Task("stuck").Block("cannot proceed")
	_ = group.Wait()
	if got := group.Snapshot().State; got != evo.Blocked {
		t.Fatalf("group with a Blocked child snapshots %s, want Blocked", got)
	}
}

func apiGolden(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(apiGoldenPath)
	if err != nil {
		t.Fatalf("read exported API golden: %v", err)
	}
	return string(raw)
}

func TestC30_039_NoWarnCall(t *testing.T) {
	for line := range strings.SplitSeq(apiGolden(t), "\n") {
		if strings.Contains(line, ") Warn(") || strings.HasPrefix(line, "func Warn(") {
			t.Errorf("exported API has a Warn call: %s", line)
		}
	}
}

func TestC30_042_NoSecondFindingType(t *testing.T) {
	for line := range strings.SplitSeq(apiGolden(t), "\n") {
		for _, banned := range []string{"type Finding", "type Warning", "type Diagnostic", "type Issue"} {
			if strings.HasPrefix(line, banned) {
				t.Errorf("exported API declares a second finding type: %s", line)
			}
		}
	}
}
