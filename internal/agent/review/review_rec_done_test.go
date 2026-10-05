package review_test

import (
	"strings"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/agent/review"
)

// API-032 (ZYS-812): TaskHandle.Done was removed in 1.1. Every call site
// gets its Define/Summary rewrite; context/WaitGroup Done never matches.

const removedDoneSrc = `package p

import (
	"context"
	"sync"

	evo "github.com/zachbornheimer/evident-output"
)

func run(ctx context.Context, wg *sync.WaitGroup, n int) {
	evo.Task("working tree").Done()
	branches := evo.Task("branches")
	branches.Done("3 checked")
	remotes := evo.Task("remotes")
	remotes.Done("%d stale refs", n)
	scan := evo.Task("scan")
	scan.Define(func(ctx context.Context) error {
		scan.Done("7 repos")
		return nil
	})
	wg.Done()
	<-ctx.Done()
}
`

func TestAPI032_RemovedDoneRewritesToDefineAndSummary(t *testing.T) {
	joined := joinSuggestions(findAPI032(review.GoSource("prune.go", removedDoneSrc)))
	for _, want := range []string{
		`replace evo.Task("working tree").Done() with evo.Task("working tree").Define(func(ctx context.Context) error { ... })`,
		`replace branches.Done("3 checked") with branches.Summary("3 checked").Define(func(ctx context.Context) error { ... })`,
		`replace remotes.Done("%d stale refs", n) with remotes.Summary(fmt.Sprintf("%d stale refs", n)).Define(func(ctx context.Context) error { ... })`,
		`replace scan.Done("7 repos") with scan.Summary("7 repos")`,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing rewrite:\nwant %s\ngot  %q", want, joined)
		}
	}
	for _, notWant := range []string{"wg.Done", "ctx.Done"} {
		if strings.Contains(joined, notWant) {
			t.Fatalf("%s is not TaskHandle.Done and must not be flagged, got %q", notWant, joined)
		}
	}
}

// A bare Done() inside the Task's own Define callback is deleted, not
// rewritten: returning nil already resolves the Task.
func TestAPI032_RemovedDoneInsideOwnDefineIsDeleted(t *testing.T) {
	src := `package p

import (
	"context"

	evo "github.com/zachbornheimer/evident-output"
)

func run() {
	scan := evo.Task("scan")
	scan.Define(func(ctx context.Context) error {
		scan.Done()
		return nil
	})
}
`
	joined := joinSuggestions(findAPI032(review.GoSource("scan.go", src)))
	if !strings.Contains(joined, "delete scan.Done()") {
		t.Fatalf("want a delete suggestion, got %q", joined)
	}
}

// A 1.0 pin still has TaskHandle.Done; flagging it there would tell a
// caller to use an API their pinned version lacks (Summary is 1.1).
func TestAPI032_RemovedDoneNotFlaggedForOneZeroPin(t *testing.T) {
	for _, f := range findAPI032(review.GoSourceAt("prune.go", removedDoneSrc, "v1.0.0")) {
		if strings.Contains(f.Message, "Done was removed") {
			t.Fatalf("a v1.0.0 pin must not see the 1.1 Done removal, got %+v", f)
		}
	}
}
