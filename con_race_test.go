package evo_test

import (
	"fmt"
	"io"
	"sync"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestCON001_ConcurrentTaskUpdates(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	tasks := out.DisplayGroup("batch")
	const n = 50
	children := make([]*evo.TaskHandle, n)
	for i := 0; i < n; i++ {
		children[i] = tasks.Task("t")
	}
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(task *evo.TaskHandle) {
			defer wg.Done()
			task.Progress(1, 1)
			task.Done()
		}(children[i])
	}
	wg.Wait()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if tasks.Snapshot().State != evo.Done {
		t.Fatal(tasks.Snapshot().State)
	}
}

func TestCON012_ConcurrentItemOK(t *testing.T) {
	out := evo.Init(evo.Config{Isolated: true, Options: []evo.Option{evo.To(io.Discard)}})
	t.Cleanup(func() { _ = out.Close() })
	items := make([]*evo.TaskHandle, 20)
	for i := range items {
		items[i] = out.Task("x", evo.ID(fmt.Sprintf("x%d", i)))
	}
	var wg sync.WaitGroup
	for _, it := range items {
		wg.Add(1)
		go func(it *evo.TaskHandle) {
			defer wg.Done()
			it.Done()
		}(it)
	}
	wg.Wait()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
}
