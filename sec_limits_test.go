package evo_test

import (
	"errors"
	"io"
	"math"
	"strings"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
	txt "github.com/zachbornheimer/evident-output/internal/text"
)

func TestSEC003_MaxEntitiesEnforced(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard, MaxEntities: 3})
	t.Cleanup(func() { _ = out.Close() })
	succeed(out.Task("a"))
	succeed(out.Task("b"))
	succeed(out.Task("c"))
	succeed(out.Task("d")) // should record limit
	if !errors.Is(out.Err(), evo.ErrLimitExceeded) {
		t.Fatalf("err=%v", out.Err())
	}
}

func TestSEC005_ProgressOverflowRejected(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
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
	out := evo.Init(evo.Config{Isolated: true, Stdout: io.Discard})
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
	out := evo.Init(evo.Config{Isolated: true, Stdout: &buf, Stderr: &buf, Debug: evo.DebugConfig{Level: evo.LevelDebug}, Plain: true})
	t.Cleanup(func() { _ = out.Close() })
	out.DebugForTest("auth", evo.Field{Key: "token", Value: "super-secret", Sensitive: true})
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
	got := txt.Text("safe\u202Eevil")
	if strings.ContainsRune(got, '\u202e') {
		t.Fatalf("bidi retained: %q", got)
	}
}
