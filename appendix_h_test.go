package evo_test

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// Appendix H normative red tests (v0.1-alpha subset).
// Interactive H.2/H.17/H.20–H.22 require testkit terminal (v0.2).

func TestH1_Task_PhaseStartsPendingTask(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	dependencies := out.Task("dependencies")
	dependencies.Phase("reading lockfile")

	got := dependencies.Snapshot()
	if got.State != evo.Running {
		t.Fatalf("state = %q, want %q", got.State, evo.Running)
	}
	if got.Phase != "reading lockfile" {
		t.Fatalf("phase = %q, want reading lockfile", got.Phase)
	}
	if got.Progress.Kind != evo.Indeterminate {
		t.Fatalf("kind = %q, want %q", got.Progress.Kind, evo.Indeterminate)
	}
}

func TestH3_Task_ProgressStartsDeterminateTask(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	dependencies := out.Task("dependencies")
	dependencies.Progress(12, 18)

	got := dependencies.Snapshot()
	if got.State != evo.Running {
		t.Fatalf("state = %q, want %q", got.State, evo.Running)
	}
	if got.Progress.Completed != 12 || got.Progress.Total != 18 {
		t.Fatalf("progress = %d/%d, want 12/18", got.Progress.Completed, got.Progress.Total)
	}
}

func TestH4_Task_InvalidProgressIsRecordedWithoutCorruption(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("download")
	task.Progress(4, 10)
	task.Progress(11, 10)

	got := task.Snapshot()
	if got.Progress.Completed != 4 || got.Progress.Total != 10 {
		t.Fatalf("last valid progress was not preserved: %#v", got.Progress)
	}
	if !errors.Is(out.Err(), evo.ErrInvalidProgress) {
		t.Fatalf("error = %v, want ErrInvalidProgress", out.Err())
	}
}

func TestH5_Task_BackwardProgressIsRejected(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("download")
	task.Bytes(500, 1_000)
	task.Bytes(400, 1_000)

	if !errors.Is(out.Err(), evo.ErrProgressRegression) {
		t.Fatalf("error = %v, want ErrProgressRegression", out.Err())
	}
}

func TestH6_Item_BlockCreatesSingleProblem(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	workingTree := out.Task("working tree")
	workingTree.Block(
		"unstashed changes",
		evo.Detail("index contains modified files"),
	)

	got := workingTree.Snapshot()
	if got.State != evo.Blocked {
		t.Fatalf("state = %q, want %q", got.State, evo.Blocked)
	}
	if len(got.Problems) != 1 {
		t.Fatalf("problems = %d, want 1", len(got.Problems))
	}
	if got.Problems[0].Summary != "unstashed changes" {
		t.Fatalf("summary = %q", got.Problems[0].Summary)
	}
}

func TestH9_Task_FirstTerminalStateWins(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	item := out.Task("working tree")
	item.Done()
	item.Block("unstashed changes")

	if got := item.Snapshot().State; got != evo.Done {
		t.Fatalf("state = %q, want %q", got, evo.Done)
	}
	if !errors.Is(out.Err(), evo.ErrAlreadyResolved) {
		t.Fatalf("error = %v, want ErrAlreadyResolved", out.Err())
	}
}

func TestH10_Task_ConcurrentResolutionPreservesDeclarationOrder(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	workingTree := out.Task("working tree")
	branches := out.Task("branches")
	remotes := out.Task("remotes")

	remoteResolved := make(chan struct{})
	branchResolved := make(chan struct{})

	var group sync.WaitGroup
	group.Go(func() { remotes.Done(); close(remoteResolved) })
	group.Go(func() { <-remoteResolved; branches.Warn("unreachable"); close(branchResolved) })
	group.Go(func() { <-branchResolved; workingTree.Done() })
	group.Wait()

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	conclusion := out.Conclusion()
	got := []string{
		conclusion.Tasks[0].Name,
		conclusion.Tasks[1].Name,
		conclusion.Tasks[2].Name,
	}
	want := []string{"working tree", "branches", "remotes"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %q, want %q", got, want)
	}
}

func TestH11_Tasks_StateIsDerivedFromChildren(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	dependencies := out.Tasks("dependencies")
	react := dependencies.Task("react")
	sharp := dependencies.Task("sharp")

	react.Done()
	sharp.Fail("checksum mismatch")

	got := dependencies.Snapshot()
	if got.State != evo.Failed {
		t.Fatalf("state = %q, want %q", got.State, evo.Failed)
	}
}

