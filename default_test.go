package evo

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/testkit"
)

// TestTask_PackageFuncRepeatedNameIsDuplicateSibling proves defaultRun().Task(name)
// called twice with the same name is a duplicate sibling declaration
// (§3.1), not a get-or-create — handles are values, so a caller that wants
// to keep using one declaration keeps the *Task it got instead of
// re-declaring by name.
func TestTask_PackageFuncRepeatedNameIsDuplicateSibling(t *testing.T) {
	var buf bytes.Buffer
	out := initRun(Config{stdout: &buf, color: colorNever, plain: true})
	setDefault(out)

	a := defaultRun().Task("branches")
	b := defaultRun().Task("branches")

	if a == b {
		t.Fatal("defaultRun().Task(name) called twice must return distinct handles")
	}
	if !errors.Is(out.err(), errDuplicateSiblingName) {
		t.Fatalf("Err() = %v, want ErrDuplicateSiblingName", out.err())
	}

	other := defaultRun().Task("worktrees")
	if other == a {
		t.Fatal("a different name must not reuse the same handle")
	}
}

func TestPackageFuncs_DelegateToDefaultInstance(t *testing.T) {
	var buf bytes.Buffer
	setDefault(initRun(Config{stdout: &buf, color: colorNever, plain: true}))

	succeed(defaultRun().Task("working tree"))
	println("hello from package func")

	if err := defaultRun().finish(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "working tree") {
		t.Fatalf("missing item in output:\n%s", got)
	}
	if !strings.Contains(got, "hello from package func") {
		t.Fatalf("missing Println line in output:\n%s", got)
	}
}

func TestVerbose_PackageFuncScopesVisibility(t *testing.T) {
	var buf bytes.Buffer
	setDefault(initRun(Config{
		stdout:    &buf,
		stderr:    &buf,
		verbosity: verbosityVerbose,
	}))

	verbose().println("verbose only line")
	if err := defaultRun().finish(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "verbose only line") {
		t.Fatalf("missing verbose line:\n%s", buf.String())
	}
}

func TestMain_OKExitZero(t *testing.T) {
	var buf bytes.Buffer
	setDefault(initRun(Config{stdout: &buf, color: colorNever, plain: true}))

	code := execute_1(context.Background(), func(ctx context.Context, r *Run) error {
		succeed(defaultRun().Task("working tree"))
		return nil
	}).ExitCode()
	if code != exitOK {
		t.Fatalf("exit %d, want %d; out:\n%s", code, exitOK, buf.String())
	}
}

func TestMain_BlockedExitOne(t *testing.T) {
	var buf bytes.Buffer
	setDefault(initRun(Config{stdout: &buf, color: colorNever, plain: true}))

	code := execute_1(context.Background(), func(ctx context.Context, r *Run) error {
		defaultRun().Task("working tree").block("dirty")
		return nil
	}).ExitCode()
	if code != exitRefused {
		t.Fatalf("exit %d, want %d", code, exitRefused)
	}
}

func TestMain_FailedExitTwo(t *testing.T) {
	var buf bytes.Buffer
	setDefault(initRun(Config{stdout: &buf, color: colorNever, plain: true}))

	code := execute_1(context.Background(), func(ctx context.Context, r *Run) error {
		return errors.New("app boom")
	}).ExitCode()
	if code != exitFailed {
		t.Fatalf("exit %d, want %d", code, exitFailed)
	}
}

func TestMain_NilRunNeverPanics(t *testing.T) {
	var buf bytes.Buffer
	setDefault(initRun(Config{stdout: &buf, color: colorNever, plain: true}))

	code := execute_1(context.Background(), nil).ExitCode()
	if code != exitOK {
		t.Fatalf("exit %d, want %d", code, exitOK)
	}
}

func TestInit_ArmsFirstPaintBeforeAnyEntity(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := initRun(Config{
		stdout: io.Discard, stderr: io.Discard,
		title:           "demo",
		terminal:        screen,
		color:           colorNever,
		visibilityDelay: DelayForTest(0),
	})
	t.Cleanup(func() { _ = out.close() })

	if got := screen.LiveFrameCount(); got == 0 {
		t.Fatal("Init must paint a live frame before any entity is declared")
	}
	if !strings.Contains(screen.LatestLiveText(), "demo") {
		t.Fatalf("armed title line missing subject, got %q", screen.LatestLiveText())
	}
}

// Isolated means "not the package default", not "skip first paint". Nested
// Isolated inits (zq purge/prune preview) used to emit [dry-run] and then
// freeze while Inventory ran — Isolated must still arm.
func TestInit_IsolatedDryRunArmsAndEmitsConstructionFacts(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := initRun(Config{
		title:           "zq",
		subject:         "zq purge",
		isolated:        true,
		DryRun:          true,
		facts:           []factRecord{{Name: "scan roots", Value: "~/Developer"}},
		stdout:          io.Discard,
		stderr:          io.Discard,
		terminal:        screen,
		color:           colorNever,
		visibilityDelay: DelayForTest(0),
	})
	t.Cleanup(func() { _ = out.close() })

	if got := screen.LiveFrameCount(); got == 0 {
		t.Fatal("Isolated Init must paint a live frame before any Task; Isolated is not a first-paint exemption")
	}
	if live := screen.LatestLiveText(); !strings.Contains(live, "zq") {
		t.Fatalf("armed title missing subject, live=%q", live)
	}
	snap := out.snapshot()
	if !snap.DryRun {
		t.Fatal("DryRun must be on the snapshot")
	}
	if len(snap.Facts) != 1 || snap.Facts[0].Name != "scan roots" || snap.Facts[0].Value != "~/Developer" {
		t.Fatalf("construction Facts missing from snapshot: %+v", snap.Facts)
	}
	persisted := screen.PersistedText()
	if !strings.Contains(persisted, "[dry-run]") {
		t.Fatalf("dry-run header missing from persisted text:\n%s", persisted)
	}
	if !strings.Contains(persisted, "scan roots") || !strings.Contains(persisted, "~/Developer") {
		t.Fatalf("construction Facts must persist with the header:\n%s", persisted)
	}
}

// TestDefault_LazyInitNeverPanics runs in a fresh subprocess so defaultOut is
// unset at the top of the run — package funcs before Init must still work.
func TestDefault_LazyInitNeverPanics(t *testing.T) {
	if os.Getenv("EVO_LAZY_DEFAULT_SUBPROCESS") == "1" {
		println("hello without Init")
		task := defaultRun().Task("background")
		succeed(task)
		first := defaultRun()
		second := defaultRun()
		if first != second {
			os.Exit(1)
		}
		os.Exit(0)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestDefault_LazyInitNeverPanics")
	cmd.Env = append(os.Environ(), "EVO_LAZY_DEFAULT_SUBPROCESS=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess with no initRun(Config{}) panicked or failed: %v\n%s", err, out)
	}
}
