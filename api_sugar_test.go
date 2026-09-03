package evo_test

import (
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// --- Item 1: printf-variadic entity names ---

func TestAPISugar_TaskNameIsPrintfWhenArgsPresent(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("build %s #%d", "worker", 3)
	if got := task.Snapshot().Name; got != "build worker #3" {
		t.Fatalf("name = %q, want %q", got, "build worker #3")
	}
}

func TestAPISugar_TaskNameUnchangedWithoutArgs(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("100% done")
	if got := task.Snapshot().Name; got != "100% done" {
		t.Fatalf("name = %q, want literal passthrough (no Sprintf) when no args given", got)
	}
}

func TestAPISugar_TaskGetOrCreateKeysOnFormattedName(t *testing.T) {
	var buf strings.Builder
	evo.SetDefault(evo.NewWithOptions(evo.To(&buf), evo.Plain(), evo.NoColor()))

	first := evo.Task("branch %s", "main")
	second := evo.Task("branch %s", "main")
	if first != second {
		t.Fatal("expected get-or-create identity on the formatted name")
	}
	other := evo.Task("branch %s", "dev")
	if other == first {
		t.Fatal("differently formatted names must not collide")
	}
}

func TestAPISugar_ItemOptionSurvivesAmongFormatArgs(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })

	item := out.Item("probe %s", "docker", evo.ID("probe.docker"))
	if got := item.Snapshot().Name; got != "probe docker" {
		t.Fatalf("name = %q, want %q", got, "probe docker")
	}
	if got := item.Snapshot().Key; got != "probe.docker" {
		t.Fatalf("key = %q, want %q", got, "probe.docker")
	}
}

func TestAPISugar_GroupNameIsPrintfWhenArgsPresent(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })

	g := out.Group("stage %d", 2)
	if got := g.Snapshot().Name; got == "" || got == "stage %d" {
		t.Fatalf("group name not formatted: %q", got)
	}
}

// --- Item 2: resolving failure verbs return error ---

func TestAPISugar_TaskFailReturnsWrappedCause(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("validate")
	cause := errors.New("manifest missing")
	err := task.Fail("validate policy manifest", evo.Cause(cause))
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(err.Error(), "validate policy manifest") {
		t.Fatalf("error message = %q, want it to contain the summary", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is(err, cause) = false, want true (must wrap with %%w)")
	}
}

func TestAPISugar_TaskFailNoCauseIsPlainError(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("validate")
	err := task.Fail("no cause here")
	if err == nil || err.Error() != "no cause here" {
		t.Fatalf("err = %v, want plain errors.New(summary)", err)
	}
	if errors.Unwrap(err) != nil {
		t.Fatalf("expected no wrapped error when Cause is absent")
	}
}

func TestAPISugar_TaskFailfFormatsAndReturnsError(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("validate")
	err := task.Failf("validate %s: exit %d", "manifest", 1)
	if err == nil || err.Error() != "validate manifest: exit 1" {
		t.Fatalf("err = %v, want formatted summary", err)
	}
}

func TestAPISugar_ItemBlockReturnsWrappedCause(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })

	item := out.Item("policy gate")
	cause := errors.New("denied")
	err := item.Block("blocked by policy", evo.Cause(cause))
	if err == nil || !errors.Is(err, cause) {
		t.Fatalf("err = %v, want it to wrap cause", err)
	}
}

func TestAPISugar_FailNilHandleIsSafe(t *testing.T) {
	var task *evo.TaskHandle
	err := task.Fail("summary", evo.Cause(errors.New("boom")))
	if err == nil {
		t.Fatal("expected non-nil error even on a nil handle")
	}

	var item *evo.ItemHandle
	if err := item.Block("summary"); err == nil {
		t.Fatal("expected non-nil error even on a nil handle")
	}
}

// --- Item 3: task.Run subprocess facade ---

func TestAPISugar_RunCapturesOutputAndUpdatesPhase(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard), evo.Plain())
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("build")
	cmd := exec.Command("/bin/sh", "-c", "echo line-one; echo line-two 1>&2")
	if err := task.Run(cmd); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	tail := task.Capture().Tail()
	if !strings.Contains(tail, "line-one") || !strings.Contains(tail, "line-two") {
		t.Fatalf("capture tail = %q, want both stdout and stderr lines retained", tail)
	}
	if got := task.Snapshot().Phase; got == "" {
		t.Fatal("expected Phase to have been set by child output")
	}
}

func TestAPISugar_RunSetsPhaseFromCommandName(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard), evo.Plain())
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("build")
	cmd := exec.Command("/bin/sh", "-c", "true")
	if err := task.Run(cmd); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := task.Snapshot().Phase; got != "sh" {
		t.Fatalf("phase = %q, want basename of argv[0] (%q)", got, "sh")
	}
}

func TestAPISugar_RunTeesPreWiredWriters(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard), evo.Plain())
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("build")
	var mine strings.Builder
	cmd := exec.Command("/bin/sh", "-c", "echo hello")
	cmd.Stdout = &mine
	if err := task.Run(cmd); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !strings.Contains(mine.String(), "hello") {
		t.Fatalf("pre-wired writer = %q, want it still received output", mine.String())
	}
	if !strings.Contains(task.Capture().Tail(), "hello") {
		t.Fatal("expected Run's own capture to also observe teed stdout")
	}
}

func TestAPISugar_RunReturnsSubprocessErrorVerbatim(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard), evo.Plain())
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("build")
	cmd := exec.Command("/bin/sh", "-c", "exit 3")
	err := task.Run(cmd)
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
	out := evo.NewWithOptions(evo.To(io.Discard), evo.Plain(), evo.Redact(literalRedactor{secret: "s3kr3t"}))
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("build")
	cmd := exec.Command("/bin/sh", "-c", "echo token=s3kr3t")
	if err := task.Run(cmd); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if strings.Contains(task.Capture().Tail(), "s3kr3t") {
		t.Fatalf("capture tail leaked the redacted secret: %q", task.Capture().Tail())
	}
}
