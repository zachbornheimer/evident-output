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
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("build %s #%d", "worker", 3)
	if got := task.Snapshot().Name; got != "build worker #3" {
		t.Fatalf("name = %q, want %q", got, "build worker #3")
	}
}

func TestAPISugar_TaskNameUnchangedWithoutArgs(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("100% done")
	if got := task.Snapshot().Name; got != "100% done" {
		t.Fatalf("name = %q, want literal passthrough (no Sprintf) when no args given", got)
	}
}

func TestAPISugar_TaskGetOrCreateKeysOnFormattedName(t *testing.T) {
	var buf strings.Builder
	evo.SetDefault(evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}}))

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
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	item := out.Task("probe %s", "docker", evo.ID("probe.docker"))
	if got := item.Snapshot().Name; got != "probe docker" {
		t.Fatalf("name = %q, want %q", got, "probe docker")
	}
	if got := item.Snapshot().Key; got != "probe.docker" {
		t.Fatalf("key = %q, want %q", got, "probe.docker")
	}
}

func TestAPISugar_GroupNameIsPrintfWhenArgsPresent(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	g := out.Group("stage %d", 2)
	if got := g.Snapshot().Name; got == "" || got == "stage %d" {
		t.Fatalf("group name not formatted: %q", got)
	}
}

func TestAPISugar_ReasonfFormatsAndGetsOrCreates(t *testing.T) {
	var buf strings.Builder
	evo.SetDefault(evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}}))

	first := evo.Reasonf("stage %d", 2)
	if got := first.Name(); got != "stage 2" {
		t.Fatalf("name = %q, want %q", got, "stage 2")
	}
	second := evo.Reason("stage 2")
	if first.Name() != second.Name() {
		t.Fatal("expected Reasonf's formatted name to get-or-create the same bucket as Reason")
	}
}

func TestAPISugar_ScopeTaskNameIsPrintfWhenArgsPresent(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	scoped := out.Scope("registry")
	task := scoped.Task("sync %s", "auth")
	if got := task.Snapshot().Name; got != "sync auth" {
		t.Fatalf("name = %q, want %q", got, "sync auth")
	}
}

func TestAPISugar_ScopeItemNameIsPrintfWhenArgsPresent(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	scoped := out.Scope("registry")
	item := scoped.Task("probe %s", "docker")
	if got := item.Snapshot().Name; got != "probe docker" {
		t.Fatalf("name = %q, want %q", got, "probe docker")
	}
}

func TestAPISugar_GroupTaskNameIsPrintfWhenArgsPresent(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	group := out.Group("stages")
	child := group.Task("stage %d", 2)
	if got := child.Snapshot().Name; got != "stage 2" {
		t.Fatalf("name = %q, want %q", got, "stage 2")
	}
}

// --- Item 0: Fail/Block are statement-form; Failf/Blockf return %w errors ---

