package engine

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// TestPartialEffect_InterruptAfterPartialCommitKeepsTheCommittedSubset pins
// ZYS-972's cancellation case: ^C lands while an Effect callback has
// committed 2 of 3 refs; the callback reports PartialEffect(2, ctx.Err()),
// and the cancelled run still owes the reader those 2 changed refs.
func TestPartialEffect_InterruptAfterPartialCommitKeepsTheCommittedSubset(t *testing.T) {
	var buf strings.Builder
	out := Init(Config{
		Isolated: true, Plain: true, Color: ColorNever,
		MaxConcurrency: 1, Stdout: &buf, Stderr: io.Discard,
	})
	interrupt := sendOneSignal(t)

	committedTwo := make(chan struct{})
	var effectErr error
	code := make(chan int, 1)
	go func() {
		code <- out.Run(context.Background(), func(ctx context.Context) error {
			task := out.Task("refs")
			task.Define(func(ctx context.Context) error {
				spec := EffectSpec{Verb: EffectDelete, Object: "remote ref", Quantity: 3}
				effectErr = Effect(ctx, spec, func(ctx context.Context) error {
					close(committedTwo) // refs 1 and 2 are gone
					<-ctx.Done()
					return PartialEffect(2, ctx.Err())
				})
				return effectErr
			})
			return task.Wait()
		}).ExitCode()
	}()

	<-committedTwo
	interrupt()

	var got int
	select {
	case got = <-code:
	case <-time.After(interruptBudget):
		t.Fatalf("interrupt did not stop the run; output so far:\n%s", buf.String())
	}
	if got != ExitCancelled {
		t.Fatalf("exit %d, want %d (ExitCancelled); output:\n%s", got, ExitCancelled, buf.String())
	}
	if !errors.Is(effectErr, context.Canceled) {
		t.Fatalf("Effect err = %v, want context.Canceled kept reachable", effectErr)
	}
	changes := out.Snapshot().Changes
	if len(changes) != 1 || len(changes[0].Records) != 1 || changes[0].Records[0].Quantity != 2 {
		t.Fatalf("changes = %+v, want one changed record with quantity 2", changes)
	}
	if rendered := strings.Join(strings.Fields(buf.String()), " "); !strings.Contains(rendered, "deleted 2 remote refs") {
		t.Fatalf("cancelled run must still report the 2 committed refs:\n%s", buf.String())
	}
}
