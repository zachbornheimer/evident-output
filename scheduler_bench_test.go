package evo_test

import (
	"context"
	"fmt"
	"io"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// schedulerBenchWidth is the width of each Group the benchmark chains.
// Every dependent is waiting before any member settles, so each settle
// that re-tests every dependent, each re-test scanning the Group, costs
// O(width²) and the run O(width³).
const schedulerBenchWidth = 400

func BenchmarkScheduler_GroupAfterGroup(b *testing.B) {
	noop := func(context.Context) error { return nil }
	for b.Loop() {
		out := evo.Init(evo.Config{Isolated: true, Plain: true, Stdout: io.Discard, Stderr: io.Discard})
		fetch := out.Group("fetch")
		members := make([]*evo.TaskHandle, schedulerBenchWidth)
		for i := range members {
			members[i] = fetch.Task(fmt.Sprintf("fetch-%d", i))
		}
		for i := range schedulerBenchWidth {
			out.Task(fmt.Sprintf("build-%d", i)).After(fetch).Define(noop)
		}
		for _, member := range members {
			member.Define(noop)
		}
		if err := out.Finish(); err != nil {
			b.Fatal(err)
		}
		_ = out.Close()
	}
}
