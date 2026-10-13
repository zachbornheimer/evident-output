package evo

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/project/wire"
)

// Appendix H normative red tests (v0.1-alpha subset).
// Interactive H.2/H.17/H.20–H.22 require testkit terminal (v0.2).

func TestH1_Task_PhaseStartsPendingTask(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })

	dependencies := out.Task("dependencies")
	dependencies.Doing("reading lockfile")

	got := dependencies.snapshot()
	if got.State != running {
		t.Fatalf("state = %q, want %q", got.State, running)
	}
	if got.Phase != "reading lockfile" {
		t.Fatalf("phase = %q, want reading lockfile", got.Phase)
	}
	if got.Progress.Kind != indeterminate {
		t.Fatalf("kind = %q, want %q", got.Progress.Kind, indeterminate)
	}
}

func TestH3_Task_ProgressStartsDeterminateTask(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })

	dependencies := out.Task("dependencies")
	dependencies.Progress(12, 18)

	got := dependencies.snapshot()
	if got.State != running {
		t.Fatalf("state = %q, want %q", got.State, running)
	}
	if got.Progress.Completed != 12 || got.Progress.Total != 18 {
		t.Fatalf("progress = %d/%d, want 12/18", got.Progress.Completed, got.Progress.Total)
	}
}

func TestH4_Task_InvalidProgressIsRecordedWithoutCorruption(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("download")
	task.Progress(4, 10)
	task.Progress(11, 10)

	got := task.snapshot()
	if got.Progress.Completed != 4 || got.Progress.Total != 10 {
		t.Fatalf("last valid progress was not preserved: %#v", got.Progress)
	}
	if !errors.Is(out.err(), errInvalidProgress) {
		t.Fatalf("error = %v, want ErrInvalidProgress", out.err())
	}
}

func TestH5_Task_BackwardProgressIsRejected(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("download")
	task.bytes(500, 1_000)
	task.bytes(400, 1_000)

	if !errors.Is(out.err(), errProgressRegression) {
		t.Fatalf("error = %v, want ErrProgressRegression", out.err())
	}
}

func TestH6_Item_BlockCreatesSingleProblem(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })

	workingTree := out.Task("working tree")
	workingTree.block(
		"unstashed changes",
		Detail("index contains modified files"),
	)

	got := workingTree.snapshot()
	if got.State != blocked {
		t.Fatalf("state = %q, want %q", got.State, blocked)
	}
	if len(got.Problems) != 1 {
		t.Fatalf("problems = %d, want 1", len(got.Problems))
	}
	if got.Problems[0].Summary != "unstashed changes" {
		t.Fatalf("summary = %q", got.Problems[0].Summary)
	}
}

func TestH9_Task_FirstTerminalStateWins(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })

	item := out.Task("working tree")
	succeed(item)
	item.block("unstashed changes")

	if got := item.snapshot().State; got != done {
		t.Fatalf("state = %q, want %q", got, done)
	}
	if !errors.Is(out.err(), errAlreadyResolved) {
		t.Fatalf("error = %v, want ErrAlreadyResolved", out.err())
	}
}

func TestH10_Task_ConcurrentResolutionPreservesDeclarationOrder(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })

	workingTree := out.Task("working tree")
	branches := out.Task("branches")
	remotes := out.Task("remotes")

	remoteResolved := make(chan struct{})
	branchResolved := make(chan struct{})

	var group sync.WaitGroup
	group.Go(func() { succeed(remotes); close(remoteResolved) })
	group.Go(func() {
		<-remoteResolved
		branches.Problem("unreachable", severity(severityWarning))
		close(branchResolved)
	})
	group.Go(func() { <-branchResolved; succeed(workingTree) })
	group.Wait()

	if err := out.finish(); err != nil {
		t.Fatal(err)
	}
	conclusion := out.conclusion()
	got := []string{
		conclusion.tasks[0].Name,
		conclusion.tasks[1].Name,
		conclusion.tasks[2].Name,
	}
	want := []string{"working tree", "branches", "remotes"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %q, want %q", got, want)
	}
}

func TestH11_Tasks_StateIsDerivedFromChildren(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })

	dependencies := out.Group("dependencies")
	react := dependencies.Task("react")
	sharp := dependencies.Task("sharp")

	succeed(react)
	sharp.fail("checksum mismatch")

	got := dependencies.snapshot()
	if got.State != failed {
		t.Fatalf("state = %q, want %q", got.State, failed)
	}
}

func TestH12_Tasks_SuccessSummaryIsSuppressedOnFailure(t *testing.T) {
	var output bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &output, title: "dependencies", color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	dependencies := out.Group("dependencies")
	dependencies.Summary("installed 2 packages")
	succeed(dependencies.Task("react"))
	dependencies.Task("sharp").fail("checksum mismatch")

	_ = out.finish()

	if strings.Contains(output.String(), "installed 2 packages") {
		t.Fatalf("success summary was rendered for failed collection:\n%s", output.String())
	}
}

// TestH13_Output_FinishLeavesUnresolvedTaskPartial pins the release-gate
// round 4 finding 3 policy: an unresolved task with no problems of its own,
// on an otherwise clean finish, reads as an honest incomplete outcome
// (Conclusion.Partial), never as misuse — the caller simply forgot a
// terminal verb, and a clean finish must never escalate that to
// ErrUnresolvedTask (previously pinned here as the old behavior).
func TestH13_Output_FinishLeavesUnresolvedTaskPartial(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard})
	t.Cleanup(func() { _ = out.close() })

	dependencies := out.Group("dependencies")
	succeed(dependencies.Task("react"))
	dependencies.Task("esbuild")

	err := out.finish()
	if err != nil {
		t.Fatalf("error = %v, want nil (clean finish, no amnesty-defeating problems)", err)
	}
	if !out.conclusion().partial {
		t.Fatal("want Conclusion.Partial = true for the unresolved esbuild task")
	}
}

