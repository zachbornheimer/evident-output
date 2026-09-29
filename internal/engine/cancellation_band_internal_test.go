package engine

import (
	"context"
	"io"
	"strings"
	"testing"
)

// runInterruptedPrune runs a three-category Group at concurrency 1, interrupts
// while worktrees blocks, and returns the human output. When committed is
// true the first category commits an Effect before the interrupt.
func runInterruptedPrune(t *testing.T, committed bool) (string, int) {
	t.Helper()
	var buf strings.Builder
	out := Init(Config{
		Isolated: true, Plain: true, Color: ColorNever, Title: "prune",
		MaxConcurrency: 1, Stdout: &buf, Stderr: io.Discard,
	})
	interrupt := sendOneSignal(t)

	group := out.Group("categories")
	remote := group.Task("remote-tracking")
	worktrees := group.Task("worktrees")
	branches := group.Task("branches")

	blocking := make(chan struct{})
	code := make(chan int, 1)
	go func() {
		code <- out.Run(context.Background(), func(ctx context.Context) error {
			remote.Define(func(ctx context.Context) error {
				if !committed {
					return nil
				}
				return Effect(ctx, EffectSpec{Verb: EffectDelete, Object: "stale origin/*", Quantity: 4}, func(context.Context) error { return nil })
			})
			_ = remote.Wait()
			worktrees.Define(func(ctx context.Context) error {
				close(blocking)
				<-worktrees.Context().Done()
				return nil
			})
			branches.Define(func(ctx context.Context) error { return nil })
			return worktrees.Wait()
		}).ExitCode()
	}()

	<-blocking
	interrupt()
	exit := <-code
	return buf.String(), exit
}

func TestCancellation_ExitsWithCancelledCode(t *testing.T) {
	_, exit := runInterruptedPrune(t, true)
	if exit != ExitCancelled {
		t.Fatalf("exit %d, want %d", exit, ExitCancelled)
	}
}

func TestCancellation_RenderedBandMatchesContract(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{
		Isolated: true, Plain: true, Color: ColorNever, Title: "prune",
		MaxConcurrency: 1, Stdout: &buf, Stderr: io.Discard,
	})
	interrupt := sendOneSignal(t)
	group := out.Group("categories")
	remote := group.Task("remote-tracking")
	worktrees := group.Task("worktrees")
	branches := group.Task("branches")
	blocking := make(chan struct{})
	code := make(chan int, 1)
	go func() {
		code <- out.Run(context.Background(), func(ctx context.Context) error {
			remote.Define(func(ctx context.Context) error {
				return Effect(ctx, EffectSpec{Verb: EffectDelete, Object: "stale origin/*", Quantity: 4}, func(context.Context) error { return nil })
			})
			_ = remote.Wait()
			worktrees.Define(func(ctx context.Context) error {
				close(blocking)
				<-worktrees.Context().Done()
				return nil
			})
			branches.Define(func(ctx context.Context) error { return nil })
			return worktrees.Wait()
		}).ExitCode()
	}()
	<-blocking
	interrupt()
	<-code

	if got := out.Conclusion().Explanation; got != cancelCauseUser {
		t.Fatalf("machine conclusion explanation = %q, want %q", got, cancelCauseUser)
	}

	want := `✓ remote-tracking
■ worktrees        interrupted
- branches         not started

[changed] remote-tracking  deleted 4 stale origin/*

[cancelled] prune  by user
  ! partial changes were applied before cancellation
`
	if got := buf.String(); got != want {
		t.Fatalf("cancellation output mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestCancellation_NothingChangedOmitsThePartialChangesLine(t *testing.T) {
	got, _ := runInterruptedPrune(t, false)
	if strings.Contains(got, "partial changes") || strings.Contains(got, "already mutated") {
		t.Fatalf("no committed effects, so no partial-changes line; got:\n%s", got)
	}
	if !strings.Contains(got, "[cancelled] prune  by user\n") {
		t.Fatalf("band must read \"[cancelled] prune  by user\"; got:\n%s", got)
	}
}
