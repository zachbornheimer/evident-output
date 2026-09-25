package evo_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
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

// --- Item 0: Fail/Block are statement-form; Failf/Blockf return %w errors ---

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

func TestAPISugar_TaskFailfWrapsAndReturnsError(t *testing.T) {
	out := evo.Init(evo.Config{Stdout: io.Discard})
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
	out := evo.Init(evo.Config{Stdout: io.Discard})
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
	out := evo.Init(evo.Config{Stdout: io.Discard})
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

// TestAPISugar_ProgressDoingSetsCountAndItem pins the canonical current-
// item form that replaced Step in 1.1 (E-119): Progress carries the count
// and Doing the item.
func TestAPISugar_ProgressDoingSetsCountAndItem(t *testing.T) {
	out := evo.Init(evo.Config{Stdout: io.Discard})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("sync")
	task.Progress(3, 10).Doing("syncing widget-3")

	snap := task.Snapshot()
	if snap.Progress.Completed != 3 || snap.Progress.Total != 10 {
		t.Fatalf("progress = %+v, want 3/10", snap.Progress)
	}
	if snap.Phase != "syncing widget-3" {
		t.Fatalf("phase = %q, want %q", snap.Phase, "syncing widget-3")
	}
}

// TestProgressDoing_IsolatedPlainDoesNotEmitPerItemPhase pins the item
// name of Progress(i, total).Doing(item) as live-only, the property Step
// owned before 1.1 removed it (E-119): Isolated+Plain may stream thinned
// progress milestones (~10), but must not emit a durable phase line per
// item. Snapshot.Phase is still the last item (the TTY bar can show the
// path without flooding the pipe).
func TestProgressDoing_IsolatedPlainDoesNotEmitPerItemPhase(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("sync")
	const total = 40
	names := make([]string, total)
	for i := 1; i <= total; i++ {
		names[i-1] = fmt.Sprintf("widget-%02d", i)
		task.Progress(i, total).Doing(names[i-1])
	}
	last := names[total-1]
	if got := task.Snapshot().Phase; got != last {
		t.Fatalf("phase = %q, want last item %q", got, last)
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
		t.Fatalf("durable transcript contained %d unique item names (want < %d; progress milestones are allowed):\n%s", namedLines, total, transcript)
	}
}

// TestProgressDoing_PlainMilestoneNamesItsOwnItem pins which item a plain
// milestone line names: in a Progress(i, total).Doing(item) loop, the line
// for count i names item i, the one in progress, never the previous one.
func TestProgressDoing_PlainMilestoneNamesItsOwnItem(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("sync")

	const total = 40
	for i := 1; i <= total; i++ {
		task.Progress(i, total).Doing("widget-%02d", i)
	}
	succeed(task)
	_ = out.Close()

	var itemLines int
	seenCounts := map[string]string{} // "C/40" -> the whole line it appeared on first
	for line := range strings.SplitSeq(buf.String(), "\n") {
		fields := strings.Fields(line)
		// Every milestone line, bare or item-named, carries its "C/40" count
		// as the field right after the glyph and task name. A count must
		// stream on exactly one line — never a bare line and then a
		// separately-named item line for the same count (the E-119 review's
		// duplicate-first-milestone bug: namesItems being false on the
		// first Progress streamed a bare line immediately, then the Doing
		// that followed streamed the same count again).
		for _, f := range fields {
			if !strings.HasSuffix(f, "/"+strconv.Itoa(total)) {
				continue
			}
			if prior, ok := seenCounts[f]; ok {
				t.Fatalf("count %s streamed on more than one line:\n  %s\n  %s", f, prior, line)
			}
			seenCounts[f] = line
		}
		var completed, item int
		if len(fields) < 4 || !strings.HasPrefix(fields[3], "widget-") {
			continue
		}
		itemLines++
		if _, err := fmt.Sscanf(fields[2]+" "+fields[3], "%d/40 widget-%d", &completed, &item); err != nil {
			t.Fatalf("unparsable item line %q: %v", line, err)
		}
		if completed != item {
			t.Fatalf("milestone %d/%d names widget-%02d, want widget-%02d:\n%s", completed, total, item, completed, buf.String())
		}
	}
	if itemLines == 0 {
		t.Fatalf("no milestone line named an item:\n%s", buf.String())
	}
}

// TestProgressDoing_FinalMilestoneDoingNoOrphanLine is the regression for
// the canonical `task.Progress(total, total).Doing(item)` chain's last
// iteration: the final tick streams its count bare ("40/40") the instant it
// happens (TestProgressDoing_PlainMilestoneNamesItsOwnItem's own final
// line), and the Doing right after it — naming that same, already-sealed
// milestone's item — must go unshown, never trail it as a second,
// item-only line with no count on it at all. A prior regression fed that
// Doing through the ordinary narrated path once the count was sealed,
// which streamed exactly that orphan line.
func TestProgressDoing_FinalMilestoneDoingNoOrphanLine(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("sync")

	const total = 20
	for i := 1; i <= total; i++ {
		task.Progress(i, total).Doing("widget-%02d", i)
	}
	succeed(task)
	_ = out.Close()

	for line := range strings.SplitSeq(buf.String(), "\n") {
		if strings.Contains(line, "widget-20") {
			t.Fatalf("want the final milestone's Doing to stay unshown (already sealed \"20/20\" streamed bare), got an orphan line:\n%s\nfull transcript:\n%s", line, buf.String())
		}
	}
}

// TestDoingProgress_PlainMilestoneNeverPairsWrongItem is
// TestProgressDoing_PlainMilestoneNamesItsOwnItem's sibling for the
// Doing-before-Progress loop order (`task.Doing(item); task.Progress(i,
// total)`). This order is not a supported pairing shape — see
// docs/migration/1.1.md's "Only task.Progress(i, total).Doing(item) …
// pairs" note: a Doing that precedes the count ever opening is
// indistinguishable, from call order alone, between an ordinary narrated
// step ahead of the loop (`task.Doing("reading manifest")`) and that same
// loop's own first item (E-119 review's RED repro, r8-red-e119.txt —
// guessing between the two misclassified the prelude case). So items are
// never guessed onto a milestone in this order; this test pins that the
// milestones themselves still survive correctly (each count exactly once,
// the final tick included) rather than pinning a pairing this order
// cannot support unambiguously.
func TestDoingProgress_PlainMilestoneNeverPairsWrongItem(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Color: evo.ColorNever, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("sync")

	const total = 40
	for i := 1; i <= total; i++ {
		task.Doing("widget-%02d", i)
		task.Progress(i, total)
	}
	succeed(task)
	_ = out.Close()

	seenCounts := map[string]string{} // "C/40" -> the whole line it appeared on first
	for line := range strings.SplitSeq(buf.String(), "\n") {
		fields := strings.FieldsSeq(line)
		for f := range fields {
			if !strings.HasSuffix(f, "/"+strconv.Itoa(total)) {
				continue
			}
			if prior, ok := seenCounts[f]; ok {
				t.Fatalf("count %s streamed on more than one line:\n  %s\n  %s", f, prior, line)
			}
			seenCounts[f] = line
		}
	}
	final := strconv.Itoa(total) + "/" + strconv.Itoa(total)
	if _, ok := seenCounts[final]; !ok {
		t.Fatalf("want the final milestone %s to survive, got:\n%s", final, buf.String())
	}
}

func TestAPISugar_TaskFailfNoTrailingWrapIsWholeSummary(t *testing.T) {
	out := evo.Init(evo.Config{Stdout: io.Discard})
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
	out := evo.Init(evo.Config{Stdout: io.Discard})
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

	var item *evo.TaskHandle
	item.Block("summary") // must not panic

	var itemF *evo.TaskHandle
	if err := itemF.Blockf("summary: %w", errors.New("boom")); err == nil {
		t.Fatal("expected non-nil error even on a nil handle")
	}
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

	tail := task.EvidenceForTest().Text()
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
	if !strings.Contains(task.EvidenceForTest().Text(), "hello") {
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
	if strings.Contains(task.EvidenceForTest().Text(), "s3kr3t") {
		t.Fatalf("capture tail leaked the redacted secret: %q", task.EvidenceForTest().Text())
	}
}
