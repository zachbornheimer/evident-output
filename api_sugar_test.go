package evo

import (
	"errors"
	"fmt"
	"io"
	"testing"
)

// --- Item 1: printf-variadic entity names ---

func TestAPISugar_TaskNameIsPrintfWhenArgsPresent(t *testing.T) {
	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task(fmt.Sprintf("build %s #%d", "worker", 3))
	if got := task.snapshot().Name; got != "build worker #3" {
		t.Fatalf("name = %q, want %q", got, "build worker #3")
	}
}

func TestAPISugar_TaskNameUnchangedWithoutArgs(t *testing.T) {
	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("100% done")
	if got := task.snapshot().Name; got != "100% done" {
		t.Fatalf("name = %q, want literal passthrough (no Sprintf) when no args given", got)
	}
}

func TestAPISugar_ItemOptionSurvivesAmongFormatArgs(t *testing.T) {
	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })

	item := out.Task(fmt.Sprintf("probe %s", "docker"))
	if got := item.snapshot().Name; got != "probe docker" {
		t.Fatalf("name = %q, want %q", got, "probe docker")
	}
}

func TestAPISugar_GroupNameIsPrintfWhenArgsPresent(t *testing.T) {
	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })

	g := out.Sequence(fmt.Sprintf("stage %d", 2))
	if got := g.snapshot().Name; got == "" || got == "stage %d" {
		t.Fatalf("group name not formatted: %q", got)
	}
}

func TestAPISugar_GroupTaskNameIsPrintfWhenArgsPresent(t *testing.T) {
	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })

	group := out.Sequence("stages")
	child := group.Task(fmt.Sprintf("stage %d", 2))
	if got := child.snapshot().Name; got != "stage 2" {
		t.Fatalf("name = %q, want %q", got, "stage 2")
	}
}

// --- Item 0: Fail/Block are statement-form ---

func TestAPISugar_TaskFailIsStatementForm(t *testing.T) {
	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("validate")
	task.fail("validate policy manifest") // errcheck-clean: no return value
	if got := task.snapshot().State; got != failed {
		t.Fatalf("state = %q, want Failed", got)
	}
	if got := task.snapshot().Summary; got != "validate policy manifest" {
		t.Fatalf("summary = %q, want the Fail argument", got)
	}
}

func TestAPISugar_TaskFailAttachesDetail(t *testing.T) {
	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("validate")
	cause := errors.New("manifest missing")
	task.fail("validate policy manifest", Detail(cause.Error()))
	if got := task.snapshot().Summary; got != "validate policy manifest" {
		t.Fatalf("summary = %q, want the Fail argument", got)
	}
	if got := task.snapshot().Problems[0].Detail; got != "manifest missing" {
		t.Fatalf("detail = %q, want the cause text", got)
	}
}

func TestAPISugar_TaskFailNextAttachesRemedy(t *testing.T) {
	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("validate")
	task.fail("validate policy manifest", Detail("manifest missing"),
		Next(label("re-run with --force")))
	snap := task.snapshot().Problems[0]
	if len(snap.Actions) != 1 || snap.Actions[0].Label != "re-run with --force" {
		t.Fatalf("actions = %#v, want the Next label attached", snap.Actions)
	}
}

func TestAPISugar_TaskBlockNextCommandAttachesRemedy(t *testing.T) {
	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("apply")
	task.block("dirty working tree", nextCommand("git", "status"))
	snap := task.snapshot().Problems[0]
	if len(snap.Actions) != 1 || snap.Actions[0].Command == nil || snap.Actions[0].Command.Executable != "git" {
		t.Fatalf("actions = %#v, want the NextCommand attached", snap.Actions)
	}
}

// TestAPISugar_DoingDeclaresWithPhaseSet pins L7: Task(...).Doing(...)
// declares with the first phase set in one chained call (StartPhase,
// removed in 1.1, was the old spelling).
func TestAPISugar_DoingDeclaresWithPhaseSet(t *testing.T) {
	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("download base image").Doing("resolving tag")
	snap := task.snapshot()
	if snap.Phase != "resolving tag" {
		t.Fatalf("phase = %q, want %q", snap.Phase, "resolving tag")
	}
	if snap.State != running {
		t.Fatalf("state = %q, want Running", snap.State)
	}
}

// TestAPISugar_ProgressDoingSetsCountAndCurrentItem pins the 1.1 form:
// Progress(completed, total).Doing(item) sets count and current item.
func TestAPISugar_ProgressDoingSetsCountAndCurrentItem(t *testing.T) {
	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("sync")
	task.Progress(3, 10).Doing("syncing widget-3")

	snap := task.snapshot()
	if snap.Progress.Completed != 3 || snap.Progress.Total != 10 {
		t.Fatalf("progress = %+v, want 3/10", snap.Progress)
	}
	if snap.Phase != "syncing widget-3" {
		t.Fatalf("phase = %q, want %q", snap.Phase, "syncing widget-3")
	}
}

func TestAPISugar_TaskFailTakesWholeSummary(t *testing.T) {
	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("validate")
	task.fail("validate manifest: exit 1")
	if got := task.snapshot().Summary; got != "validate manifest: exit 1" {
		t.Fatalf("summary = %q, want the Fail argument", got)
	}
}

func TestAPISugar_ItemBlockSetsBlockedState(t *testing.T) {
	out := initRun(Config{Facades: Facades{Stderr: io.Discard}})
	t.Cleanup(func() { _ = out.close() })

	item := out.Task("policy gate")
	cause := errors.New("denied")
	item.block("blocked by policy", Detail(cause.Error()))
	if got := item.snapshot().State; got != refused {
		t.Fatalf("state = %q, want Blocked", got)
	}
}

func TestAPISugar_FailNilHandleIsSafe(t *testing.T) {
	var task *Task
	task.fail("summary") // must not panic

	var item *Task
	item.block("summary") // must not panic

	var itemF *Task
	itemF.block("summary")
}

// --- Item 3: task.Run subprocess facade ---
