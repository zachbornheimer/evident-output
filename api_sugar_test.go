package evo_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// --- Item 1: printf-variadic entity names ---

func TestAPISugar_TaskNameIsPrintfWhenArgsPresent(t *testing.T) {
	out := evo.Init(evo.Config{Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task(fmt.Sprintf("build %s #%d", "worker", 3))
	if got := task.Snapshot().Name; got != "build worker #3" {
		t.Fatalf("name = %q, want %q", got, "build worker #3")
	}
}

func TestAPISugar_TaskNameUnchangedWithoutArgs(t *testing.T) {
	out := evo.Init(evo.Config{Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("100% done")
	if got := task.Snapshot().Name; got != "100% done" {
		t.Fatalf("name = %q, want literal passthrough (no Sprintf) when no args given", got)
	}
}

// TestAPISugar_TaskDuplicateFormattedNameIsRejected proves identity keys off
// the *formatted* name (not the raw format string): a second evo.Task call
// producing the same formatted text is a duplicate sibling declaration
// (§3.1), while a differently formatted name declares a distinct sibling.
func TestAPISugar_TaskDuplicateFormattedNameIsRejected(t *testing.T) {
	var buf strings.Builder
	out := evo.Init(evo.Config{Stdout: &buf, Color: evo.ColorNever, Plain: true})
	evo.SetDefault(out)

	first := evo.Task(fmt.Sprintf("branch %s", "main"))
	second := evo.Task(fmt.Sprintf("branch %s", "main"))
	if second == first {
		t.Fatal("expected a distinct (failed, orphaned) handle for the duplicate declaration")
	}
	if err := out.Err(); !errors.Is(err, evo.ErrDuplicateSiblingName) {
		t.Fatalf("Err() = %v, want ErrDuplicateSiblingName", err)
	}
	other := evo.Task(fmt.Sprintf("branch %s", "dev"))
	if other == first {
		t.Fatal("differently formatted names must not collide")
	}
}

func TestAPISugar_ItemOptionSurvivesAmongFormatArgs(t *testing.T) {
	out := evo.Init(evo.Config{Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	item := out.Task(fmt.Sprintf("probe %s", "docker"))
	if got := item.Snapshot().Name; got != "probe docker" {
		t.Fatalf("name = %q, want %q", got, "probe docker")
	}
}

func TestAPISugar_GroupNameIsPrintfWhenArgsPresent(t *testing.T) {
	out := evo.Init(evo.Config{Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	g := out.Sequence(fmt.Sprintf("stage %d", 2))
	if got := g.Snapshot().Name; got == "" || got == "stage %d" {
		t.Fatalf("group name not formatted: %q", got)
	}
}

// TestAPISugar_ReasonFormatsAndGetsOrCreates is C6: Reason is
// printf-variadic itself now (Reasonf deleted) — a format arg formats the
// name, and the formatted text still get-or-creates the same bucket a
// literal call with the same text would.
func TestAPISugar_ReasonFormatsAndGetsOrCreates(t *testing.T) {
	var buf strings.Builder
	evo.SetDefault(evo.Init(evo.Config{Stdout: &buf, Color: evo.ColorNever, Plain: true}))

	first := evo.Reason(fmt.Sprintf("stage %d", 2))
	if got := first.Name(); got != "stage 2" {
		t.Fatalf("name = %q, want %q", got, "stage 2")
	}
	second := evo.Reason("stage 2")
	if first.Name() != second.Name() {
		t.Fatal("expected Reason's formatted name to get-or-create the same bucket as a literal call")
	}
}

func TestAPISugar_GroupTaskNameIsPrintfWhenArgsPresent(t *testing.T) {
	out := evo.Init(evo.Config{Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	group := out.Sequence("stages")
	child := group.Task(fmt.Sprintf("stage %d", 2))
	if got := child.Snapshot().Name; got != "stage 2" {
		t.Fatalf("name = %q, want %q", got, "stage 2")
	}
}

// --- Item 0: Fail/Block are statement-form ---

func TestAPISugar_TaskFailIsStatementForm(t *testing.T) {
	out := evo.Init(evo.Config{Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("validate")
	task.Fail("validate policy manifest") // errcheck-clean: no return value
	if got := task.Snapshot().State; got != evo.Failed {
		t.Fatalf("state = %q, want Failed", got)
	}
	if got := task.Snapshot().Summary; got != "validate policy manifest" {
		t.Fatalf("summary = %q, want the Fail argument", got)
	}
}

// TestAPISugar_DoingDeclaresWithPhaseSet pins L7: Task(...).Doing(...)
// declares with the first phase set in one chained call (evo.StartPhase,
// removed in 1.1, was the old spelling).
func TestAPISugar_DoingDeclaresWithPhaseSet(t *testing.T) {
	out := evo.Init(evo.Config{Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("download base image").Doing("resolving tag")
	snap := task.Snapshot()
	if snap.Phase != "resolving tag" {
		t.Fatalf("phase = %q, want %q", snap.Phase, "resolving tag")
	}
	if snap.State != evo.Running {
		t.Fatalf("state = %q, want Running", snap.State)
	}
}

// TestAPISugar_StepSetsProgressAndPhaseUnderOneLock pins L8: Step updates
// progress count and phase text together, so both always describe the same
// unit of work even under concurrent callers.
func TestAPISugar_StepSetsProgressAndPhaseUnderOneLock(t *testing.T) {
	out := evo.Init(evo.Config{Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("sync")
	task.StepForTest(3, 10, "syncing widget-3")

	snap := task.Snapshot()
	if snap.Progress.Completed != 3 || snap.Progress.Total != 10 {
		t.Fatalf("progress = %+v, want 3/10", snap.Progress)
	}
	if snap.Phase != "syncing widget-3" {
		t.Fatalf("phase = %q, want %q", snap.Phase, "syncing widget-3")
	}
}

// TestAPISugar_StepConcurrentWorkersNeverInterleave races N goroutines each
// calling Step with a matched (index, name) pair; the final Snapshot's
// Progress and Phase must always agree with ONE goroutine's own pair — never
// a mix (the defect two separate Progress+Phase calls under two separate
// locks allowed).
func TestAPISugar_StepConcurrentWorkersNeverInterleave(t *testing.T) {
	out := evo.Init(evo.Config{Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("sync")

	const n = 50
	var wg sync.WaitGroup
	for i := 1; i <= n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			task.StepForTest(i, n, fmt.Sprintf("item-%d", i))
		}(i)
	}
	wg.Wait()

	snap := task.Snapshot()
	want := fmt.Sprintf("item-%d", snap.Progress.Completed)
	if snap.Progress.Completed < 1 || snap.Progress.Completed > n {
		t.Fatalf("completed = %d out of range", snap.Progress.Completed)
	}
	if snap.Phase != want {
		t.Fatalf("phase = %q, want %q (matched to completed=%d)", snap.Phase, want, snap.Progress.Completed)
	}
}

// TestStep_IsolatedPlainDoesNotEmitPerNamePhase pins Step's live-only name:
// Isolated+Plain may stream thinned progress milestones (~10), but must not
// emit a durable phase line per unique item name. Snapshot.Phase is still
// the last name (the TTY bar can show the path without flooding the pipe).
func TestStep_IsolatedPlainDoesNotEmitPerNamePhase(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("sync")
	const total = 40
	names := make([]string, total)
	for i := 1; i <= total; i++ {
		names[i-1] = fmt.Sprintf("widget-%02d", i)
		task.StepForTest(i, total, names[i-1])
	}
	last := names[total-1]
	if got := task.Snapshot().Phase; got != last {
		t.Fatalf("phase = %q, want last Step name %q", got, last)
	}

	succeed(task)
	if err := out.Finish(); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	transcript := buf.String()
	namedLines := 0
	for _, name := range names {
		if strings.Contains(transcript, name) {
			namedLines++
		}
	}
	if namedLines >= total {
		t.Fatalf("durable transcript contained %d unique Step names (want < %d; progress milestones are allowed):\n%s", namedLines, total, transcript)
	}
}

func TestAPISugar_FailNilHandleIsSafe(t *testing.T) {
	var task *evo.TaskHandle
	task.Fail("summary") // must not panic

	var item *evo.TaskHandle
	item.Block("summary") // must not panic
}

// --- Item 3: task.Run subprocess facade ---

func TestAPISugar_RunCapturesOutputAndUpdatesPhase(t *testing.T) {
	out := evo.Init(evo.Config{Stdout: io.Discard, Plain: true})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("build")
	cmd := exec.Command("/bin/sh", "-c", "echo line-one; echo line-two 1>&2")
	if err := task.RunForTest(cmd); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	tail := task.CaptureForTest().Text()
	if !strings.Contains(tail, "line-one") || !strings.Contains(tail, "line-two") {
		t.Fatalf("capture tail = %q, want both stdout and stderr lines retained", tail)
	}
	if got := task.Snapshot().Phase; got == "" {
		t.Fatal("expected Phase to have been set by child output")
	}
}

func TestAPISugar_RunSetsPhaseFromCommandName(t *testing.T) {
	out := evo.Init(evo.Config{Stdout: io.Discard, Plain: true})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("build")
	cmd := exec.Command("/usr/bin/true")
	if err := task.RunForTest(cmd); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := task.Snapshot().Phase; got != "true" {
		t.Fatalf("phase = %q, want basename of argv[0] (%q)", got, "true")
	}
}

// TestAPISugar_RunSkipsShellWrapperPhase is beginner-11: task.Run must
// never publish a shell wrapper's own basename ("sh") as a placeholder
// phase — it reads the meaningful command from the wrapper's -c script
// instead (FP-004 applies to ourselves).
func TestAPISugar_RunSkipsShellWrapperPhase(t *testing.T) {
	out := evo.Init(evo.Config{Stdout: io.Discard, Plain: true})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("build")
	cmd := exec.Command("/bin/sh", "-c", "true")
	if err := task.RunForTest(cmd); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := task.Snapshot().Phase; got != "true" {
		t.Fatalf("phase = %q, want the shell script's own command (%q), not the wrapper", got, "true")
	}
}

func TestAPISugar_RunTeesPreWiredWriters(t *testing.T) {
	out := evo.Init(evo.Config{Stdout: io.Discard, Plain: true})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("build")
	var mine strings.Builder
	cmd := exec.Command("/bin/sh", "-c", "echo hello")
	cmd.Stdout = &mine
	if err := task.RunForTest(cmd); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(mine.String(), "hello") {
		t.Fatalf("pre-wired writer = %q, want it still received output", mine.String())
	}
	if !strings.Contains(task.CaptureForTest().Text(), "hello") {
		t.Fatal("expected Run's own capture to also observe teed stdout")
	}
}

func TestAPISugar_RunReturnsSubprocessErrorVerbatim(t *testing.T) {
	out := evo.Init(evo.Config{Stdout: io.Discard, Plain: true})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("build")
	cmd := exec.Command("/bin/sh", "-c", "exit 3")
	err := task.RunForTest(cmd)
	if err == nil {
		t.Fatal("expected non-nil error for a nonzero exit")
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("err = %v (%T), want *exec.ExitError", err, err)
	}
	if task.Snapshot().State != evo.Pending && task.Snapshot().State != evo.Running {
		t.Fatalf("state = %q, want Run to leave verdict to the caller", task.Snapshot().State)
	}
}

type literalRedactor struct{ secret string }

func (r literalRedactor) RedactString(s string) string {
	return strings.ReplaceAll(s, r.secret, "***")
}

func TestAPISugar_RunRedactsSecrets(t *testing.T) {
	out := evo.Init(evo.Config{Stdout: io.Discard, Redactor: literalRedactor{secret: "s3kr3t"}, Plain: true})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("build")
	cmd := exec.Command("/bin/sh", "-c", "echo token=s3kr3t")
	if err := task.RunForTest(cmd); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if strings.Contains(task.CaptureForTest().Text(), "s3kr3t") {
		t.Fatalf("capture tail leaked the redacted secret: %q", task.CaptureForTest().Text())
	}
}