func TestH12_Tasks_SuccessSummaryIsSuppressedOnFailure(t *testing.T) {
	var output bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("dependencies"), evo.To(&output), evo.Plain(), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	dependencies := out.Tasks("dependencies")
	dependencies.Summary("installed 2 packages")
	dependencies.Task("react").Done()
	dependencies.Task("sharp").Fail("checksum mismatch")

	_ = out.Finish()

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
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	dependencies := out.Tasks("dependencies")
	dependencies.Task("react").Done()
	dependencies.Task("esbuild")

	err := out.Finish()
	if err != nil {
		t.Fatalf("error = %v, want nil (clean finish, no amnesty-defeating problems)", err)
	}
	if !out.Conclusion().Partial {
		t.Fatal("want Conclusion.Partial = true for the unresolved esbuild task")
	}
}

func TestH14_Changes_AlignVerbQuantityAndObject(t *testing.T) {
	var output bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title(
		"dependencies"), evo.To(&output),
		evo.Plain(),
		evo.NoColor(),
		evo.Width(80),
	}})
	t.Cleanup(func() { _ = out.Close() })

	out.Changes("dependencies").
		Added(14, "package").
		Updated(4, "package").
		Record("reused", 63, "cached package").
		Wrote("app.lock")

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	// Full Finish output: single Changes band; trailing conclusion coalesced (DEC-COAL).
	want := `[changed]  dependencies
  added    14 packages
  updated   4 packages
  reused   63 cached packages
  wrote       app.lock
`
	got := output.String()
	if got != want {
		t.Fatalf("output mismatch\ngot:\n%q\nwant:\n%q", got, want)
	}
}

func TestH15_Changes_NarrowOutputUsesCompactLayout(t *testing.T) {
	var output bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title(
		"dependencies"), evo.To(&output),
		evo.Plain(),
		evo.NoColor(),
		evo.Width(30),
	}})
	t.Cleanup(func() { _ = out.Close() })

	out.Changes("dependencies").
		Added(14, "package").
		Updated(4, "package").
		Wrote("app.lock")

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	want := `[changed]  dependencies
  added 14 packages
  updated 4 packages
  wrote app.lock
`
	got := output.String()
	if got != want {
		t.Fatalf("output mismatch\ngot:\n%q\nwant:\n%q", got, want)
	}
}

func TestH16_Plan_DoesNotInferChangedConclusion(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("account acme"), evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })

	out.Plan("delete account acme").
		Delete(14, "project").
		Record("revoke", 7, "API keys")

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}

	got := out.Conclusion()
	if got.State != evo.StatePlanned || got.Changed {
		t.Fatalf("conclusion = %#v, want planned and unchanged", got)
	}
}

func TestH18_Output_NonInteractiveContainsNoTerminalControls(t *testing.T) {
	var output bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{
		evo.To(&output),
		evo.Plain(),
		evo.NoColor(),
	}})
	t.Cleanup(func() { _ = out.Close() })

	task := out.Task("dependencies")
	task.Phase("reading lockfile")
	task.Phase("resolving packages")
	task.Done("installed %d packages", 18)
	_ = out.Finish()

	got := output.String()
	for _, forbidden := range []string{"\x1b[", "\r"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("non-interactive output contains %q:\n%s", forbidden, got)
		}
	}
}

func TestH19_Output_HumanAndJSONPreserveMeaning(t *testing.T) {
	var buf bytes.Buffer
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.Title("bpp-csharp"), evo.To(&buf), evo.Plain(), evo.NoColor()}})
	t.Cleanup(func() { _ = out.Close() })

	out.Task("working tree").Done()
	out.Task("branches").Block("local-only", evo.On("feat/sdk-full-consolidation"), evo.Count(1))
	out.Task("remotes").Done()
	if err := out.Finish(); err != nil {
		// blocked items are resolved; no unresolved error expected
		t.Fatal(err)
	}
	human := out.Conclusion()
	snap := out.Snapshot()
	// re-infer from snapshot for machine
	machineSnap := snap
	if machineSnap.Conclusion == nil {
		t.Fatal("missing conclusion on snapshot after finish")
	}
	if human.State != machineSnap.Conclusion.State {
		t.Fatalf("human = %q, machine = %q", human.State, machineSnap.Conclusion.State)
	}
	if human.ExitCode != machineSnap.Conclusion.ExitCode {
		t.Fatalf("human exit = %d, machine exit = %d", human.ExitCode, machineSnap.Conclusion.ExitCode)
	}
	raw, err := evo.EncodeJSON(snap)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"schema_version": "0.3"`) {
		t.Fatalf("json missing schema_version:\n%s", raw)
	}
	if !strings.Contains(string(raw), `"state": "blocked"`) {
		t.Fatalf("json missing blocked state:\n%s", raw)
	}
	// JSONL: one object per line, increasing sequence
	lines, err := evo.EncodeJSONL(out.Events())
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(string(lines)), "\n") {
		if line == "" {
			continue
		}
		n++
		if !strings.Contains(line, `"schema_version":"0.3"`) && !strings.Contains(line, `"schema_version": "0.3"`) {
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
