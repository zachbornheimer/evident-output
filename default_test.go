package evo_test

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/testkit"
)

func TestTask_PackageFuncGetOrCreateReturnsSameHandle(t *testing.T) {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}}))

	a := evo.Task("branches")
	b := evo.Task("branches")

	if a != b {
		t.Fatal("evo.Task(name) called twice must return the same *TaskHandle")
	}
	if a.Snapshot().ID != b.Snapshot().ID {
		t.Fatalf("same handle reported different ids: %q vs %q", a.Snapshot().ID, b.Snapshot().ID)
	}

	other := evo.Task("worktrees")
	if other == a {
		t.Fatal("a different name must not reuse the same handle")
	}
}

func TestPackageFuncs_DelegateToDefaultInstance(t *testing.T) {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}}))

	evo.Task("working tree").Done()
	evo.Println("hello from package func")

	if err := evo.Default().Finish(); err != nil {
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
	evo.SetDefault(evo.Init(evo.Config{
		Stdout:    &buf,
		Stderr:    &buf,
		Verbosity: evo.VerbosityVerbose,
	}))

	evo.Verbose().Println("verbose only line")
	if err := evo.Default().Finish(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "verbose only line") {
		t.Fatalf("missing verbose line:\n%s", buf.String())
	}
}

func TestMain_OKExitZero(t *testing.T) {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}}))

	code := evo.Main(func() error {
		evo.Task("working tree").Done()
		return nil
	})
	if code != evo.ExitOK {
		t.Fatalf("exit %d, want %d; out:\n%s", code, evo.ExitOK, buf.String())
	}
}

func TestMain_BlockedExitOne(t *testing.T) {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}}))

	code := evo.Main(func() error {
		evo.Task("working tree").Block("dirty")
		return nil
	})
	if code != evo.ExitBlocked {
		t.Fatalf("exit %d, want %d", code, evo.ExitBlocked)
	}
}

func TestMain_FailedExitTwo(t *testing.T) {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}}))

	code := evo.Main(func() error {
		return errors.New("app boom")
	})
	if code != evo.ExitFailed {
		t.Fatalf("exit %d, want %d", code, evo.ExitFailed)
	}
}

func TestMain_NilRunNeverPanics(t *testing.T) {
	var buf bytes.Buffer
	evo.SetDefault(evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.NoColor()}}))

	code := evo.Main(nil)
	if code != evo.ExitOK {
		t.Fatalf("exit %d, want %d", code, evo.ExitOK)
	}
}

func TestInit_ArmsFirstPaintBeforeAnyEntity(t *testing.T) {
	screen := testkit.NewScreen(testkit.Interactive(), testkit.Width(80), testkit.NoColor())
	out := evo.Init(evo.Config{
		Title:           "demo",
		Terminal:        screen,
		Color:           evo.ColorNever,
		VisibilityDelay: evo.Delay(0),
	})
	t.Cleanup(func() { _ = out.Close() })

	if got := screen.LiveFrameCount(); got == 0 {
		t.Fatal("Init must paint a live frame before any entity is declared")
	}
	if !strings.Contains(screen.LatestLiveText(), "demo") {
		t.Fatalf("armed title line missing subject, got %q", screen.LatestLiveText())
	}
}

// TestDefault_LazyInitNeverPanics runs in a fresh subprocess so defaultOut is
// unset at the top of the run — package funcs before Init must still work.
func TestDefault_LazyInitNeverPanics(t *testing.T) {
	if os.Getenv("EVO_LAZY_DEFAULT_SUBPROCESS") == "1" {
		evo.Println("hello without Init")
		task := evo.Task("background")
		task.Done()
		first := evo.Default()
		second := evo.Default()
		if first != second {
			os.Exit(1)
		}
		os.Exit(0)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestDefault_LazyInitNeverPanics")
	cmd.Env = append(os.Environ(), "EVO_LAZY_DEFAULT_SUBPROCESS=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("subprocess with no Init() panicked or failed: %v\n%s", err, out)
	}
}
