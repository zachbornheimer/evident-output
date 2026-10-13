package evo

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/zachbornheimer/evident-output/internal/graph"
)

// The documented lent-slot shape at MaxConcurrency 1: a running callback
// Waits on Tasks the pool has not started, so each runs inline in the slot the
// callback lends. Each such Task starts one goroutine (as Exec does). With
// 5,000 idle goroutines in the process, 200 such Waits must not each stop the
// world to dump every goroutine.
func TestLentSlotWaitCostIsIndependentOfGoroutineCount(t *testing.T) {
	stop := make(chan struct{})
	defer close(stop)
	for range 5000 {
		go func() { <-stop }()
	}
	out := initRun(Config{isolated: true, stdout: io.Discard, plain: true, MaxConcurrency: 1})
	defer func() { _ = out.close() }()
	children := out.Group("children")
	tasks := make([]*Task, 200)
	for i := range tasks {
		tasks[i] = children.Task(fmt.Sprint("child", i))
	}
	running, defined := make(chan struct{}), make(chan struct{})
	var dumps int64
	outer := out.Group("outer").Task("outer")
	outer.Define(func(context.Context) error {
		close(running)
		<-defined
		before := graph.GoroutineDumps()
		for _, child := range tasks {
			if err := child.wait(); err != nil {
				return err
			}
		}
		dumps = graph.GoroutineDumps() - before
		return nil
	})
	<-running
	for _, child := range tasks {
		child.Define(func(context.Context) error {
			done := make(chan struct{})
			go func() { close(done) }()
			<-done
			return nil
		})
	}
	close(defined)
	_ = out.finish()
	if err := outer.wait(); err != nil {
		t.Fatal(err)
	}
	if dumps > fewGetDumps {
		t.Fatalf("200 lent-slot Waits took %d goroutine dumps, want at most %d: each inline claim stops the world", dumps, fewGetDumps)
	}
}
