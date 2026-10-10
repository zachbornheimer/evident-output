package mcp_test

import (
	"testing"

	"github.com/zachbornheimer/evident-output/mcp/internal/agent/review"
)

// GUESSED SIGNATURES (ZYS-1369; none are settled):
//
//	review.GapEvent{Signature string, Shape string, DesiredVersion string}
//	review.GapSink interface{ Emit(GapEvent) }
//	review.GoSourceWithGapSink(filename, src, desiredVersion string, sink GapSink) review.Result
type recordingGapSink struct{ events []review.GapEvent }

func (s *recordingGapSink) Emit(e review.GapEvent) { s.events = append(s.events, e) }

// A consumer pinned to 1.1.0 asks for typed dataflow that release cannot
// express: Compute does not exist there.
const inexpressibleShapeSrc = `package p

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func run(out *evo.Output) {
	inventory := evo.Compute(out.Task("discover installed packages"), discoverPackages)
	out.Task("centralize packages").After(inventory).Define(func(ctx context.Context) error { return nil })
}
`

const gapPin = "1.1.0"

func TestCMCP_009_InexpressibleShapeKeepsRecheckAndEmitsDedupedGapEvent(t *testing.T) {
	sink := &recordingGapSink{}
	res := review.GoSourceWithGapSink("run.go", inexpressibleShapeSrc, gapPin, sink)
	if !res.RecheckRequired {
		t.Fatal("an inexpressible shape must keep recheck_required=true")
	}
	if len(sink.events) != 1 || sink.events[0].Signature == "" {
		t.Fatalf("want exactly one gap event with a signature, got %+v", sink.events)
	}
	review.GoSourceWithGapSink("run.go", inexpressibleShapeSrc, gapPin, sink)
	if len(sink.events) != 1 {
		t.Fatalf("a repeat of the same gap must be deduped by signature, got %d events", len(sink.events))
	}
}
