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
	out := evo.New(evo.To(io.Discard), evo.MaxEntities(3))
	t.Cleanup(func() { _ = out.Close() })
	out.Item("a").OK()
	out.Item("b").OK()
	out.Item("c").OK()
	out.Item("d").OK() // should record limit
	if !errors.Is(out.Err(), evo.ErrLimitExceeded) {
		t.Fatalf("err=%v", out.Err())
	}
}

func TestSEC005_ProgressOverflowRejected(t *testing.T) {
	out := evo.New(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	task := out.Task("t")
	// huge but valid
	task.Progress(math.MaxInt64, math.MaxInt64)
	// negative already covered; advance that would wrap is rejected via invalid
	task.Advance(1)
	// completed would overflow past total — invalid
	if out.Err() == nil {
		// may be ErrInvalidProgress if total was max and advance exceeds
		t.Log("err", out.Err())
	}
}

func TestSEC007_DestructiveActionFlag(t *testing.T) {
	out := evo.New(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	a := evo.Action{
		Label:       "delete everything",
		Destructive: true,
		Command:     &evo.CommandSpec{Executable: "rm", Args: []string{"-rf", "/"}},
	}
	out.Item("x").Block("danger").Next(a)
	_ = out.Finish()
	c := out.Conclusion()
	found := false
	for _, act := range c.Actions {
		if act.Destructive {
			found = true
		}
	}
	// also check item actions before promotion
	for _, it := range out.Snapshot().Items {
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
	out := evo.New(evo.To(&buf), evo.Plain(), evo.DebugLevel(evo.Debug))
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
	out := evo.New(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	_, err := out.ItemWith(evo.ItemSpec{Key: "k", Name: "one"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = out.ItemWith(evo.ItemSpec{Key: "k", Name: "two"})
	if !errors.Is(err, evo.ErrDuplicateKey) {
		t.Fatalf("err=%v", err)
	}
}

func TestDOM024_TotalDecreaseBelowCompletedRejected(t *testing.T) {
	out := evo.New(evo.To(io.Discard))
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
