package render

import (
	"strings"
	"testing"
	"time"

	"github.com/zachbornheimer/evident-output/internal/core"
)

// A span too small to show at display precision says nothing: the zq
// canary rendered "0ms waiting on capacity" for a 40µs scheduler wait.
func TestWriteMetrics_OmitsSpansBelowDisplayPrecision(t *testing.T) {
	var b strings.Builder
	WriteMetrics(&b, core.RunMetrics{Executed: 1, Running: 2 * time.Second, SchedulerWait: 40 * time.Microsecond}, false)
	const want = "timing  1 executed · 2s running\n"
	if b.String() != want {
		t.Fatalf("WriteMetrics = %q, want %q", b.String(), want)
	}
}

func TestWriteMetrics_NamesEveryBucketOnce(t *testing.T) {
	var b strings.Builder
	WriteMetrics(&b, core.RunMetrics{
		Executed:       1,
		Operations:     core.OperationCounts{Current: 1, Executed: 1, BasisDrift: 1, Unchanged: 1},
		Running:        3 * time.Second,
		DependencyWait: time.Second,
		Definition:     2 * time.Second,
		Provenance:     250 * time.Millisecond,
		Evidence:       100 * time.Millisecond,
		TrackedState:   150 * time.Millisecond,
		CriticalPath:   3 * time.Second,
	}, false)
	const want = "timing  1 executed · 1 of 2 operations current · 1 basis changed · 1 identical outputs · " +
		"3s running · 1s waiting on dependencies · " +
		"2s in definitions (250ms checking provenance, 150ms verifying tracked state) · " +
		"100ms evaluating Verify · 3s critical path\n"
	if b.String() != want {
		t.Fatalf("WriteMetrics =\n%q\nwant\n%q", b.String(), want)
	}
}

func TestWriteMetrics_EmptyRunWritesNothing(t *testing.T) {
	var b strings.Builder
	WriteMetrics(&b, core.RunMetrics{}, false)
	if b.Len() != 0 {
		t.Fatalf("WriteMetrics(zero) = %q, want nothing", b.String())
	}
}

// Provenance and TrackedState are spent inside Define callbacks, so they
// are part of Definition; Evidence (Verify) is not. The line must show that
// nesting, or a reader adding its clauses together double-counts.
func TestWriteMetrics_NestsProvenanceAndTrackedStateInsideDefinitions(t *testing.T) {
	var b strings.Builder
	WriteMetrics(&b, core.RunMetrics{
		Definition:   time.Second,
		Provenance:   400 * time.Millisecond,
		TrackedState: 300 * time.Millisecond,
		Evidence:     200 * time.Millisecond,
	}, false)
	const want = "timing  1s in definitions (400ms checking provenance, 300ms verifying tracked state) · " +
		"200ms evaluating Verify\n"
	if b.String() != want {
		t.Fatalf("WriteMetrics =\n%q\nwant\n%q", b.String(), want)
	}
}

func TestWriteMetrics_DefinitionsWithoutNestedTimeHaveNoParenthetical(t *testing.T) {
	var b strings.Builder
	WriteMetrics(&b, core.RunMetrics{Definition: 3 * time.Second, Provenance: 10 * time.Microsecond}, false)
	const want = "timing  3s in definitions\n"
	if b.String() != want {
		t.Fatalf("WriteMetrics = %q, want %q", b.String(), want)
	}
}