func TestAPISugar_TaskFailIsStatementForm(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
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

func TestAPISugar_TaskFailfWrapsAndReturnsError(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("validate")
	cause := errors.New("manifest missing")
	err := task.Failf("validate policy manifest: %w", cause)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(err.Error(), "validate policy manifest") {
		t.Fatalf("error message = %q, want it to contain the summary", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is(err, cause) = false, want true (must wrap with %%w)")
	}
	if got := task.Snapshot().Summary; got != "validate policy manifest" {
		t.Fatalf("summary = %q, want the text before the trailing %%w split off", got)
	}
}

// TestAPISugar_TaskFailfNextAttachesRemedy pins L2: Failf/Blockf return a
// *Failure so the remedy for a failure has somewhere to attach at the return
// site — `return task.Failf("...: %w", err).Next(...)` — instead of a second
// statement (the zq clean_repo.go build break this closes).
func TestAPISugar_TaskFailfNextAttachesRemedy(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("validate")
	cause := errors.New("manifest missing")

	run := func() error {
		return task.Failf("validate policy manifest: %w", cause).
			Next(evo.Label("re-run with --force"))
	}
	err := run()

	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is(err, cause) = false, want true through *Failure.Unwrap")
	}
	var failure *evo.Failure
	if !errors.As(err, &failure) {
		t.Fatalf("errors.As(err, *evo.Failure) = false, want true")
	}
	snap := task.Snapshot()
	if len(snap.Actions) != 1 || snap.Actions[0].Label != "re-run with --force" {
		t.Fatalf("actions = %#v, want the Next label attached", snap.Actions)
	}
}

// TestAPISugar_TaskBlockfNextCommandAttachesRemedy exercises Blockf's
// matching Next/NextCommand contract.
func TestAPISugar_TaskBlockfNextCommandAttachesRemedy(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("apply")
	err := task.Blockf("dirty working tree").NextCommand("git", "status")
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	snap := task.Snapshot()
	if len(snap.Actions) != 1 || snap.Actions[0].Command == nil || snap.Actions[0].Command.Executable != "git" {
		t.Fatalf("actions = %#v, want the NextCommand attached", snap.Actions)
	}
}

// TestAPISugar_StartPhaseDeclaresWithPhaseSet pins L7: evo.StartPhase
// collapses declare + first Phase into one call, with no gap where the task
// sits Pending between two statements.
func TestAPISugar_StartPhaseDeclaresWithPhaseSet(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("download base image", evo.StartPhase("resolving tag"))
	snap := task.Snapshot()
	if snap.Phase != "resolving tag" {
		t.Fatalf("phase = %q, want %q", snap.Phase, "resolving tag")
	}
	if snap.State != evo.Running {
		t.Fatalf("state = %q, want Running", snap.State)
	}
}

func TestAPISugar_TaskFailfNoTrailingWrapIsWholeSummary(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("validate")
	err := task.Failf("validate %s: exit %d", "manifest", 1)
	if err == nil || err.Error() != "validate manifest: exit 1" {
		t.Fatalf("err = %v, want formatted summary", err)
	}
	if got := task.Snapshot().Summary; got != "validate manifest: exit 1" {
		t.Fatalf("summary = %q, want the whole formatted text (no %%w to split on)", got)
	}
}

func TestAPISugar_ItemBlockfWrapsAndReturnsError(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	item := out.Task("policy gate")
	cause := errors.New("denied")
	err := item.Blockf("blocked by policy: %w", cause)
	if err == nil || !errors.Is(err, cause) {
		t.Fatalf("err = %v, want it to wrap cause", err)
	}
}

func TestAPISugar_FailNilHandleIsSafe(t *testing.T) {
	var task *evo.TaskHandle
	task.Fail("summary") // must not panic

	var item *evo.ItemHandle
	item.Block("summary") // must not panic

	var itemF *evo.ItemHandle
	if err := itemF.Blockf("summary: %w", errors.New("boom")); err == nil {
		t.Fatal("expected non-nil error even on a nil handle")
	}
}

// --- Item 3: task.Run subprocess facade ---

func TestAPISugar_RunCapturesOutputAndUpdatesPhase(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard), evo.Plain()}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("build")
	cmd := exec.Command("/bin/sh", "-c", "echo line-one; echo line-two 1>&2")
	if err := task.Run(cmd); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	tail := task.Evidence().Tail()
	if !strings.Contains(tail, "line-one") || !strings.Contains(tail, "line-two") {
		t.Fatalf("capture tail = %q, want both stdout and stderr lines retained", tail)
	}
	if got := task.Snapshot().Phase; got == "" {
		t.Fatal("expected Phase to have been set by child output")
	}
}

func TestAPISugar_RunSetsPhaseFromCommandName(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard), evo.Plain()}})
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
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard), evo.Plain()}})
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
	if !strings.Contains(task.Evidence().Tail(), "hello") {
		t.Fatal("expected Run's own capture to also observe teed stdout")
	}
}

func TestAPISugar_RunReturnsSubprocessErrorVerbatim(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard), evo.Plain()}})
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
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard), evo.Plain(), evo.Redact(literalRedactor{secret: "s3kr3t"})}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("build")
	cmd := exec.Command("/bin/sh", "-c", "echo token=s3kr3t")
	if err := task.Run(cmd); err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if strings.Contains(task.Evidence().Tail(), "s3kr3t") {
		t.Fatalf("capture tail leaked the redacted secret: %q", task.Evidence().Tail())
	}
}
