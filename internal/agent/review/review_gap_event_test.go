package review_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

const computeSrc = `package p

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func run(out *evo.Output) {
	inventory := evo.Compute(out.Task("discover"), discover)
	out.Task("use").After(inventory).Define(func(ctx context.Context) error { return nil })
}
`

func TestAPI072_OlderPinGetsAGapFindingWithNoSuggestion(t *testing.T) {
	res := review.GoSourceAt("run.go", computeSrc, "1.1.0")
	f := assertFinding(t, res, "API-072")
	if f.Suggestion != "" {
		t.Fatalf("a product gap must not invent a workaround, got suggestion %q", f.Suggestion)
	}
	if !res.RecheckRequired {
		t.Fatal("a gap must keep recheck_required=true")
	}
}

func TestAPI072_CurrentDialectHasNoGap(t *testing.T) {
	assertNoFinding(t, review.GoSource("run.go", computeSrc), "API-072")
	assertNoFinding(t, review.GoSourceAt("run.go", computeSrc, "1.2.0"), "API-072")
}

func TestGapSink_DedupesPerSinkAndSignature(t *testing.T) {
	first, second := &review.MemoryGapSink{}, &review.MemoryGapSink{}
	for range 3 {
		review.GoSourceWithGapSink("run.go", computeSrc, "1.1.0", first)
	}
	review.GoSourceWithGapSink("run.go", computeSrc, "1.1.0", second)
	if n := len(first.Events()); n != 1 {
		t.Fatalf("first sink got %d events, want 1", n)
	}
	if n := len(second.Events()); n != 1 {
		t.Fatalf("a different sink has its own ledger, got %d events, want 1", n)
	}
	review.GoSourceWithGapSink("run.go", computeSrc, "1.0.5", first)
	if n := len(first.Events()); n != 2 {
		t.Fatalf("a different pin is a different gap, got %d events, want 2", n)
	}
}

func TestGapSink_NoEventWhenTheShapeIsExpressible(t *testing.T) {
	sink := &review.MemoryGapSink{}
	review.GoSourceWithGapSink("run.go", computeSrc, "", sink)
	review.GoSourceWithGapSink("run.go", computeSrc, "1.2.0", sink)
	if n := len(sink.Events()); n != 0 {
		t.Fatalf("got %d events, want none", n)
	}
}
