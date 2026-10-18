package evo_test

import (
	"context"
	"strings"
	"testing"
	"time"

	evo "github.com/zachbornheimer/evident-output/v2"
)

// settleWait bounds how long the slower Task waits to see the faster one's ending on a stream.
const settleWait = 30 * time.Second

// twoFailuresCeiling lets the slow Task hold a slot while the fast one fails,
// regardless of GOMAXPROCS.
const twoFailuresCeiling = 2

// TestExplanationWithTwoFailuresIsDeterministic pins CONTRACT section 4: when several Tasks end the run's
// way, Conclusion.Explanation is the ending of the one that settled first, whatever the declaration order.
// "slow" is declared first but settles after "fast", so "fast failed" must decide in every Output.
func TestExplanationWithTwoFailuresIsDeterministic(t *testing.T) {
	for _, mode := range contractOutputs {
		t.Run(outputName[mode], func(t *testing.T) {
			var stdout, stderr lockedBuffer
			cfg := evo.Config{Name: "explanation", Output: mode, MaxConcurrency: twoFailuresCeiling, Facades: evo.Facades{Stdout: &stdout, Stderr: &stderr}}
			fastEnded := make(chan struct{})
			res := evo.Execute(context.Background(), cfg, func(_ context.Context, r *evo.Run) error {
				r.Task("slow").Define(func(context.Context) error {
					<-fastEnded
					awaitText(t, func() string { return stdout.String() + stderr.String() }, "fast failed")
					return evo.Fail("slow failed")
				})
				r.Task("fast").Define(func(context.Context) error {
					defer close(fastEnded)
					return evo.Fail("fast failed")
				})
				return nil
			})

			got := res.Conclusion.Explanation
			if got != "fast failed" {
				t.Errorf("Conclusion.Explanation = %q, want %q: the Task that settled first decides", got, "fast failed")
			}
			switch mode {
			case evo.JSON:
				assertWireExplanation(t, "evo.run explanation", runDocument(t, stdout.String())["explanation"], got)
			case evo.JSONL:
				for _, event := range eventsOfType(runEvents(t, stdout.String()), "run.finished") {
					assertWireExplanation(t, "run.finished explanation", payload(event)["explanation"], got)
				}
			}
		})
	}
}

// awaitText polls read until it holds want, so the caller continues only once want reached a stream.
func awaitText(t *testing.T, read func() string, want string) {
	t.Helper()
	deadline := time.Now().Add(settleWait)
	for !strings.Contains(read(), want) {
		if time.Now().After(deadline) {
			t.Errorf("%q never reached a stream within %s", want, settleWait)
			return
		}
		time.Sleep(time.Millisecond)
	}
}
