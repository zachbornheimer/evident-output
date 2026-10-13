package evo

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"
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
	var elapsed time.Duration
	outer := out.Group("outer").Task("outer")
	outer.Define(func(context.Context) error {
		close(running)
		<-defined
		start := time.Now()
		for _, child := range tasks {
			if err := child.wait(); err != nil {
				return err
			}
		}
		elapsed = time.Since(start)
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
	t.Logf("200 lent-slot Waits with 5000 idle goroutines: %v", elapsed)
	if elapsed > 500*time.Millisecond {
		t.Fatalf("200 lent-slot Waits took %v: each inline claim stops the world", elapsed)
	}
}
