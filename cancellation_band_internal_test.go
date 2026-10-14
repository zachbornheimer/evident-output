package evo

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
)

// runInterruptedPrune runs a three-category Group at concurrency 1, interrupts
// while worktrees blocks, and returns the human output. When committed is
// true the first category commits an Effect before the interrupt.
func runInterruptedPrune(t *testing.T, committed bool) (string, int) {
	t.Helper()
	var buf strings.Builder
	out := initRun(Config{Name: "prune", MaxConcurrency: 1, Output: Plain, Facades: Facades{Stdout: io.Discard, Stderr: &buf}}, tuning{color: colorNever})
	interrupt := sendOneSignal(t)

	group := out.Group("categories")
	remote := group.Task("remote-tracking")
	worktrees := group.Task("worktrees")
	branches := group.Task("branches")

	blocking := make(chan struct{})
	code := make(chan int, 1)
	go func() {
		code <- out.run(context.Background(), func(ctx context.Context, r *Run) error {
			remote.Define(func(ctx context.Context) error {
				if !committed {
					return nil
				}
				return Effect(ctx, EffectSpec{Verb: Delete, Object: "stale origin/*", Quantity: 4}, func(context.Context) error { return nil })
			})
			_ = remote.wait()
			worktrees.Define(func(ctx context.Context) error {
				close(blocking)
				<-worktrees.context().Done()
				return nil
			})
			branches.Define(func(ctx context.Context) error { return nil })
			return worktrees.wait()
		}).ExitCode()
	}()

	<-blocking
	interrupt()
	exit := <-code
	return buf.String(), exit
}

func TestCancellation_ExitsWithCancelledCode(t *testing.T) {
	_, exit := runInterruptedPrune(t, true)
	if exit != exitCancelled {
		t.Fatalf("exit %d, want %d", exit, exitCancelled)
	}
}

func TestCancellation_RenderedBandMatchesContract(t *testing.T) {
	var buf strings.Builder
	out := initRun(Config{Name: "prune", MaxConcurrency: 1, Output: Plain, Facades: Facades{Stdout: io.Discard, Stderr: &buf}}, tuning{color: colorNever})
	interrupt := sendOneSignal(t)
	group := out.Group("categories")
	remote := group.Task("remote-tracking")
	worktrees := group.Task("worktrees")
	branches := group.Task("branches")
	blocking := make(chan struct{})
	code := make(chan int, 1)
	go func() {
		code <- out.run(context.Background(), func(ctx context.Context, r *Run) error {
			remote.Define(func(ctx context.Context) error {
				return Effect(ctx, EffectSpec{Verb: Delete, Object: "stale origin/*", Quantity: 4}, func(context.Context) error { return nil })
			})
			_ = remote.wait()
			worktrees.Define(func(ctx context.Context) error {
				close(blocking)
				<-worktrees.context().Done()
				return nil
			})
			branches.Define(func(ctx context.Context) error { return nil })
			return worktrees.wait()
		}).ExitCode()
	}()
	<-blocking
	interrupt()
	<-code

	if got := out.conclusion().Explanation; got != cancelCauseUser {
		t.Fatalf("machine conclusion explanation = %q, want %q", got, cancelCauseUser)
	}

	want := `■ categories
   ✓ remote-tracking
   ■ worktrees        cancelled
   - branches         not started

[changed] remote-tracking  deleted 4 stale origin/*

[cancelled]  prune  by user
!  already mutated: deleted 4 stale origin/*
`
	if got := buf.String(); got != want {
		t.Fatalf("cancellation output mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestCancellation_NothingChangedOmitsThePartialChangesLine(t *testing.T) {
	got, _ := runInterruptedPrune(t, false)
	if strings.Contains(got, "partial changes") || strings.Contains(got, "already mutated") {
		t.Fatalf("no committed effects, so no already-mutated line; got:\n%s", got)
	}
	if !strings.Contains(got, "[cancelled]  prune  by user\n") {
		t.Fatalf("band must read \"[cancelled]  prune  by user\"; got:\n%s", got)
	}
}

// TestCancelledRunUsesOneWordAndAlignedBand pins that an interrupted run says "cancelled" everywhere: the
// interrupted row, the row that never started because of it, and the band, whose tag is followed by the same
// two spaces as every other conclusion band.
func TestCancelledRunUsesOneWordAndAlignedBand(t *testing.T) {
	var buf strings.Builder
	out := initRun(Config{Name: "probe", Output: Plain, Facades: Facades{Stdout: io.Discard, Stderr: &buf}}, tuning{color: colorNever})
	interrupt := sendOneSignal(t)
	long := out.Task("long")
	after := out.Task("after").After(long)
	blocking := make(chan struct{})
	code := make(chan int, 1)
	go func() {
		code <- out.run(context.Background(), func(ctx context.Context, r *Run) error {
			long.Define(func(ctx context.Context) error {
				close(blocking)
				<-long.context().Done()
				return nil
			})
			after.Define(func(ctx context.Context) error { return nil })
			return long.wait()
		}).ExitCode()
	}()
	<-blocking
	interrupt()
	<-code

	got := buf.String()
	if strings.Contains(got, "interrupted") {
		t.Errorf("a cancelled run says \"interrupted\" where the Outcome is cancelled:\n%s", got)
	}
	for _, want := range []string{"■ long   cancelled\n", "- after  not started — long cancelled\n", "\n[cancelled]  probe  by user\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("cancelled run output lacks %q:\n%s", want, got)
		}
	}
}

// TestCancelledBandMatchesContractShape pins CONTRACT section 4: a Cancelled
// run ends with the band `[cancelled]  <name>  <cause>`, and a mutation it
// leaves behind is stated the way a Failed run states it,
// `!  already mutated: <ledger summary>`, so one fact has one spelling. The
// shape is in the contract text, not only in the code.
func TestCancelledBandMatchesContractShape(t *testing.T) {
	got, _ := runInterruptedPrune(t, true)
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	band := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "[cancelled]") {
			band = i
		}
	}
	if band < 0 {
		t.Fatalf("no cancelled band in:\n%s", got)
	}
	if want := "[cancelled]  prune  " + cancelCauseUser; lines[band] != want {
		t.Errorf("band line = %q, want %q", lines[band], want)
	}
	if want := "!  already mutated: deleted 4 stale origin/*"; band+1 >= len(lines) || lines[band+1] != want {
		t.Errorf("line under the band = %q, want %q", lines[band+1:], want)
	}
	if strings.Contains(got, "partial changes") {
		t.Errorf("a second spelling of the mutated fact remains:\n%s", got)
	}
	contract, err := os.ReadFile("CONTRACT.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, shape := range []string{"`[cancelled]  <name>  by user`", "`!  already mutated: <summary>`"} {
		if !strings.Contains(string(contract), shape) {
			t.Errorf("CONTRACT.md does not state the shape %s", shape)
		}
	}
}
