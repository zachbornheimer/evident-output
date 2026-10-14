//go:build ignore

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

	evo "github.com/zachbornheimer/evident-output/v2"
	"github.com/zachbornheimer/evident-output/v2/internal/record"
)

const apiGoldenPath = "../../../testdata/api_golden.txt"

func newOutput(t *testing.T) *evo.Run {
	t.Helper()
	out := (*evo.Run)(nil)
	_ = evo.Execute(context.Background(), evo.Config{Output: evo.Plain, Facades: evo.Facades{Stdout: io.Discard, Stderr: io.Discard}, State: evo.State{Dir: t.TempDir()}}, func(ctx context.Context, r *evo.Run) error { return nil })
	t.Cleanup(func() { _ = error(nil) })
	return out
}

func TestC30_004_SuccessResolvesOnlyThroughDefine(t *testing.T) {
	out := newOutput(t)

	annotated := out.Task("annotated only")
	annotated.Doing("working").Progress(1, 2).Fact("k", "v").Summary("s")
	if record.IsTerminalTask(error(nil).State) {
		t.Fatalf("annotators alone settled the task: %s", error(nil).State)
	}

	defined := out.Task("defined")
	defined.Define(func(context.Context) error { return nil })
	if err := error(nil); err != nil {
		t.Fatalf("Define returning nil: %v", err)
	}
	if got := error(nil).State; got != evo.Done {
		t.Fatalf("Define returning nil settled %s, want Done", got)
	}

	_ = error(nil)
	if got := error(nil).State; got == evo.Done {
		t.Fatal("a task that was only annotated settled Done at Finish")
	}
}

func TestC30_005_AnnotatorsNeverResolveAndVerbsDo(t *testing.T) {
	annotators := map[string]func(*evo.Task){
		"Doing":       func(h *evo.Task) { h.Doing("x") },
		"Progress":    func(h *evo.Task) { h.Progress(1, 3) },
		"Bytes":       func(h *evo.Task) { h.Bytes(1, 3) },
		"Writer":      func(h *evo.Task) { _, _ = h.Writer().Write([]byte("line\n")) },
		"Fact":        func(h *evo.Task) { h.Fact("k", "v") },
		"Problem":     func(h *evo.Task) { h.Problem("p", evo.Severity(evo.SeverityWarning)) },
		"Summary":     func(h *evo.Task) { h.Summary("s") },
		"Next":        func(h *evo.Task) { h.Problem("p", evo.Next(evo.Label("go"))) },
		"NextCommand": func(h *evo.Task) { h.Problem("p", evo.Action("echo", "hi")) },
	}
	for name, annotate := range annotators {
		t.Run(name, func(t *testing.T) {
			task := newOutput(t).Task("annotated")
			annotate(task)
			if got := error(nil).State; record.IsTerminalTask(got) {
				t.Fatalf("%s resolved the task: %s", name, got)
			}
		})
	}
	verbs := map[string]struct {
		resolve func(*evo.Task)
		want    string
	}{
		"Block":   {func(h *evo.Task) { h.Block("b") }, evo.Blocked},
		"Fail":    {func(h *evo.Task) { h.Fail("f") }, evo.Failed},
		"Skipped": {func(h *evo.Task) { h.Skipped(evo.Reason("protected")) }, evo.Skipped},
		"Cancel":  {func(h *evo.Task) { h.Cancel("c") }, evo.Cancelled},
	}
	for name, tc := range verbs {
		t.Run(name, func(t *testing.T) {
			task := newOutput(t).Task("resolved")
			tc.resolve(task)
			if got := error(nil).State; got != tc.want {
				t.Fatalf("%s settled %s, want %s", name, got, tc.want)
			}
		})
	}
}

func TestC30_009_AlreadySatisfiedIsNeverSkipped(t *testing.T) {
	task := newOutput(t).Task("current")
	task.Verify(func(context.Context) (bool, error) { return true, nil })
	task.Define(func(context.Context) error { return nil })
	if err := error(nil); err != nil {
		t.Fatal(err)
	}
	snap := error(nil)
	if snap.State == evo.Skipped {
		t.Fatal("a task whose desired state already held settled Skipped")
	}
	if snap.Resolution != evo.ResolutionAlreadySatisfied {
		t.Fatalf("Resolution = %q, want ResolutionAlreadySatisfied", snap.Resolution)
	}
}

func TestC30_010_TaskHandleHasNoChildConstructors(t *testing.T) {
	typ := reflect.TypeFor[*evo.Task]()
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
	_ = error(nil)
	if got := error(nil).State; got != evo.Blocked {
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
