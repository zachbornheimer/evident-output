package engine

import (
	"context"
	"errors"
	"io"
	"testing"
)

func quietOutput(cfg Config) *Output {
	cfg.Isolated, cfg.Plain, cfg.Color = true, true, ColorNever
	cfg.Stdout, cfg.Stderr = io.Discard, io.Discard
	return Init(cfg)
}

// waiter is anything whose Wait answers "did the declared work succeed".
type waiter interface{ Wait() error }

// TestRejectedDeclaration_WaitNeverReportsSuccess pins that every rejected
// declaration hands back a handle whose Wait says the work never ran:
// gating a destructive next step on Wait() == nil must not proceed.
func TestRejectedDeclaration_WaitNeverReportsSuccess(t *testing.T) {
	cases := map[string]struct {
		cfg      Config
		declare  func(o *Output) waiter
		rejected error
	}{
		"duplicate root task": {declare: func(o *Output) waiter {
			o.Task("x")
			return definedNoop(o.Task("x"))
		}, rejected: ErrDuplicateSiblingName},
		"duplicate key": {declare: func(o *Output) waiter {
			o.taskScoped("a", "", ID("k"))
			return definedNoop(o.taskScoped("b", "", ID("k")))
		}, rejected: ErrDuplicateKey},
		"limit exceeded": {cfg: Config{MaxEntities: 1}, declare: func(o *Output) waiter {
			o.Task("a")
			return definedNoop(o.Task("b"))
		}, rejected: ErrLimitExceeded},
		"closed": {declare: func(o *Output) waiter {
			_ = o.Close()
			return definedNoop(o.Task("late"))
		}, rejected: ErrClosed},
		"duplicate group task": {declare: func(o *Output) waiter {
			g := o.Group("g")
			g.Task("x")
			return definedNoop(g.Task("x"))
		}, rejected: ErrDuplicateSiblingName},
		"duplicate root group": {declare: func(o *Output) waiter {
			o.Group("g")
			g2 := o.Group("g")
			definedNoop(g2.Task("child"))
			return g2
		}, rejected: ErrDuplicateSiblingName},
		"duplicate root sequence": {declare: func(o *Output) waiter {
			o.Sequence("s")
			s2 := o.Sequence("s")
			definedNoop(s2.Task("child"))
			return s2
		}, rejected: ErrDuplicateSiblingName},
		"duplicate nested group": {declare: func(o *Output) waiter {
			g := o.Group("g")
			g.Group("n")
			n2 := g.Group("n")
			definedNoop(n2.Task("child"))
			return n2
		}, rejected: ErrDuplicateSiblingName},
		"task under a rejected group": {declare: func(o *Output) waiter {
			o.Group("g")
			return definedNoop(o.Group("g").Task("child"))
		}, rejected: ErrDuplicateSiblingName},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			out := quietOutput(tc.cfg)
			t.Cleanup(func() { _ = out.Close() })
			err := tc.declare(out).Wait()
			if !errors.Is(err, ErrNotStarted) || !errors.Is(err, tc.rejected) {
				t.Fatalf("Wait on a rejected declaration = %v, want ErrNotStarted wrapping %v", err, tc.rejected)
			}
		})
	}
}

// TestRejectedDeclaration_DefineRecordsMisuse: Define on a rejected handle
// never runs fn and is misuse, not a silent drop.
func TestRejectedDeclaration_DefineRecordsMisuse(t *testing.T) {
	out := quietOutput(Config{})
	t.Cleanup(func() { _ = out.Close() })
	out.Task("x")
	ran := false
	dup := out.Task("x")
	out.mu.Lock()
	out.misuse = nil
	out.mu.Unlock()
	dup.Define(func(context.Context) error { ran = true; return nil })
	_ = out.Finish()
	if ran {
		t.Fatal("Define on a rejected handle ran its callback")
	}
	if err := misuseOf(out); !errors.Is(err, ErrDuplicateSiblingName) {
		t.Fatalf("Define on a rejected handle must record misuse, got %v", err)
	}
}

func definedNoop(h *TaskHandle) *TaskHandle {
	return h.Define(func(context.Context) error { return nil })
}
