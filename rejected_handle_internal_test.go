package evo

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/misuse"
)

func quietOutput(cfg Config) *Run {
	cfg.isolated, cfg.plain, cfg.color = true, true, colorNever
	cfg.stdout, cfg.stderr = io.Discard, io.Discard
	return initRun(cfg)
}

// waiter is anything whose Wait answers "did the declared work succeed".
type waiter interface{ wait() error }

// TestRejectedDeclaration_WaitNeverReportsSuccess pins that every rejected
// declaration hands back a handle whose Wait says the work never ran:
// gating a destructive next step on Wait() == nil must not proceed.
func TestRejectedDeclaration_WaitNeverReportsSuccess(t *testing.T) {
	cases := map[string]struct {
		cfg      Config
		declare  func(o *Run) waiter
		rejected error
	}{
		"duplicate root task": {declare: func(o *Run) waiter {
			o.Task("x")
			return definedNoop(o.Task("x"))
		}, rejected: errDuplicateSiblingName},
		"limit exceeded": {cfg: Config{maxEntities: 1}, declare: func(o *Run) waiter {
			o.Task("a")
			return definedNoop(o.Task("b"))
		}, rejected: errLimitExceeded},
		"closed": {declare: func(o *Run) waiter {
			_ = o.close()
			return definedNoop(o.Task("late"))
		}, rejected: errClosed},
		"duplicate group task": {declare: func(o *Run) waiter {
			g := o.Group("g")
			g.Task("x")
			return definedNoop(g.Task("x"))
		}, rejected: errDuplicateSiblingName},
		"duplicate root group": {declare: func(o *Run) waiter {
			o.Group("g")
			g2 := o.Group("g")
			definedNoop(g2.Task("child"))
			return g2
		}, rejected: errDuplicateSiblingName},
		"duplicate root sequence": {declare: func(o *Run) waiter {
			o.Sequence("s")
			s2 := o.Sequence("s")
			definedNoop(s2.Task("child"))
			return s2
		}, rejected: errDuplicateSiblingName},
		"duplicate nested group": {declare: func(o *Run) waiter {
			g := o.Group("g")
			g.Group("n")
			n2 := g.Group("n")
			definedNoop(n2.Task("child"))
			return n2
		}, rejected: errDuplicateSiblingName},
		"task under a rejected group": {declare: func(o *Run) waiter {
			o.Group("g")
			return definedNoop(o.Group("g").Task("child"))
		}, rejected: errDuplicateSiblingName},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			out := quietOutput(tc.cfg)
			t.Cleanup(func() { _ = out.close() })
			err := tc.declare(out).wait()
			if !errors.Is(err, errNotStarted) || !errors.Is(err, tc.rejected) {
				t.Fatalf("Wait on a rejected declaration = %v, want ErrNotStarted wrapping %v", err, tc.rejected)
			}
		})
	}
}

// TestRejectedDeclaration_DefineRecordsMisuse: Define on a rejected handle
// never runs fn and is misuse, not a silent drop.
func TestRejectedDeclaration_DefineRecordsMisuse(t *testing.T) {
	out := quietOutput(Config{})
	t.Cleanup(func() { _ = out.close() })
	out.Task("x")
	ran := false
	dup := out.Task("x")
	out.mu.Lock()
	out.misuse = misuse.New(false)
	out.mu.Unlock()
	dup.Define(func(context.Context) error { ran = true; return nil })
	_ = out.finish()
	if ran {
		t.Fatal("Define on a rejected handle ran its callback")
	}
	if err := misuseOf(out); !errors.Is(err, errDuplicateSiblingName) {
		t.Fatalf("Define on a rejected handle must record misuse, got %v", err)
	}
}

func definedNoop(h *Task) *Task {
	return h.Define(func(context.Context) error { return nil })
}
