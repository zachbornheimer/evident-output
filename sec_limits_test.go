package evo_test

import (
	"errors"
	"io"
	"math"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	"github.com/zachbornheimer/evident-output/internal/sanitize"
)

func TestSEC003_MaxEntitiesEnforced(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard), evo.MaxEntities(3)}})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("a").Done()
	out.Task("b").Done()
	out.Task("c").Done()
	out.Task("d").Done() // should record limit
	if !errors.Is(out.Err(), evo.ErrLimitExceeded) {
		t.Fatalf("err=%v", out.Err())
	}
}

func TestSEC005_ProgressOverflowRejected(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("t")
	// Valid absolute max equal values.
	task.Progress(math.MaxInt64, math.MaxInt64)
	// Completed past the sealed total must record misuse (C7: Advance/
	// Progress64 deleted — Progress is the sole absolute-count API now, so
	// the overflow guard is exercised directly with a completed > total call).
	task.Progress(math.MaxInt64, math.MaxInt64-1)
	if !errors.Is(out.Err(), evo.ErrInvalidProgress) {
		t.Fatalf("expected ErrInvalidProgress after completed exceeds the sealed total, got %v", out.Err())
	}
	// Last valid progress preserved.
	got := task.Snapshot().Progress
	if got.Completed != math.MaxInt64 || got.Total != math.MaxInt64 {
		t.Fatalf("last valid progress corrupted: %+v", got)
	}
}

func TestSEC007_DestructiveActionFlag(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	a := evo.Action{
		Label:       "delete everything",
		Destructive: true,
		Command:     &evo.CommandSpec{Executable: "rm", Args: []string{"-rf", "/"}},
	}
	item := out.Task("x")
	item.Block("danger")
	item.Next(a)
	_ = out.Finish()
	c := out.Conclusion()
	found := false
	for _, act := range c.Actions {
		if act.Destructive {
			found = true
		}
	}
	// also check item actions before promotion
	for _, it := range out.Snapshot().Tasks {
		for _, act := range it.Actions {
			if act.Destructive {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("destructive flag lost")
	}
}

func TestSEC002_SensitiveFieldRedactedInDebug(t *testing.T) {
	var buf strings.Builder
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(&buf), evo.Plain(), evo.DebugLevel(evo.Debug)}})
	t.Cleanup(func() { _ = out.Close() })
	out.Debug("auth", evo.Field{Key: "token", Value: "super-secret", Sensitive: true})
	_ = out.Finish()
	if strings.Contains(buf.String(), "super-secret") {
		t.Fatal("secret leaked")
	}
	if !strings.Contains(buf.String(), "***") {
		t.Fatal(buf.String())
	}
}

func TestSEC011_BidiControlsStripped(t *testing.T) {
	// U+202E RTL override
	got := sanitize.Text("safe\u202Eevil")
	if strings.ContainsRune(got, '\u202e') {
		t.Fatalf("bidi retained: %q", got)
	}
}

func TestDOM005_DuplicateKeyRejected(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("one", evo.ID("k"))
	out.Task("two", evo.ID("k"))
	if !errors.Is(out.Err(), evo.ErrDuplicateKey) {
		t.Fatalf("err=%v", out.Err())
	}
}

func TestDOM024_TotalDecreaseBelowCompletedRejected(t *testing.T) {
	out := evo.Init(evo.Config{Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("t")
	task.Progress(5, 10)
	task.Progress(5, 3) // total < completed
	if !errors.Is(out.Err(), evo.ErrInvalidProgress) {
		t.Fatalf("err=%v", out.Err())
	}
	if task.Snapshot().Progress.Total != 10 {
		t.Fatal("last valid total not preserved")
	}
}
