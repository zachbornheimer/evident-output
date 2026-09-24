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
		"3s running · 1s waiting on dependencies · 2s in definitions · 250ms checking provenance · " +
		"250ms verifying tracked state · 3s critical path\n"
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
