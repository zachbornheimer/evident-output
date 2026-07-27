package evo_test

import (
	"io"
	"sync"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

func TestCON001_ConcurrentTaskUpdates(t *testing.T) {
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	tasks := out.Tasks("batch")
	const n = 50
	children := make([]*evo.Task, n)
	for i := 0; i < n; i++ {
		children[i] = tasks.Task("t")
	}
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(task *evo.Task) {
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
	out := evo.NewWithOptions(evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })
	items := make([]*evo.Item, 20)
	for i := range items {
		items[i] = out.Item("x")
	}
	var wg sync.WaitGroup
	for _, it := range items {
		wg.Add(1)
		go func(it *evo.Item) {
			defer wg.Done()
			it.OK()
		}(it)
	}
	wg.Wait()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
}
