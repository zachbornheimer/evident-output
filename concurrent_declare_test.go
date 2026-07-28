package evo_test

import (
	"fmt"
	"io"
	"sync"
	"testing"

	evo "github.com/zachbornheimer/evident-output"
)

// WS-6: predeclared Tasks keep declaration order under concurrent updates.
func TestConcurrent_PredeclaredTaskOrderStable(t *testing.T) {
	out := evo.NewWithOptions(evo.Title("batch"), evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })

	jobs := out.Tasks("placement")
	const n = 20
	tasks := make([]*evo.Task, n)
	for i := 0; i < n; i++ {
		tasks[i] = jobs.Task(fmt.Sprintf("file-%02d", i), evo.ID(fmt.Sprintf("file.%d", i)))
	}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			tasks[i].Phase("work")
			tasks[i].Progress(1, 1)
			tasks[i].Done()
		}(i)
	}
	wg.Wait()

	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	snap := out.Snapshot()
	if len(snap.Collections) != 1 {
		t.Fatalf("collections = %d", len(snap.Collections))
	}
	// Child order follows declaration, not finish race.
	// Tasks may be flat or nested in collection depending on snapshot shape.
	names := make([]string, 0, n)
	for _, task := range snap.Tasks {
		names = append(names, task.Name)
	}
	if len(names) < n {
		// Collection-only listing
		for _, col := range snap.Collections {
			for _, ch := range col.Tasks {
				names = append(names, ch.Name)
			}
		}
	}
	if len(names) < n {
		t.Fatalf("expected >= %d task names, got %v", n, names)
	}
	for i := 0; i < n; i++ {
		want := fmt.Sprintf("file-%02d", i)
		if names[i] != want {
			t.Fatalf("order[%d] = %q, want %q (full=%v)", i, names[i], want, names)
		}
	}
}

func TestConcurrent_AggregateProgress_NotPerFile(t *testing.T) {
	out := evo.NewWithOptions(evo.Title("huge"), evo.To(io.Discard))
	t.Cleanup(func() { _ = out.Close() })

	// Model-by-scale: one aggregate Task for huge batches.
	t0 := out.Task("placement", evo.ID("placement"))
	const total = 10_000
	for done := 0; done <= total; done += 1000 {
		t0.Progress(done, total)
	}
	t0.Done()
	if err := out.Finish(); err != nil {
		t.Fatal(err)
	}
	if len(out.Snapshot().Tasks) != 1 {
		t.Fatalf("want 1 aggregate task, got %d", len(out.Snapshot().Tasks))
	}
}