func TestH14_Changes_AlignVerbQuantityAndObject(t *testing.T) {
	var output bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &output, title: "dependencies", width: 80, color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	commit(out.Task("dependencies"),
		EffectSpec{Verb: Verb(effectAdd), Object: "package", Quantity: 14},
		EffectSpec{Verb: Update, Object: "package", Quantity: 4},
		EffectSpec{Verb: Verb(effectInstall), Object: "cached package", Quantity: 63},
		EffectSpec{Verb: Create, Object: "app.lock", Quantity: 1},
	)

	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	// Full Finish output: the mutation verbs now live on a Task (P1), so its
	// own terminal row (auto-resolved Done via Finish's amnesty) precedes the
	// Changes band; trailing conclusion still coalesces (DEC-COAL).
	want := `✓ dependencies

[changed]  dependencies
  added      14 packages
  updated     4 packages
  installed  63 cached packages
  created     1 app.lock
`
	got := output.String()
	if got != want {
		t.Fatalf("output mismatch\ngot:\n%q\nwant:\n%q", got, want)
	}
}

func TestH15_Changes_NarrowOutputUsesCompactLayout(t *testing.T) {
	var output bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &output, title: "dependencies", width: 30, color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	commit(out.Task("dependencies"),
		EffectSpec{Verb: Verb(effectAdd), Object: "package", Quantity: 14},
		EffectSpec{Verb: Update, Object: "package", Quantity: 4},
		EffectSpec{Verb: Create, Object: "app.lock", Quantity: 1},
	)

	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	want := `✓ dependencies

[changed]  dependencies
  added 14 packages
  updated 4 packages
  created 1 app.lock
`
	got := output.String()
	if got != want {
		t.Fatalf("output mismatch\ngot:\n%q\nwant:\n%q", got, want)
	}
}

func TestH16_Plan_DoesNotInferChangedConclusion(t *testing.T) {
	out := initRun(Config{isolated: true, stdout: io.Discard, title: "account acme", DryRun: true})
	t.Cleanup(func() { _ = out.close() })

	commit(out.Task("delete account acme"),
		EffectSpec{Verb: Delete, Object: "project", Quantity: 14},
		EffectSpec{Verb: Verb(effectRemove), Object: "API key", Quantity: 7},
	)

	if err := out.finish(); err != nil {
		t.Fatal(err)
	}

	got := out.conclusion()
	if got.state != statePlanned || got.Changed {
		t.Fatalf("conclusion = %#v, want planned and unchanged", got)
	}
}

func TestH18_Output_NonInteractiveContainsNoTerminalControls(t *testing.T) {
	var output bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &output, color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	task := out.Task("dependencies")
	task.Doing("reading lockfile")
	task.Doing("resolving packages")
	succeed(task, "installed 18 packages")
	_ = out.finish()

	got := output.String()
	for _, forbidden := range []string{"\x1b[", "\r"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("non-interactive output contains %q:\n%s", forbidden, got)
		}
	}
}

func TestH19_Output_HumanAndJSONPreserveMeaning(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{isolated: true, stdout: &buf, title: "bpp-csharp", color: colorNever, plain: true})
	t.Cleanup(func() { _ = out.close() })

	succeed(out.Task("working tree"))
	out.Task("branches").block("local-only", On("feat/sdk-full-consolidation"), count(1))
	succeed(out.Task("remotes"))
	if err := out.finish(); err != nil {
		// blocked items are resolved; no unresolved error expected
		t.Fatal(err)
	}
	human := out.conclusion()
	snap := out.snapshot()
	// re-infer from snapshot for machine
	machineSnap := snap
	if machineSnap.Conclusion == nil {
		t.Fatal("missing conclusion on snapshot after finish")
	}
	if human.state != machineSnap.Conclusion.State {
		t.Fatalf("human = %q, machine = %q", human.state, machineSnap.Conclusion.State)
	}
	if human.exitCode != machineSnap.Conclusion.ExitCode {
		t.Fatalf("human exit = %d, machine exit = %d", human.exitCode, machineSnap.Conclusion.ExitCode)
	}
	raw, err := wire.EncodeJSON(snap)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"schema_version": "0.4"`) {
		t.Fatalf("json missing schema_version:\n%s", raw)
	}
	if !strings.Contains(string(raw), `"state": "blocked"`) {
		t.Fatalf("json missing blocked state:\n%s", raw)
	}
	// JSONL: one object per line, increasing sequence
	lines, err := wire.EncodeJSONL(out.events())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for line := range strings.SplitSeq(strings.TrimSpace(string(lines)), "\n") {
		if line == "" {
			continue
		}
		n++
		if !strings.Contains(line, `"schema_version":"0.4"`) && !strings.Contains(line, `"schema_version": "0.4"`) {
			// compact marshal has no space
			if !strings.Contains(line, "schema_version") {
				t.Fatalf("jsonl line missing schema_version: %s", line)
			}
		}
	}
	if n < 2 {
		t.Fatalf("expected multiple jsonl events, got %d", n)
	}
}
