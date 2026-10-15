package evo

import (
	"context"
	"strings"
	"testing"
)

// TestUnorderedGetExplanationNeverSaysPanic pins CONTRACT.md §8 (evo never crashes): a Computed read the
// declared order does not prove ends the run with an Explanation that names the refused read, never a
// "panic" the caller did not write.
func TestUnorderedGetExplanationNeverSaysPanic(t *testing.T) {
	out := newQuietOutput(t)
	makeTask := out.Group("producers").Task("make")
	producer := Compute(makeTask, func(context.Context) (int, error) { return 1, nil })
	out.Sequence("consumers").Task("read").Define(func(context.Context) error {
		_ = makeTask.wait()
		_ = producer.Get()
		return nil
	})

	_ = out.finish()

	explanation := out.decodedRunDocument(t).Explanation
	if !strings.Contains(explanation, ErrComputedUnordered.Error()) {
		t.Errorf("Explanation = %q, want it to name %q", explanation, ErrComputedUnordered)
	}
	if strings.Contains(strings.ToLower(explanation), "panic") {
		t.Errorf("Explanation = %q, which says panic; a refused read is not a crash", explanation)
	}
}
